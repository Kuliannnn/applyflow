package config

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	CredentialKey                                   []byte
	CredentialKeyVersion                            int
	FileStorageDir, PDFInfoBin                      string
	Environment, Address, DatabaseURL, PublicOrigin string
	SessionKey, CSRFKey                             []byte
	SessionTTL                                      time.Duration
	DBMaxConns                                      int
}

func Load(get func(string) string) (Config, error) {
	c := Config{Environment: get("APP_ENV"), Address: get("HTTP_ADDR"), DatabaseURL: get("DATABASE_URL"), PublicOrigin: get("PUBLIC_ORIGIN"), SessionTTL: 30 * time.Minute, DBMaxConns: 10}
	if c.Environment == "" {
		c.Environment = "local"
	}
	if c.Environment != "local" && c.Environment != "production" {
		return c, errors.New("APP_ENV must be local or production")
	}
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return c, errors.New("invalid HTTP_ADDR")
	}
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return c, errors.New("PUBLIC_ORIGIN must be an exact http(s) origin without a path")
	}
	if c.Environment == "production" && u.Scheme != "https" {
		return c, errors.New("production requires HTTPS PUBLIC_ORIGIN")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && net.ParseIP(u.Hostname()) == nil {
		return c, errors.New("plain HTTP is restricted to loopback development")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback() {
		return c, errors.New("plain HTTP is restricted to loopback development")
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL required")
	}
	c.SessionKey, err = decodeKey(get("AUTH_SIGNING_KEY"))
	if err != nil {
		return c, errors.New("AUTH_SIGNING_KEY must be base64 with at least 32 random bytes")
	}
	c.CSRFKey, err = decodeKey(get("CSRF_KEY"))
	if err != nil {
		return c, errors.New("CSRF_KEY must be base64 with at least 32 random bytes")
	}
	if string(c.SessionKey) == string(c.CSRFKey) {
		return c, errors.New("auth and CSRF keys must differ")
	}
	if get("CREDENTIAL_MASTER_KEY") != "" || get("CREDENTIAL_KEY_VERSION") != "" {
		c.CredentialKey, err = base64.StdEncoding.DecodeString(strings.TrimSpace(get("CREDENTIAL_MASTER_KEY")))
		if err != nil || len(c.CredentialKey) != 32 {
			return c, errors.New("CREDENTIAL_MASTER_KEY must be base64 with exactly 32 random bytes")
		}
		c.CredentialKeyVersion, err = strconv.Atoi(get("CREDENTIAL_KEY_VERSION"))
		if err != nil || c.CredentialKeyVersion < 1 || c.CredentialKeyVersion > 2147483647 {
			return c, errors.New("CREDENTIAL_KEY_VERSION must be 1..2147483647")
		}
		if string(c.CredentialKey) == string(c.SessionKey) || string(c.CredentialKey) == string(c.CSRFKey) {
			return c, errors.New("credential master key must differ from signing keys")
		}
	}
	if v := get("AUTH_TTL"); v != "" {
		c.SessionTTL, err = time.ParseDuration(v)
		if err != nil || c.SessionTTL < time.Minute || c.SessionTTL > time.Hour {
			return c, errors.New("AUTH_TTL must be between 1m and 1h")
		}
	}
	if v := get("DB_MAX_CONNS"); v != "" {
		c.DBMaxConns, err = strconv.Atoi(v)
		if err != nil || c.DBMaxConns < 2 || c.DBMaxConns > 50 {
			return c, errors.New("DB_MAX_CONNS must be 2..50")
		}
	}
	c.FileStorageDir = get("FILE_STORAGE_DIR")
	c.PDFInfoBin = get("PDFINFO_BIN")
	if c.FileStorageDir == "" {
		if c.Environment == "production" {
			return c, errors.New("FILE_STORAGE_DIR required in production")
		}
		c.FileStorageDir, err = filepath.Abs("var/private-files")
		if err != nil {
			return c, err
		}
	}
	if !filepath.IsAbs(c.FileStorageDir) {
		return c, errors.New("FILE_STORAGE_DIR must be absolute")
	}
	return c, nil
}
func decodeKey(v string) ([]byte, error) {
	b, e := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
	if e != nil || len(b) < 32 {
		return nil, errors.New("invalid key")
	}
	return b, nil
}

// Worker needs the shared file directory and renderer, not API signing keys.
type Worker struct{ DatabaseURL, FileStorageDir, Python string }

func LoadWorker(get func(string) string) (Worker, error) {
	c := Worker{DatabaseURL: get("DATABASE_URL"), FileStorageDir: get("FILE_STORAGE_DIR"), Python: get("EXPORT_PYTHON")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL required")
	}
	if c.FileStorageDir == "" {
		if get("APP_ENV") == "production" {
			return c, errors.New("FILE_STORAGE_DIR required in production")
		}
		var err error
		c.FileStorageDir, err = filepath.Abs("var/private-files")
		if err != nil {
			return c, err
		}
	}
	if !filepath.IsAbs(c.FileStorageDir) {
		return c, errors.New("FILE_STORAGE_DIR must be absolute")
	}
	return c, nil
}
