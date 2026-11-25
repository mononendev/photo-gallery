package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mononendev/photo-gallery/internal/db/dbtest"
	"github.com/mononendev/photo-gallery/internal/store"
)

type fixture struct {
	h  http.Handler
	st *store.Store
	// photo ids by media key
	ids map[string]int64
}

func sp(s string) *string { return &s }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := dbtest.New(t)
	st := store.New(pool)
	ctx := context.Background()
	date := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
	ev, err := st.UpsertEvent(ctx, store.EventInput{Slug: "ev", Title: "Ev", EventDate: date, Published: true,
		Albums: []store.AlbumInput{{Name: "Highlights", ShareURL: "https://photos.app.goo.gl/a"}, {Name: "All", ShareURL: "https://photos.app.goo.gl/b"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertEvent(ctx, store.EventInput{Slug: "draft", Title: "Draft", EventDate: date,
		Albums: []store.AlbumInput{{Name: "D", ShareURL: "https://photos.app.goo.gl/d"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertSources(ctx, ev.ID, []store.SourceInput{
		{SourceID: 1, RelPath: "a", Action: sp("carving"), Keywords: []string{"trail", "onewheel"}, Rating: ip(3), QualityScore: ip(4),
			Exif: json.RawMessage(`{"lens":"85mm"}`)},
		{SourceID: 2, RelPath: "b", Action: sp("standing"), Keywords: []string{"trail"}, Rating: ip(1)},
		{SourceID: 3, RelPath: "c", Action: sp("carving"), Keywords: []string{"trail"}, Rating: ip(2)},
	}); err != nil {
		t.Fatal(err)
	}
	srcs, _ := st.SourcesForEvent(ctx, ev.ID)
	sid := map[int64]int64{}
	for _, s := range srcs {
		sid[s.SourceID] = s.ID
	}
	t0 := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)
	ph := func(key string, sec int, src int64) store.PhotoInput {
		in := store.PhotoInput{MediaKey: key, BaseURL: "https://lh3/" + key, Width: 3, Height: 2,
			TakenAt: t0.Add(time.Duration(sec) * time.Second), Position: sec}
		if src != 0 {
			id := sid[src]
			m := "time"
			in.SourcePhotoID, in.MatchMethod = &id, &m
		}
		return in
	}
	// Highlights: A(src1), B(src2), U(unmatched). All: A again (src1), C(src3), H(src2, will be hidden).
	if err := st.ReplaceAlbumPhotos(ctx, ev.Albums[0].ID, "", []store.PhotoInput{ph("A", 1, 1), ph("B", 2, 2), ph("U", 3, 0)}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceAlbumPhotos(ctx, ev.Albums[1].ID, "", []store.PhotoInput{ph("A2", 1, 1), ph("C", 4, 3), ph("H", 5, 0)}); err != nil {
		t.Fatal(err)
	}
	dev, _ := st.GetEvent(ctx, "draft")
	if err := st.ReplaceAlbumPhotos(ctx, dev.Albums[0].ID, "", []store.PhotoInput{ph("X", 1, 0)}); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	rows, _ := pool.Query(ctx, `SELECT media_key, id FROM photos`)
	for rows.Next() {
		var k string
		var id int64
		rows.Scan(&k, &id)
		ids[k] = id
	}
	rows.Close()
	if err := st.SetPhotoHidden(ctx, ids["H"], true); err != nil {
		t.Fatal(err)
	}
	return &fixture{h: (&Server{Pool: pool, Version: "t"}).Public(), st: st, ids: ids}
}

func ip(n int) *int { return &n }

func (f *fixture) get(t *testing.T, path string, out any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if out != nil && rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatal(err, rec.Body.String())
		}
	}
	return rec
}

type page struct {
	Items []store.PublicPhoto `json:"items"`
	Total int                 `json:"total"`
}

func urls(p page) []string {
	var out []string
	for _, i := range p.Items {
		out = append(out, i.URL)
	}
	return out
}

func TestSearchCarving(t *testing.T) {
	f := newFixture(t)
	var p page
	f.get(t, "/api/photos?event=ev&q=carving", &p)
	if p.Total != 2 {
		t.Fatalf("got %v", urls(p))
	}
	for _, i := range p.Items {
		if i.Action == nil || *i.Action != "carving" {
			t.Fatalf("not carving: %+v", i)
		}
	}
	f.get(t, "/api/photos?keyword=trail&keyword=onewheel", &p)
	if p.Total != 1 {
		t.Fatalf("keyword AND: %v", urls(p))
	}
}

func TestHiddenAbsent(t *testing.T) {
	f := newFixture(t)
	var p page
	f.get(t, "/api/photos?event=ev", &p)
	for _, i := range p.Items {
		if i.ID == f.ids["H"] {
			t.Fatal("hidden photo listed")
		}
	}
	if rec := f.get(t, "/api/photos/"+itoa(f.ids["H"]), nil); rec.Code != 404 {
		t.Fatalf("hidden detail: %d", rec.Code)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestUnpublishedEvent404(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/api/events/draft", "/api/events/nope", "/api/photos?event=draft", "/api/photos/" + itoa(f.ids["X"])} {
		if rec := f.get(t, path, nil); rec.Code != 404 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	var evs []store.PublicEvent
	rec := f.get(t, "/api/events", &evs)
	if len(evs) != 1 || evs[0].Slug != "ev" || evs[0].PhotoCount != 4 || evs[0].CoverURL == nil || *evs[0].CoverURL != "https://lh3/A" {
		t.Fatalf("%+v", evs)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatal("no cache header")
	}
	var all page
	f.get(t, "/api/photos", &all)
	if all.Total != 4 {
		t.Fatalf("all published: %v", urls(all))
	}
	if rec := f.get(t, "/api/events/nope", nil); rec.Header().Get("Cache-Control") != "" {
		t.Fatal("error cached")
	}
}

func TestDedupeAcrossAlbums(t *testing.T) {
	f := newFixture(t)
	var p page
	f.get(t, "/api/photos?event=ev", &p)
	n := 0
	for _, i := range p.Items {
		if i.URL == "https://lh3/A" || i.URL == "https://lh3/A2" {
			n++
			if i.Album != "Highlights" {
				t.Fatalf("kept album %s", i.Album)
			}
		}
	}
	if n != 1 || p.Total != 4 {
		t.Fatalf("n=%d total=%d %v", n, p.Total, urls(p))
	}
}

func TestRatingNone(t *testing.T) {
	f := newFixture(t)
	var p page
	f.get(t, "/api/photos?event=ev&rating=none", &p)
	if p.Total != 1 || p.Items[0].URL != "https://lh3/U" {
		t.Fatalf("%v", urls(p))
	}
	if p.Items[0].Keywords == nil || string(p.Items[0].Exif) != "{}" || p.Items[0].Rating != nil {
		t.Fatalf("%+v", p.Items[0])
	}
	f.get(t, "/api/photos?event=ev&rating=3&rating=2", &p)
	if p.Total != 2 {
		t.Fatalf("%v", urls(p))
	}
	if rec := f.get(t, "/api/photos?rating=9", nil); rec.Code != 400 {
		t.Fatalf("bad rating: %d", rec.Code)
	}
	if rec := f.get(t, "/api/photos?sort=x", nil); rec.Code != 400 {
		t.Fatalf("bad sort: %d", rec.Code)
	}
}

func TestPrevNext(t *testing.T) {
	f := newFixture(t)
	var d store.PublicPhotoDetail
	// Taken order after dedupe: A, B, U, C.
	order := []string{"A", "B", "U", "C"}
	for i, k := range order {
		f.get(t, "/api/photos/"+itoa(f.ids[k]), &d)
		var prev, next *int64
		if i > 0 {
			prev = ptr(f.ids[order[i-1]])
		}
		if i < len(order)-1 {
			next = ptr(f.ids[order[i+1]])
		}
		if !eq(d.PrevID, prev) || !eq(d.NextID, next) {
			t.Fatalf("%s: prev=%v next=%v", k, d.PrevID, d.NextID)
		}
	}
	var ev store.PublicEventDetail
	f.get(t, "/api/events/ev", &ev)
	if len(ev.Facets.Keywords) != 2 || ev.Facets.Keywords[0].Keyword != "trail" || ev.Facets.Keywords[0].Count != 3 ||
		len(ev.Facets.Lenses) != 1 || len(ev.Facets.Albums) != 2 {
		t.Fatalf("%+v", ev.Facets)
	}
}

func ptr(n int64) *int64 { return &n }
func eq(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
