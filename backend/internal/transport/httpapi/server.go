package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"applyflow/backend/internal/adapters/security"
	"applyflow/backend/internal/aisettings"
	"applyflow/backend/internal/auth"
	"applyflow/backend/internal/document"
	"applyflow/backend/internal/export"
	"applyflow/backend/internal/files"
	"applyflow/backend/internal/generation"
	"applyflow/backend/internal/intake"
	"applyflow/backend/internal/resume"
	"applyflow/backend/internal/studio"
	"applyflow/backend/internal/task"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Options struct {
	AI                  *aisettings.Service
	Exports             export.Store
	Intake              *intake.Service
	Generations         studio.GenerationStore
	Documents           document.Store
	Tasks               task.ReadStore
	Files               *files.Service
	Resumes             *resume.Service
	Origin              string
	SessionKey, CSRFKey []byte
	SessionTTL          time.Duration
	Ready               func(context.Context) error
	Logger              *slog.Logger
}
type API struct {
	auth          *auth.Service
	studio        *studio.Service
	options       Options
	sessions      security.Sessions
	csrf          security.CSRF
	limits        *limiter
	passwordSlots chan struct{}
	uploadSlots   chan struct{}
}

func New(a *auth.Service, s *studio.Service, o Options) http.Handler {
	api := &API{auth: a, studio: s, options: o, sessions: security.Sessions{Key: o.SessionKey, Issuer: o.Origin, TTL: o.SessionTTL}, csrf: security.CSRF{Key: o.CSRFKey}, limits: newLimiter(), passwordSlots: make(chan struct{}, 4), uploadSlots: make(chan struct{}, 2)}
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.HandleMethodNotAllowed = true
	r.Use(api.common)
	r.GET("/health/live", func(c *gin.Context) { respond(c, 200, gin.H{"status": "ok"}) })
	r.GET("/health/ready", func(c *gin.Context) {
		if o.Ready != nil && o.Ready(c.Request.Context()) != nil {
			problem(c, 503, "not_ready")
			return
		}
		respond(c, 200, gin.H{"status": "ready"})
	})
	r.GET("/api/auth/csrf", api.issueCSRF)
	r.POST("/api/auth/register", api.csrfGuard, api.register)
	r.POST("/api/auth/login", api.csrfGuard, api.login)
	r.POST("/api/auth/logout", api.csrfGuard, api.logout)
	r.GET("/api/auth/me", api.requireSession, api.me)
	g := r.Group("/api/workspaces", api.requireSession)
	g.GET("", api.listWorkspaces)
	g.POST("", api.csrfGuard, api.createWorkspace)
	g.GET("/:id", api.getWorkspace)
	g.PATCH("/:id", api.csrfGuard, api.patchWorkspace)
	if o.Files != nil {
		f := r.Group("/api/files", api.requireSession)
		f.POST("", api.csrfGuard, api.uploadFile)
		f.GET("/:id/download", api.downloadFile)
	}
	if o.Resumes != nil {
		rs := r.Group("/api/resumes", api.requireSession)
		rs.GET("", api.listResumes)
		rs.POST("", api.csrfGuard, api.importResume)
		rs.GET("/:id", api.getResume)
		rs.PATCH("/:id", api.csrfGuard, api.patchResume)
		rs.POST("/:id/revisions", api.csrfGuard, api.confirmResume)
		rs.GET("/:id/revisions/:revision_id", api.getResumeRevision)
	}
	if o.Intake != nil {
		g.POST("/:id/sources", api.csrfGuard, api.createSource)
		g.GET("/:id/sources/:source_id", api.getSource)
		g.POST("/:id/job-revisions", api.csrfGuard, api.confirmJob)
		g.GET("/:id/job-revisions/:revision_id", api.getJobRevision)
	}
	if o.Generations != nil {
		g.POST("/:id/generations", api.csrfGuard, api.generate)
		g.GET("/:id/generations/:run_id", api.getGeneration)
		g.POST("/:id/generations/:run_id/retry", api.csrfGuard, api.retryGeneration)
	}
	if o.Documents != nil {
		g.GET("/:id/documents", api.listDocuments)
		d := r.Group("/api/documents", api.requireSession)
		d.GET("/:id", api.getDocument)
		d.GET("/:id/revisions", api.listDocumentRevisions)
		d.POST("/:id/revisions", api.csrfGuard, api.saveDocument)
		d.GET("/:id/revisions/:revision_id", api.getDocumentRevision)
		d.POST("/:id/apply", api.csrfGuard, api.applyDocument)
	}
	if o.Tasks != nil {
		t := r.Group("/api/tasks", api.requireSession)
		t.GET("/:id", api.getTask)
		t.POST("/:id/cancel", api.csrfGuard, api.cancelTask)
	}
	if o.Exports != nil {
		ex := r.Group("/api/documents", api.requireSession)
		ex.POST("/:id/exports", api.csrfGuard, api.createExport)
		ex.GET("/:id/exports/:export_id", api.getExport)
	}
	if o.AI != nil {
		r.GET("/api/ai/providers", api.requireSession, api.aiProviders)
		r.GET("/api/me/ai-usage", api.requireSession, api.aiUsage)
		a := r.Group("/api/me/ai-config", api.requireSession)
		a.GET("", api.getAI)
		a.PUT("", api.csrfGuard, api.putAI)
		a.PATCH("", api.csrfGuard, api.patchAI)
		a.DELETE("", api.csrfGuard, api.deleteAI)
		a.POST("/test", api.csrfGuard, api.testAI)
	}
	r.NoRoute(func(c *gin.Context) { problem(c, 404, "not_found") })
	r.NoMethod(func(c *gin.Context) { problem(c, 405, "method_not_allowed") })
	return r
}
func (api *API) common(c *gin.Context) {
	c.Set("request_id", security.UUID())
	c.Header("X-Request-ID", c.GetString("request_id"))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	defer func() {
		if recover() != nil {
			if api.options.Logger != nil {
				api.options.Logger.Error("request panic", "request_id", c.GetString("request_id"))
			}
			if !c.Writer.Written() {
				problem(c, 500, "internal_error")
			}
			c.Abort()
		}
	}()
	if raw, err := c.Cookie("applyflow_session"); err == nil {
		if claims, err := api.sessions.Verify(raw, time.Now()); err == nil {
			c.Set("session", claims)
		}
	}
	c.Next()
	if api.options.Logger != nil {
		api.options.Logger.Info("http request", "request_id", c.GetString("request_id"), "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "error_code", c.GetString("error_code"))
	}
}
func session(c *gin.Context) *jwt.RegisteredClaims {
	v, ok := c.Get("session")
	if !ok {
		return nil
	}
	return v.(*jwt.RegisteredClaims)
}
func owner(c *gin.Context) string { return session(c).Subject }
func (api *API) requireSession(c *gin.Context) {
	if session(c) == nil {
		problem(c, 401, "unauthorized")
		return
	}
	c.Next()
}
func (api *API) csrfGuard(c *gin.Context) {
	if c.GetHeader("Origin") != api.options.Origin {
		problem(c, 403, "csrf_failed")
		return
	}
	cookie, err := c.Cookie("applyflow_csrf")
	if err != nil {
		problem(c, 403, "csrf_failed")
		return
	}
	binding := "anonymous"
	if claims := session(c); claims != nil {
		binding = claims.ID
	}
	expiredLogout := c.Request.URL.Path == "/api/auth/logout" && session(c) == nil
	if !api.csrf.Verify(cookie, c.GetHeader("X-CSRF-Token"), binding, time.Now(), expiredLogout) {
		problem(c, 403, "csrf_failed")
		return
	}
	c.Next()
}
func (api *API) failure(c *gin.Context, err error) {
	var validation files.ValidationError
	var aiFailure generation.Failure
	switch {
	case errors.As(err, &aiFailure):
		status := 409
		if aiFailure == "ai_daily_limit" {
			status = 429
		}
		problem(c, status, aiFailure.SafeCode())
	case errors.As(err, &validation):
		problem(c, 415, string(validation))
	case errors.Is(err, files.ErrValidatorUnavailable):
		problem(c, 503, "file_validator_unavailable")
	case errors.Is(err, files.ErrUnsupported):
		problem(c, 415, "unsupported_source_file")
	case errors.Is(err, auth.ErrInvalid), errors.Is(err, studio.ErrInvalid):
		problem(c, 400, "validation_error")
	case errors.Is(err, auth.ErrCredentials), errors.Is(err, auth.ErrNotFound):
		problem(c, 401, "unauthorized")
	case errors.Is(err, auth.ErrEmailTaken):
		problem(c, 409, "email_taken")
	case errors.Is(err, studio.ErrNotFound):
		problem(c, 404, "not_found")
	case errors.Is(err, studio.ErrConflict):
		problem(c, 409, "version_conflict")
	case errors.Is(err, studio.ErrKeyConflict):
		problem(c, 409, "idempotency_conflict")
	case errors.Is(err, studio.ErrArchived):
		problem(c, 409, "archived")
	default:
		if api.options.Logger != nil {
			api.options.Logger.Error("database or service failure", "request_id", c.GetString("request_id"))
		}
		problem(c, 503, "unavailable")
	}
}
