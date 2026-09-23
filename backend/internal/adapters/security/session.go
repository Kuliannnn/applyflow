package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Passwords struct{ Cost int }

func (p Passwords) Hash(v string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(v), p.Cost)
	return string(b), e
}
func (p Passwords) Compare(hash, v string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(v)) == nil
}
func UUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure entropy unavailable")
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func ValidUUID(v string) bool {
	if len(v) != 36 || v[8] != '-' || v[13] != '-' || v[18] != '-' || v[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(v, "-", ""))
	return err == nil
}

var ErrToken = errors.New("invalid security token")

type Sessions struct {
	Key    []byte
	Issuer string
	TTL    time.Duration
}

func (s Sessions) Issue(user string, now time.Time) (string, *jwt.RegisteredClaims, error) {
	c := &jwt.RegisteredClaims{Issuer: s.Issuer, Audience: jwt.ClaimStrings{"applyflow"}, Subject: user, ID: UUID(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.TTL))}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.Key)
	return token, c, err
}
func (s Sessions) Verify(raw string, now time.Time) (*jwt.RegisteredClaims, error) {
	if len(raw) > 4096 {
		return nil, ErrToken
	}
	c := &jwt.RegisteredClaims{}
	t, err := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) { return s.Key, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(s.Issuer), jwt.WithAudience("applyflow"), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !t.Valid || !ValidUUID(c.Subject) || !ValidUUID(c.ID) || c.IssuedAt == nil {
		return nil, ErrToken
	}
	return c, nil
}

type csrfClaims struct {
	Binding string `json:"b"`
	Nonce   string `json:"n"`
	Expires int64  `json:"e"`
}
type CSRF struct{ Key []byte }

func (c CSRF) Issue(binding string, now time.Time) (string, time.Time) {
	expiry := now.Add(time.Hour).Truncate(time.Second)
	body, _ := json.Marshal(csrfClaims{binding, UUID(), expiry.Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(body)
	return encoded + "." + c.sign(encoded), expiry
}
func (c CSRF) sign(v string) string {
	mac := hmac.New(sha256.New, c.Key)
	mac.Write([]byte("csrf:" + v))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (c CSRF) Verify(cookie, header, binding string, now time.Time, allowExpiredSessionLogout bool) bool {
	if len(cookie) > 1024 || !hmac.Equal([]byte(cookie), []byte(header)) {
		return false
	}
	parts := strings.Split(cookie, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(c.sign(parts[0]))) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var claims csrfClaims
	if json.Unmarshal(raw, &claims) != nil || claims.Expires <= now.Unix() || !ValidUUID(claims.Nonce) {
		return false
	}
	return claims.Binding == binding || allowExpiredSessionLogout
}
