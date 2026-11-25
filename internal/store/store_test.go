package store

import (
	"context"
	"testing"
	"time"

	"github.com/mononendev/photo-gallery/internal/db"
	"github.com/mononendev/photo-gallery/internal/db/dbtest"
)

func sp(s string) *string { return &s }

func newEvent(t *testing.T, s *Store, slug string, urls ...string) Event {
	t.Helper()
	in := EventInput{Slug: slug, Title: "T " + slug, EventDate: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC), Published: true}
	for _, u := range urls {
		in.Albums = append(in.Albums, AlbumInput{Name: u, ShareURL: u})
	}
	e, err := s.UpsertEvent(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMigrateTwice(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("schema_migrations rows = %d, err %v", n, err)
	}
}

func TestUpsertEventRemovesAlbumAndPhotos(t *testing.T) {
	s := New(dbtest.New(t))
	ctx := context.Background()
	e := newEvent(t, s, "ev", "https://a", "https://b")
	if len(e.Albums) != 2 {
		t.Fatalf("albums = %d", len(e.Albums))
	}
	for _, a := range e.Albums {
		err := s.ReplaceAlbumPhotos(ctx, a.ID, "g"+a.Name, []PhotoInput{{MediaKey: "k", BaseURL: "u", Width: 1, Height: 1, TakenAt: time.Now()}})
		if err != nil {
			t.Fatal(err)
		}
	}
	e = newEvent(t, s, "ev", "https://b")
	if len(e.Albums) != 1 || e.Albums[0].ShareURL != "https://b" || e.Albums[0].Position != 0 {
		t.Fatalf("albums = %+v", e.Albums)
	}
	if e.Photos != 1 {
		t.Fatalf("photos = %d, want 1", e.Photos)
	}
	var n int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM photos`).Scan(&n)
	if n != 1 {
		t.Fatalf("photos rows = %d, want 1", n)
	}
}

func TestUpsertSourcesIdempotentAndSearch(t *testing.T) {
	s := New(dbtest.New(t))
	ctx := context.Background()
	e := newEvent(t, s, "ev")
	h := uint64(0xfedcba9876543210)
	in := []SourceInput{
		{SourceID: 1, RelPath: "a.cr2", Action: sp("carving"), Keywords: []string{"trail_ride"}, DHash: &h},
		{SourceID: 2, RelPath: "b.cr2", Action: sp("jumping")},
	}
	for range 2 {
		if _, err := s.UpsertSources(ctx, e.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.SourcesForEvent(ctx, e.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("sources = %d, err %v", len(got), err)
	}
	if got[0].DHash == nil || *got[0].DHash != h || got[0].Keywords[0] != "trail_ride" || string(got[1].Exif) != "{}" {
		t.Fatalf("round trip: %+v", got)
	}
	var ids []int64
	rows, err := s.Pool.Query(ctx, `SELECT source_id FROM source_photos WHERE search @@ websearch_to_tsquery('english','carving')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("search hits = %v", ids)
	}
	var st string
	s.Pool.QueryRow(ctx, `SELECT search_text FROM source_photos WHERE source_id = 1`).Scan(&st)
	if st != "trail ride carving" {
		t.Fatalf("search_text = %q", st)
	}
}

func TestReplaceAlbumPhotosKeepsHidden(t *testing.T) {
	s := New(dbtest.New(t))
	ctx := context.Background()
	e := newEvent(t, s, "ev", "https://a")
	aid := e.Albums[0].ID
	p := func(k string) PhotoInput {
		return PhotoInput{MediaKey: k, BaseURL: "u", Width: 1, Height: 1, TakenAt: time.Now()}
	}
	if err := s.ReplaceAlbumPhotos(ctx, aid, "", []PhotoInput{p("a"), p("b")}); err != nil {
		t.Fatal(err)
	}
	var id int64
	s.Pool.QueryRow(ctx, `SELECT id FROM photos WHERE media_key = 'a'`).Scan(&id)
	if err := s.SetPhotoHidden(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceAlbumPhotos(ctx, aid, "G1", []PhotoInput{p("a")}); err != nil {
		t.Fatal(err)
	}
	var hidden bool
	var n int
	s.Pool.QueryRow(ctx, `SELECT hidden FROM photos WHERE media_key = 'a'`).Scan(&hidden)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM photos`).Scan(&n)
	if !hidden || n != 1 {
		t.Fatalf("hidden=%v n=%d", hidden, n)
	}
	e, _ = s.GetEvent(ctx, "ev")
	if e.Albums[0].GoogleAlbumID == nil || *e.Albums[0].GoogleAlbumID != "G1" {
		t.Fatal("google_album_id not stored")
	}
}

func TestEventLifecycle(t *testing.T) {
	s := New(dbtest.New(t))
	ctx := context.Background()
	newEvent(t, s, "one", "https://a")
	if _, err := s.UpsertEvent(ctx, EventInput{Slug: "two", Title: "x", EventDate: time.Now()}); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListEvents(ctx, true)
	pub, _ := s.ListEvents(ctx, false)
	if len(all) != 2 || len(pub) != 1 {
		t.Fatalf("all=%d pub=%d", len(all), len(pub))
	}
	if err := s.DeleteEvent(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEvent(ctx, "one"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.GetEvent(ctx, "one"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}
