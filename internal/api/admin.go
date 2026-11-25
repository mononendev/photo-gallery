package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mononendev/photo-gallery/internal/albumsync"
	"github.com/mononendev/photo-gallery/internal/dhash"
	"github.com/mononendev/photo-gallery/internal/gphotos"
	"github.com/mononendev/photo-gallery/internal/store"
)

const (
	maxSourcesPerRequest = 500
	maxBodyBytes         = 64 << 20
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (s *Server) adminRoutes(route router) {
	route("GET /api/health", s.health)
	route("GET /admin-api/events", s.adminListEvents)
	route("PUT /admin-api/events/{slug}", s.adminPutEvent)
	route("DELETE /admin-api/events/{slug}", s.adminDeleteEvent)
	route("POST /admin-api/events/{slug}/sources", s.adminPostSources)
	route("POST /admin-api/events/{slug}/sync", s.adminStartSync)
	route("GET /admin-api/events/{slug}/sync", s.adminSyncStatus)
	route("GET /admin-api/events/{slug}/photos", s.adminPhotos)
	route("PATCH /admin-api/photos/{id}", s.adminPatchPhoto)
}

func (s *Server) store() *store.Store { return store.New(s.Pool) }

type adminAlbum struct {
	Name     string `json:"name"`
	ShareURL string `json:"share_url"`
}

type syncStatus struct {
	SyncState      string          `json:"sync_state"`
	SyncStartedAt  *time.Time      `json:"sync_started_at"`
	SyncFinishedAt *time.Time      `json:"sync_finished_at"`
	SyncResult     json.RawMessage `json:"sync_result"`
	SyncError      *string         `json:"sync_error"`
}

func statusOf(e store.Event) syncStatus {
	return syncStatus{e.SyncState, e.SyncStartedAt, e.SyncFinishedAt, e.SyncResult, e.SyncError}
}

type adminEvent struct {
	Slug        string       `json:"slug"`
	Title       string       `json:"title"`
	Series      *string      `json:"series"`
	EventDate   string       `json:"event_date"`
	Description string       `json:"description"`
	Published   bool         `json:"published"`
	Albums      []adminAlbum `json:"albums"`
	syncStatus
	Photos  int `json:"photos"`
	Matched int `json:"matched"`
	Sources int `json:"sources"`
}

func toAdminEvent(e store.Event) adminEvent {
	albums := make([]adminAlbum, len(e.Albums))
	for i, a := range e.Albums {
		albums[i] = adminAlbum{a.Name, a.ShareURL}
	}
	return adminEvent{
		Slug: e.Slug, Title: e.Title, Series: e.Series, EventDate: e.EventDate.Format("2006-01-02"),
		Description: e.Description, Published: e.Published, Albums: albums, syncStatus: statusOf(e),
		Photos: e.Photos, Matched: e.Matched, Sources: e.Sources,
	}
}

func eventNotFound(slug string, err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return errf(404, "event %s not found", slug)
	}
	return err
}

func (s *Server) adminListEvents(r *http.Request) (any, error) {
	evs, err := s.store().ListEvents(r.Context(), true)
	if err != nil {
		return nil, err
	}
	out := make([]adminEvent, len(evs))
	for i, e := range evs {
		out[i] = toAdminEvent(e)
	}
	return out, nil
}

func (s *Server) adminPutEvent(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	if !slugRe.MatchString(slug) {
		return nil, errf(400, "slug must match %s", slugRe)
	}
	var body struct {
		Title       string       `json:"title"`
		Series      *string      `json:"series"`
		EventDate   string       `json:"event_date"`
		Description string       `json:"description"`
		Published   bool         `json:"published"`
		Albums      []adminAlbum `json:"albums"`
	}
	if err := decodeBody(r, &body); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body.Title) == "" {
		return nil, errf(400, "title is required")
	}
	date, err := time.Parse("2006-01-02", body.EventDate)
	if err != nil {
		return nil, errf(400, "event_date must be YYYY-MM-DD")
	}
	in := store.EventInput{Slug: slug, Title: body.Title, EventDate: date, Description: body.Description, Published: body.Published}
	if body.Series != nil && strings.TrimSpace(*body.Series) != "" {
		in.Series = body.Series
	}
	seen := map[string]bool{}
	for i, a := range body.Albums {
		if !strings.HasPrefix(a.ShareURL, "https://photos.app.goo.gl/") && !strings.HasPrefix(a.ShareURL, "https://photos.google.com/share/") {
			return nil, errf(400, "album %d: share_url must start with https://photos.app.goo.gl/ or https://photos.google.com/share/", i+1)
		}
		if seen[a.ShareURL] {
			return nil, errf(400, "album %d: share_url is listed twice", i+1)
		}
		seen[a.ShareURL] = true
		name := strings.TrimSpace(a.Name)
		if name == "" {
			return nil, errf(400, "album %d: name is required", i+1)
		}
		in.Albums = append(in.Albums, store.AlbumInput{Name: name, ShareURL: a.ShareURL})
	}
	if _, err := s.store().UpsertEvent(r.Context(), in); err != nil {
		return nil, err
	}
	e, err := s.store().GetEvent(r.Context(), slug)
	if err != nil {
		return nil, err
	}
	return toAdminEvent(e), nil
}

func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		return errf(400, "invalid JSON body: %v", err)
	}
	return nil
}

