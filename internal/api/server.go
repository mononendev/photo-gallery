// Package api is the gallery's HTTP API: the public read-only routes under /api and the admin routes under
// /admin-api. They are served on two ports so only the public one is reachable from outside the cluster.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds what the handlers need.
type Server struct {
	Pool    *pgxpool.Pool
	Version string
	Log     *slog.Logger
}

// HTTPError is an error with a status; the body is {"detail": ...}.
type HTTPError struct {
	Code   int
	Detail any
}

func (e *HTTPError) Error() string { return fmt.Sprint(e.Detail) }

func errf(code int, format string, a ...any) error {
	return &HTTPError{code, fmt.Sprintf(format, a...)}
}

type handler func(r *http.Request) (any, error)

func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := h(r)
		if err != nil {
			var he *HTTPError
			if !errors.As(err, &he) {
				s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
				he = &HTTPError{500, err.Error()}
			}
			writeJSON(w, he.Code, map[string]any{"detail": he.Detail})
			return
		}
		if v == nil {
			v = json.RawMessage("null")
		}
		writeJSON(w, 200, v)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

type router func(pattern string, h handler)

func (s *Server) mux(routes func(router)) http.Handler {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	m := http.NewServeMux()
	routes(func(pattern string, h handler) { m.HandleFunc(pattern, s.wrap(h)) })
	return m
}

// Public is the read-only API behind the public ingress.
func (s *Server) Public() http.Handler { return cacheable(s.mux(s.publicRoutes)) }

// Admin is the management API: no login, reachable only on the LAN and from inside the cluster.
func (s *Server) Admin() http.Handler { return s.mux(s.adminRoutes) }

func (s *Server) health(r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	return map[string]any{"ok": true, "version": s.Version, "database": s.Pool.Ping(ctx) == nil}, nil
}
