package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRejectUnsafeConfiguration(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://localhost/db", "PUBLIC_ORIGIN": "http://localhost:8080", "AUTH_SIGNING_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))), "CSRF_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))}
	if _, err := Load(func(k string) string { return base[k] }); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"PUBLIC_ORIGIN": "http://public.example", "AUTH_SIGNING_KEY": "short", "CSRF_KEY": base["AUTH_SIGNING_KEY"], "AUTH_TTL": "72h", "DB_MAX_CONNS": "0", "APP_ENV": "production"} {
		t.Run(k, func(t *testing.T) {
			_, err := Load(func(key string) string {
				if key == k {
					return v
				}
				return base[key]
			})
			if err == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}