func (s *Server) adminDeleteEvent(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	if err := s.store().DeleteEvent(r.Context(), slug); err != nil {
		return nil, eventNotFound(slug, err)
	}
	return status{Code: 204}, nil
}

// sourceWire is the photosort -> gallery contract. Everything except source_id and rel_path may be null or missing.
type sourceWire struct {
	SourceID     int64           `json:"source_id"`
	RelPath      string          `json:"rel_path"`
	Taken        *string         `json:"taken"`
	TakenMS      *int            `json:"taken_ms"`
	DHash        *string         `json:"dhash"`
	Rating       *int            `json:"rating"`
	FocusTier    *int            `json:"focus_tier"`
	QualityScore *int            `json:"quality_score"`
	Keeper       *bool           `json:"keeper"`
	Subject      *string         `json:"subject"`
	Composition  *string         `json:"composition"`
	Action       *string         `json:"action"`
	Description  *string         `json:"description"`
	Keywords     []string        `json:"keywords"`
	Adjectives   []string        `json:"adjectives"`
	Exif         json.RawMessage `json:"exif"`
}

func (w sourceWire) input(i int) (store.SourceInput, error) {
	if w.RelPath == "" || w.SourceID == 0 {
		return store.SourceInput{}, errf(400, "photo %d: source_id and rel_path are required", i)
	}
	in := store.SourceInput{
		SourceID: w.SourceID, RelPath: w.RelPath, Taken: w.Taken, TakenMS: w.TakenMS, Rating: w.Rating,
		FocusTier: w.FocusTier, QualityScore: w.QualityScore, Keeper: w.Keeper, Subject: w.Subject,
		Composition: w.Composition, Action: w.Action, Description: w.Description,
		Keywords: w.Keywords, Adjectives: w.Adjectives, Exif: w.Exif,
	}
	if in.Keywords == nil {
		in.Keywords = []string{}
	}
	if in.Adjectives == nil {
		in.Adjectives = []string{}
	}
	if w.DHash != nil {
		h, err := dhash.ParseHex(*w.DHash)
		if err != nil {
			return store.SourceInput{}, errf(400, "photo %d: bad dhash %q: %v", i, *w.DHash, err)
		}
		in.DHash = &h
	}
	return in, nil
}

func (s *Server) adminPostSources(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	var body struct {
		Photos []sourceWire `json:"photos"`
	}
	if err := decodeBody(r, &body); err != nil {
		return nil, err
	}
	if len(body.Photos) > maxSourcesPerRequest {
		return nil, errf(413, "at most %d photos per request", maxSourcesPerRequest)
	}
	ev, err := s.store().GetEvent(r.Context(), slug)
	if err != nil {
		return nil, eventNotFound(slug, err)
	}
	in := make([]store.SourceInput, len(body.Photos))
	for i, p := range body.Photos {
		if in[i], err = p.input(i); err != nil {
			return nil, err
		}
	}
	n, err := s.store().UpsertSources(r.Context(), ev.ID, in)
	if err != nil {
		return nil, err
	}
	return map[string]int{"upserted": n}, nil
}

func (s *Server) adminStartSync(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	st := s.store()
	id, err := st.StartSync(r.Context(), slug)
	switch {
	case errors.Is(err, store.ErrSyncRunning):
		return nil, errf(409, "sync already running")
	case err != nil:
		return nil, eventNotFound(slug, err)
	}
	base := s.BaseCtx
	if base == nil {
		base = context.Background()
	}
	gp := s.GPhotos
	if gp == nil {
		gp = &gphotos.Client{}
	}
	go func() {
		if _, err := albumsync.RunStarted(base, st, gp, id, slug); err != nil {
			s.Log.Error("sync failed", "event", slug, "err", err)
		}
	}()
	return status{Code: 202, Body: map[string]string{"sync_state": "running"}}, nil
}

func (s *Server) adminSyncStatus(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	e, err := s.store().GetEvent(r.Context(), slug)
	if err != nil {
		return nil, eventNotFound(slug, err)
	}
	return statusOf(e), nil
}

func (s *Server) adminPhotos(r *http.Request) (any, error) {
	slug := r.PathValue("slug")
	offset, err := intParam(r, "offset", 0, 0, 1<<31-1)
	if err != nil {
		return nil, err
	}
	limit, err := intParam(r, "limit", 100, 1, 500)
	if err != nil {
		return nil, err
	}
	items, total, err := s.store().AdminPhotos(r.Context(), slug, offset, limit)
	if err != nil {
		return nil, eventNotFound(slug, err)
	}
	return map[string]any{"items": items, "total": total}, nil
}

// intParam reads an integer query parameter; a value above max is clamped to hi.
func intParam(r *http.Request, name string, def, lo, hi int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo {
		return 0, errf(400, "%s must be an integer >= %d", name, lo)
	}
	return min(n, hi), nil
}

func (s *Server) adminPatchPhoto(r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errf(404, "photo not found")
	}
	var body struct {
		Hidden *bool `json:"hidden"`
	}
	if err := decodeBody(r, &body); err != nil {
		return nil, err
	}
	if body.Hidden == nil {
		return nil, errf(400, "hidden is required")
	}
	if err := s.store().SetPhotoHidden(r.Context(), id, *body.Hidden); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errf(404, "photo not found")
		}
		return nil, err
	}
	return status{Code: 204}, nil
}
