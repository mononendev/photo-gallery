package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mononendev/photo-gallery/internal/store"
)

func (s *Server) publicRoutes(route router) {
	route("GET /api/health", s.health)
	route("GET /api/events", s.publicEvents)
	route("GET /api/events/{slug}", s.publicEvent)
	route("GET /api/photos", s.publicPhotos)
	route("GET /api/photos/{id}", s.publicPhoto)
}

// cacheable adds Cache-Control to successful responses of the event and photo routes.
func cacheable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; strings.HasPrefix(p, "/api/events") || strings.HasPrefix(p, "/api/photos") {
			w = &cacheWriter{ResponseWriter: w}
		}
		next.ServeHTTP(w, r)
	})
}

type cacheWriter struct{ http.ResponseWriter }

func (c *cacheWriter) WriteHeader(code int) {
	if code == http.StatusOK {
		c.Header().Set("Cache-Control", "public, max-age=60")
	}
	c.ResponseWriter.WriteHeader(code)
}

func (s *Server) publicEvents(r *http.Request) (any, error) {
	return store.New(s.Pool).ListPublicEvents(r.Context())
}

func (s *Server) publicEvent(r *http.Request) (any, error) {
	d, err := store.New(s.Pool).GetPublicEvent(r.Context(), r.PathValue("slug"))
	if errors.Is(err, store.ErrNotFound) {
		return nil, errf(404, "event not found")
	}
	return d, err
}

func intList(vals []string, name string, min, max int) ([]int, error) {
	var out []int
	for _, v := range vals {
		n, err := strconv.Atoi(v)
		if err != nil || n < min || n > max {
			return nil, errf(400, "invalid %s %q", name, v)
		}
		out = append(out, n)
	}
	return out, nil
}

func parsePhotoFilter(r *http.Request) (store.PhotoFilter, error) {
	q := r.URL.Query()
	f := store.PhotoFilter{
		Event: q.Get("event"), Q: q.Get("q"), Keywords: q["keyword"], Lens: q.Get("lens"), Album: q.Get("album"),
		Sort: q.Get("sort"), Limit: 60,
	}
	var err error
	for _, v := range q["rating"] {
		if v == "none" {
			f.RatingNone = true
			continue
		}
		n, e := intList([]string{v}, "rating", 0, 4)
		if e != nil {
			return f, e
		}
		f.Ratings = append(f.Ratings, n...)
	}
	if f.Tiers, err = intList(q["tier"], "tier", 0, 3); err != nil {
		return f, err
	}
	if v := q.Get("min_score"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 || n > 5 {
			return f, errf(400, "invalid min_score %q", v)
		}
		f.MinScore = &n
	}
	switch q.Get("keeper") {
	case "", "0":
	case "1":
		f.KeeperOnly = true
	default:
		return f, errf(400, "invalid keeper %q", q.Get("keeper"))
	}
	switch f.Sort {
	case "", "taken", "score", "rating":
	default:
		return f, errf(400, "invalid sort %q", f.Sort)
	}
	if v := q.Get("offset"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return f, errf(400, "invalid offset %q", v)
		}
		f.Offset = n
	}
	if v := q.Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 {
			return f, errf(400, "invalid limit %q", v)
		}
		f.Limit = min(n, 200)
	}
	return f, nil
}

func (s *Server) publicPhotos(r *http.Request) (any, error) {
	f, err := parsePhotoFilter(r)
	if err != nil {
		return nil, err
	}
	st := store.New(s.Pool)
	if f.Event != "" {
		if ok, err := st.PublicEventExists(r.Context(), f.Event); err != nil {
			return nil, err
		} else if !ok {
			return nil, errf(404, "event not found")
		}
	}
	items, total, err := st.ListPublicPhotos(r.Context(), f)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total}, nil
}

func (s *Server) publicPhoto(r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errf(404, "photo not found")
	}
	d, err := store.New(s.Pool).GetPublicPhoto(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errf(404, "photo not found")
	}
	return d, err
}
