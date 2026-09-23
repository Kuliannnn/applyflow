package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"applyflow/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

type identityDTO struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	ProfileVersion int64     `json:"profile_version"`
	CreatedAt      time.Time `json:"created_at"`
}

func identity(u auth.Identity) identityDTO {
	return identityDTO{u.ID, u.Email, u.ProfileVersion, u.CreatedAt}
}
func (api *API) setCookie(c *gin.Context, name, value string, expires time.Time, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(api.options.Origin, "https://"), SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: age})
}
func (api *API) issueCSRF(c *gin.Context) {
	if !api.allow(c, "csrf:"+c.ClientIP(), 60, time.Minute) {
		return
	}
	binding := "anonymous"
	if claims := session(c); claims != nil {
		binding = claims.ID
	}
	token, expiry := api.csrf.Issue(binding, time.Now())
	api.setCookie(c, "applyflow_csrf", token, expiry, 3600)
	respond(c, 200, gin.H{"csrf_token": token, "expires_at": expiry})
}
func (api *API) register(c *gin.Context) { api.credentials(c, true) }
func (api *API) login(c *gin.Context)    { api.credentials(c, false) }
func (api *API) credentials(c *gin.Context, register bool) {
	if !api.allow(c, "auth-ip:"+c.ClientIP(), 30, time.Minute) {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(in.Email))))
	if !api.allow(c, "auth-email:"+hex.EncodeToString(h[:]), 10, 15*time.Minute) {
		return
	}
	select {
	case api.passwordSlots <- struct{}{}:
		defer func() { <-api.passwordSlots }()
	default:
		c.Header("Retry-After", "1")
		problem(c, 429, "rate_limited")
		return
	}
	var u auth.Identity
	var err error
	if register {
		u, err = api.auth.Register(c.Request.Context(), in.Email, in.Password)
	} else {
		u, err = api.auth.Login(c.Request.Context(), in.Email, in.Password)
	}
	if err != nil {
		api.failure(c, err)
		return
	}
	token, claims, err := api.sessions.Issue(u.ID, time.Now())
	if err != nil {
		api.failure(c, err)
		return
	}
	api.setCookie(c, "applyflow_session", token, claims.ExpiresAt.Time, int(api.options.SessionTTL.Seconds()))
	// Rotate anonymous CSRF context; callers fetch a token bound to the new session.
	api.setCookie(c, "applyflow_csrf", "", time.Unix(1, 0), -1)
	status := 200
	if register {
		status = 201
	}
	respond(c, status, gin.H{"user": identity(u), "expires_at": claims.ExpiresAt.Time})
}
func (api *API) logout(c *gin.Context) {
	api.setCookie(c, "applyflow_session", "", time.Unix(1, 0), -1)
	// Keep the signed context until its expiry so repeating logout remains idempotent.
	c.Status(204)
}
func (api *API) me(c *gin.Context) {
	u, err := api.auth.Identity(c.Request.Context(), owner(c))
	if err != nil {
		api.failure(c, err)
		return
	}
	respond(c, 200, gin.H{"user": identity(u), "expires_at": session(c).ExpiresAt.Time})
}
