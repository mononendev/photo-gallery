package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mononendev/photo-gallery/internal/db/dbtest"
)

func TestSyncStates(t *testing.T) {
	ctx := context.Background()
	s := New(dbtest.New(t))
	newEvent(t, s, "ev")

	if _, err := s.StartSync(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
	id, err := s.StartSync(ctx, "ev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSync(ctx, "ev"); !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("second start: %v", err)
	}
	if err := s.MarkInterruptedSyncs(ctx); err != nil {
		t.Fatal(err)
	}
	e, _ := s.GetEvent(ctx, "ev")
	if e.SyncState != "failed" || e.SyncError == nil || *e.SyncError != "interrupted by a restart" || e.SyncFinishedAt == nil {
		t.Fatalf("after interrupt: %+v", e)
	}

	id, err = s.StartSync(ctx, "ev")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSync(ctx, id, json.RawMessage(`{"items":3}`), nil); err != nil {
		t.Fatal(err)
	}
	e, _ = s.GetEvent(ctx, "ev")
	if e.SyncState != "done" || e.SyncError != nil || string(e.SyncResult) != `{"items": 3}` {
		t.Fatalf("after done: %+v result %s", e, e.SyncResult)
	}

	id, _ = s.StartSync(ctx, "ev")
	if err := s.FinishSync(ctx, id, nil, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	e, _ = s.GetEvent(ctx, "ev")
	if e.SyncState != "failed" || e.SyncError == nil || *e.SyncError != "boom" || e.SyncResult != nil {
		t.Fatalf("after fail: %+v", e)
	}
}
