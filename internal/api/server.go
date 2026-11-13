// Package api exposes the Pulsewatch REST API and serves the dashboard.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"pulsewatch/internal/notify"
	"pulsewatch/internal/store"
)

// Refresher is notified when monitor configuration changes.
type Refresher interface{ Refresh() }

// Options configure the API server.
type Options struct {
	AgentToken string
	APIToken   string
	WebDir     string
	CORSOrigin string
}

// Server holds handler dependencies.
type Server struct {
	store     *store.Store
	scheduler Refresher
	notifier  *notify.Notifier
	log       *slog.Logger
	opts      Options
}

// New creates the API server.
func New(st *store.Store, sched Refresher, n *notify.Notifier, log *slog.Logger, opts Options) *Server {
	return &Server{store: st, scheduler: sched, notifier: n, log: log, opts: opts}
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()

	api.HandleFunc("GET /api/v1/overview", s.overview)

	api.HandleFunc("GET /api/v1/hosts", s.listHosts)
	api.HandleFunc("GET /api/v1/hosts/{id}", s.getHost)
	api.HandleFunc("DELETE /api/v1/hosts/{id}", s.deleteHost)
	api.HandleFunc("GET /api/v1/hosts/{id}/metrics", s.hostMetrics)
	api.HandleFunc("GET /api/v1/metrics/hosts", s.fleetMetrics)

	api.HandleFunc("GET /api/v1/monitors", s.listMonitors)
	api.HandleFunc("POST /api/v1/monitors", s.createMonitor)
	api.HandleFunc("GET /api/v1/monitors/{id}", s.getMonitor)
	api.HandleFunc("PUT /api/v1/monitors/{id}", s.updateMonitor)
	api.HandleFunc("DELETE /api/v1/monitors/{id}", s.deleteMonitor)
	api.HandleFunc("GET /api/v1/monitors/{id}/results", s.monitorResults)
	api.HandleFunc("GET /api/v1/monitors/{id}/series", s.monitorSeries)
	api.HandleFunc("GET /api/v1/metrics/monitors", s.fleetMonitorSeries)

	api.HandleFunc("GET /api/v1/alerts", s.listAlerts)
	api.HandleFunc("POST /api/v1/alerts/{id}/acknowledge", s.ackAlert)
	api.HandleFunc("GET /api/v1/alert-rules", s.listRules)
	api.HandleFunc("POST /api/v1/alert-rules", s.createRule)
	api.HandleFunc("PUT /api/v1/alert-rules/{id}", s.updateRule)
	api.HandleFunc("DELETE /api/v1/alert-rules/{id}", s.deleteRule)

	api.HandleFunc("GET /api/v1/webhooks", s.listWebhooks)
	api.HandleFunc("POST /api/v1/webhooks", s.createWebhook)
	api.HandleFunc("DELETE /api/v1/webhooks/{id}", s.deleteWebhook)
	api.HandleFunc("POST /api/v1/webhooks/{id}/test", s.testWebhook)

	api.HandleFunc("GET /api/v1/events", s.listEvents)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.HandleFunc("GET /readyz", s.readyz)
	// The agent endpoint authenticates with the agent token and accepts larger bodies.
	root.Handle("POST /api/v1/agent/report", s.limitBody(4<<20, s.requireToken(s.opts.AgentToken, http.HandlerFunc(s.agentReport))))
	root.Handle("/api/", s.limitBody(64<<10, s.requireToken(s.opts.APIToken, api)))
	if s.opts.WebDir != "" {
		root.Handle("/", s.spa(s.opts.WebDir))
	}

	return s.recoverer(s.requestLog(s.securityHeaders(s.cors(root))))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "database": "unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "database": "ok"})
}

// requireToken enforces "Authorization: Bearer <token>" when token is set.
func (s *Server) requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="pulsewatch"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitBody(n int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, n)
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const requestIDKey ctxKey = 0

func requestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))

		// Skip noisy probe and static asset logs at info level.
		lvl := slog.LevelInfo
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/v1/agent/report" {
			lvl = slog.LevelDebug
		}
		if sw.status >= 500 {
			lvl = slog.LevelError
		}
		s.log.Log(r.Context(), lvl, "http request", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "duration_ms", time.Since(start).Milliseconds(), "request_id", id)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic in handler", "panic", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Content-Security-Policy",
				"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

// cors allows a single configured origin (useful for running the Vite dev
// server against a separate API). Disabled when no origin is configured.
func (s *Server) cors(next http.Handler) http.Handler {
	origin := s.opts.CORSOrigin
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves the built dashboard, falling back to index.html for client routes.
func (s *Server) spa(dir string) http.Handler {
	fsrv := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		// http.Dir already rejects ".." traversal; Clean keeps the stat consistent.
		p := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fsrv.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
