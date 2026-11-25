// Package store is the hand-written SQL layer over the gallery's Postgres tables.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested event does not exist.
var ErrNotFound = errors.New("not found")

type Store struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Album struct {
	ID            int64   `json:"-"`
	Name          string  `json:"name"`
	ShareURL      string  `json:"share_url"`
	Position      int     `json:"-"`
	GoogleAlbumID *string `json:"-"`
}

type AlbumInput struct {
	Name     string
	ShareURL string
}

type EventInput struct {
	Slug        string
	Title       string
	Series      *string
	EventDate   time.Time
	Description string
	Published   bool
	Albums      []AlbumInput
}

// Event carries the sync state columns and, from GetEvent/ListEvents, the albums and counts.
type Event struct {
	ID             int64
	Slug           string
	Title          string
	Series         *string
	EventDate      time.Time
	Description    string
	Published      bool
	SyncState      string
	SyncStartedAt  *time.Time
	SyncFinishedAt *time.Time
	SyncResult     json.RawMessage
	SyncError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Albums         []Album
	Photos         int // album photos, all albums, hidden included
	Matched        int // album photos with a source row
	Sources        int // source_photos rows
}

const eventCols = `e.id, e.slug, e.title, e.series, e.event_date, e.description, e.published, e.sync_state,
	e.sync_started_at, e.sync_finished_at, e.sync_result, e.sync_error, e.created_at, e.updated_at,
	(SELECT count(*) FROM photos p JOIN albums a ON a.id = p.album_id WHERE a.event_id = e.id),
	(SELECT count(*) FROM photos p JOIN albums a ON a.id = p.album_id WHERE a.event_id = e.id AND p.source_photo_id IS NOT NULL),
	(SELECT count(*) FROM source_photos sp WHERE sp.event_id = e.id)`

func scanEvent(row pgx.Row) (Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.Slug, &e.Title, &e.Series, &e.EventDate, &e.Description, &e.Published, &e.SyncState,
		&e.SyncStartedAt, &e.SyncFinishedAt, &e.SyncResult, &e.SyncError, &e.CreatedAt, &e.UpdatedAt,
		&e.Photos, &e.Matched, &e.Sources)
	return e, err
}

func (s *Store) albumsFor(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, eventIDs []int64) (map[int64][]Album, error) {
	rows, err := q.Query(ctx, `SELECT id, event_id, name, share_url, position, google_album_id
		FROM albums WHERE event_id = ANY($1) ORDER BY event_id, position`, eventIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Album{}
	for rows.Next() {
		var a Album
		var eid int64
		if err := rows.Scan(&a.ID, &eid, &a.Name, &a.ShareURL, &a.Position, &a.GoogleAlbumID); err != nil {
			return nil, err
		}
		out[eid] = append(out[eid], a)
	}
	return out, rows.Err()
}

// UpsertEvent inserts or updates the event by slug and replaces its album set: albums whose share_url is
// not in the input are deleted (their photos cascade); the others are upserted with position = index.
func (s *Store) UpsertEvent(ctx context.Context, in EventInput) (Event, error) {
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, `INSERT INTO events (slug, title, series, event_date, description, published)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (slug) DO UPDATE SET title = EXCLUDED.title, series = EXCLUDED.series,
				event_date = EXCLUDED.event_date, description = EXCLUDED.description,
				published = EXCLUDED.published, updated_at = now()
			RETURNING id`, in.Slug, in.Title, in.Series, in.EventDate, in.Description, in.Published).Scan(&id); err != nil {
			return err
		}
		urls := make([]string, len(in.Albums))
		for i, a := range in.Albums {
			urls[i] = a.ShareURL
		}
		if _, err := tx.Exec(ctx, `DELETE FROM albums WHERE event_id = $1 AND share_url <> ALL($2)`, id, urls); err != nil {
			return err
		}
		for i, a := range in.Albums {
			if _, err := tx.Exec(ctx, `INSERT INTO albums (event_id, name, share_url, position) VALUES ($1,$2,$3,$4)
				ON CONFLICT (event_id, share_url) DO UPDATE SET name = EXCLUDED.name, position = EXCLUDED.position`,
				id, a.Name, a.ShareURL, i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, in.Slug)
}

// GetEvent returns ErrNotFound when the slug does not exist.
func (s *Store) GetEvent(ctx context.Context, slug string) (Event, error) {
	e, err := scanEvent(s.Pool.QueryRow(ctx, `SELECT `+eventCols+` FROM events e WHERE e.slug = $1`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	albums, err := s.albumsFor(ctx, s.Pool, []int64{e.ID})
	if err != nil {
		return Event{}, err
	}
	e.Albums = albums[e.ID]
	return e, nil
}

// ListEvents returns events newest first.
func (s *Store) ListEvents(ctx context.Context, includeUnpublished bool) ([]Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+eventCols+` FROM events e
		WHERE $1 OR e.published ORDER BY e.event_date DESC, e.id DESC`, includeUnpublished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	var ids []int64
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
		ids = append(ids, e.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	albums, err := s.albumsFor(ctx, s.Pool, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Albums = albums[out[i].ID]
	}
	return out, nil
}

// DeleteEvent returns ErrNotFound when the slug does not exist.
func (s *Store) DeleteEvent(ctx context.Context, slug string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM events WHERE slug = $1`, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPhotoHidden returns ErrNotFound when the photo does not exist.
func (s *Store) SetPhotoHidden(ctx context.Context, id int64, hidden bool) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE photos SET hidden = $2 WHERE id = $1`, id, hidden)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
