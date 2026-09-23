package security

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSessionVerification(t *testing.T) {
	now := time.Unix(1800000000, 0)
	s := Sessions{Key: []byte(strings.Repeat("k", 32)), Issuer: "https://applyflow.example", TTL: 30 * time.Minute}
	raw, c, err := s.Issue(UUID(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(raw, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(raw, now.Add(time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err = s.Verify(raw+"tampered", now); err == nil {
		t.Fatal("tampered signature accepted")
	}
	for _, mutation := range []func(*jwt.RegisteredClaims){func(c *jwt.RegisteredClaims) { c.Audience = jwt.ClaimStrings{"other"} }, func(c *jwt.RegisteredClaims) { c.Issuer = "https://other.example" }, func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil }, func(c *jwt.RegisteredClaims) { c.Subject = "not-uuid" }} {
		altered := *c
		mutation(&altered)
		token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, altered).SignedString(s.Key)
		if _, err = s.Verify(token, now); err == nil {
			t.Fatal("invalid claim accepted")
		}
	}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS384, c).SignedString(s.Key)
	if _, err = s.Verify(token, now); err == nil {
		t.Fatal("unexpected signing algorithm accepted")
	}
}
func TestCSRFBindsHeaderCookieAndSession(t *testing.T) {
	now := time.Now()
	c := CSRF{Key: []byte(strings.Repeat("c", 32))}
	token, _ := c.Issue("session-A", now)
	if !c.Verify(token, token, "session-A", now, false) {
		t.Fatal("valid token rejected")
	}
	if c.Verify(token, token, "session-B", now, false) {
		t.Fatal("token from other session accepted")
	}
	if c.Verify(token, "different", "session-A", now, false) {
		t.Fatal("cookie/header mismatch accepted")
	}
	if c.Verify(token, token, "session-A", now.Add(2*time.Hour), false) {
		t.Fatal("expired CSRF accepted")
	}
	if !c.Verify(token, token, "anonymous", now, true) {
		t.Fatal("expired-session logout cannot clear cookie")
	}
	if c.Verify(token+"bad", token+"bad", "anonymous", now, true) {
		t.Fatal("logout bypassed signature")
	}
}
