package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminPhoto is one album photo as the admin grid shows it, hidden ones included. SourceID is photosort's id.
type AdminPhoto struct {
	ID          int64     `json:"id"`
	Album       string    `json:"album"`
	BaseURL     string    `json:"base_url"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	TakenAt     time.Time `json:"taken_at"`
	Hidden      bool      `json:"hidden"`
	MatchMethod *string   `json:"match_method"`
	SourceID    *int64    `json:"source_id"`
	RelPath     *string   `json:"rel_path"`
}

// AdminPhotos returns a page of the event's photos ordered by taken_at, album position, position, and the
// total count. It returns ErrNotFound when the slug does not exist.
func (s *Store) AdminPhotos(ctx context.Context, slug string, offset, limit int) ([]AdminPhoto, int, error) {
	var eventID int64
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT e.id, (SELECT count(*) FROM photos p JOIN albums a ON a.id = p.album_id
			WHERE a.event_id = e.id) FROM events e WHERE e.slug = $1`, slug).Scan(&eventID, &total); err != nil {
		return nil, 0, notFound(err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT p.id, a.name, p.base_url, p.width, p.height, p.taken_at, p.hidden,
			p.match_method, sp.source_id, sp.rel_path
		FROM photos p JOIN albums a ON a.id = p.album_id
		LEFT JOIN source_photos sp ON sp.id = p.source_photo_id
		WHERE a.event_id = $1 ORDER BY p.taken_at, a.position, p.position, p.id OFFSET $2 LIMIT $3`, eventID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AdminPhoto{}
	for rows.Next() {
		var p AdminPhoto
		if err := rows.Scan(&p.ID, &p.Album, &p.BaseURL, &p.Width, &p.Height, &p.TakenAt, &p.Hidden,
			&p.MatchMethod, &p.SourceID, &p.RelPath); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
