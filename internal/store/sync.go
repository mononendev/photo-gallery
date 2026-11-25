package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrSyncRunning is returned by StartSync when the event is already syncing.
var ErrSyncRunning = errors.New("sync already running")

// StartSync marks the event as running and returns its id. It returns ErrSyncRunning when a sync is running
// and ErrNotFound when the slug does not exist.
func (s *Store) StartSync(ctx context.Context, slug string) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `UPDATE events SET sync_state = 'running', sync_started_at = now(),
			sync_finished_at = NULL, sync_result = NULL, sync_error = NULL
		WHERE slug = $1 AND sync_state <> 'running' RETURNING id`, slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE slug = $1)`, slug).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}
	return 0, ErrSyncRunning
}

// FinishSync records the end of a sync: done with the result, or failed with the error text.
func (s *Store) FinishSync(ctx context.Context, eventID int64, result json.RawMessage, syncErr error) error {
	if syncErr != nil {
		_, err := s.Pool.Exec(ctx, `UPDATE events SET sync_state = 'failed', sync_error = $2, sync_finished_at = now()
			WHERE id = $1`, eventID, syncErr.Error())
		return err
	}
	_, err := s.Pool.Exec(ctx, `UPDATE events SET sync_state = 'done', sync_result = $2, sync_error = NULL,
		sync_finished_at = now() WHERE id = $1`, eventID, []byte(result))
	return err
}

// MarkInterruptedSyncs fails every sync left in 'running' by a previous process.
func (s *Store) MarkInterruptedSyncs(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE events SET sync_state = 'failed', sync_error = 'interrupted by a restart',
		sync_finished_at = now() WHERE sync_state = 'running'`)
	return err
}
