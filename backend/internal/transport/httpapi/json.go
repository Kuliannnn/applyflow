package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func respond(c *gin.Context, status int, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		problem(c, 500, "internal_error")
		return
	}
	c.Data(status, "application/json; charset=utf-8", b)
}
func problem(c *gin.Context, status int, code string) {
	c.Set("error_code", code)
	b, _ := json.Marshal(map[string]any{"type": "urn:applyflow:problem:" + code, "title": http.StatusText(status), "status": status, "code": code, "request_id": c.GetString("request_id")})
	c.Data(status, "application/problem+json", b)
	c.Abort()
}
func decode(c *gin.Context, out any) bool { return decodeLimit(c, out, 131072) }
func decodeLimit(c *gin.Context, out any, limit int64) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		problem(c, 415, "unsupported_media_type")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			problem(c, 413, "payload_too_large")
		} else {
			problem(c, 400, "invalid_json")
		}
		return false
	}
	if !utf8.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		problem(c, 400, "invalid_json")
		return false
	}
	// encoding/json otherwise silently accepts duplicate fields. Reject at every depth.
	tokens := json.NewDecoder(bytes.NewReader(raw))
	tokens.UseNumber()
	if err = uniqueValue(tokens, 0); err != nil {
		problem(c, 400, "invalid_json")
		return false
	}
	if _, err = tokens.Token(); err != io.EOF {
		problem(c, 400, "invalid_json")
		return false
	}
	// Enforce exact JSON field names; encoding/json alone accepts case-insensitive names.
	if err := exactShape(raw, reflect.TypeOf(out).Elem()); err != nil {
		problem(c, 400, "invalid_json")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		problem(c, 400, "invalid_json")
		return false
	}
	return true
}
func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON too deep")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("duplicate JSON field")
			}
			seen[key] = true
			if err = uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected delimiter")
	}
	_, err = d.Token()
	return err
}

// Validate nested names too: encoding/json matches fields case-insensitively.
func exactShape(raw []byte, t reflect.Type) error {
	if t == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		if string(raw) == "null" {
			return nil
		}
		return exactShape(raw, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return errors.New("object required")
		}
		allowed := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			allowed[strings.Split(f.Tag.Get("json"), ",")[0]] = f.Type
		}
		for k, v := range fields {
			ft, ok := allowed[k]
			if !ok {
				return errors.New("unknown field")
			}
			if err := exactShape(v, ft); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return errors.New("array required")
		}
		for _, v := range items {
			if err := exactShape(v, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
