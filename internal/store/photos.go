package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SourceInput is one photosort row. Taken is the EXIF DateTimeOriginal text "YYYY:MM:DD HH:MM:SS";
// DHash is stored as the int64 cast of the uint64 hash.
type SourceInput struct {
	SourceID     int64
	RelPath      string
	Taken        *string
	TakenMS      *int
	DHash        *uint64
	Rating       *int
	FocusTier    *int
	QualityScore *int
	Keeper       *bool
	Subject      *string
	Composition  *string
	Action       *string
	Description  *string
	Keywords     []string
	Adjectives   []string
	Exif         json.RawMessage // JSON object; empty means {}
}

type Source struct {
	ID      int64
	EventID int64
	SourceInput
}

func searchText(in SourceInput) string {
	var parts []string
	add := func(s string) {
		if s = strings.TrimSpace(strings.ReplaceAll(s, "_", " ")); s != "" {
			parts = append(parts, s)
		}
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	add(str(in.Description))
	for _, k := range in.Keywords {
		add(k)
	}
	for _, a := range in.Adjectives {
		add(a)
	}
	add(str(in.Subject))
	add(str(in.Composition))
	add(str(in.Action))
	return strings.Join(parts, " ")
}

// UpsertSources upserts by (event_id, source_id) and returns the number of rows written.
func (s *Store) UpsertSources(ctx context.Context, eventID int64, in []SourceInput) (int, error) {
	batch := &pgx.Batch{}
	for _, p := range in {
		var dh *int64
		if p.DHash != nil {
			v := int64(*p.DHash)
			dh = &v
		}
		kw, adj := p.Keywords, p.Adjectives
		if kw == nil {
			kw = []string{}
		}
		if adj == nil {
			adj = []string{}
		}
		exif := "{}"
		if len(p.Exif) > 0 && string(p.Exif) != "null" {
			exif = string(p.Exif)
		}
		batch.Queue(`INSERT INTO source_photos (event_id, source_id, rel_path, taken, taken_ms, dhash, rating, focus_tier,
				quality_score, keeper, subject, composition, action, description, keywords, adjectives, exif, search_text)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18)
			ON CONFLICT (event_id, source_id) DO UPDATE SET rel_path = EXCLUDED.rel_path, taken = EXCLUDED.taken,
				taken_ms = EXCLUDED.taken_ms, dhash = EXCLUDED.dhash, rating = EXCLUDED.rating,
				focus_tier = EXCLUDED.focus_tier, quality_score = EXCLUDED.quality_score, keeper = EXCLUDED.keeper,
				subject = EXCLUDED.subject, composition = EXCLUDED.composition, action = EXCLUDED.action,
				description = EXCLUDED.description, keywords = EXCLUDED.keywords, adjectives = EXCLUDED.adjectives,
				exif = EXCLUDED.exif, search_text = EXCLUDED.search_text, updated_at = now()`,
			eventID, p.SourceID, p.RelPath, p.Taken, p.TakenMS, dh, p.Rating, p.FocusTier, p.QualityScore, p.Keeper,
			p.Subject, p.Composition, p.Action, p.Description, kw, adj, exif, searchText(p))
	}
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		br := tx.SendBatch(ctx, batch)
		for range in {
			tag, err := br.Exec()
			if err != nil {
				br.Close()
				return err
			}
			n += int(tag.RowsAffected())
		}
		return br.Close()
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// SourcesForEvent returns every source row of the event ordered by (taken, taken_ms, id).
func (s *Store) SourcesForEvent(ctx context.Context, eventID int64) ([]Source, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, event_id, source_id, rel_path, taken, taken_ms, dhash, rating, focus_tier,
			quality_score, keeper, subject, composition, action, description, keywords, adjectives, exif
		FROM source_photos WHERE event_id = $1 ORDER BY taken, taken_ms, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var x Source
		var ms, rating, tier, score *int16
		var dh *int64
		if err := rows.Scan(&x.ID, &x.EventID, &x.SourceID, &x.RelPath, &x.Taken, &ms, &dh, &rating, &tier, &score,
			&x.Keeper, &x.Subject, &x.Composition, &x.Action, &x.Description, &x.Keywords, &x.Adjectives, &x.Exif); err != nil {
			return nil, err
		}
		x.TakenMS, x.Rating, x.FocusTier, x.QualityScore = int16p(ms), int16p(rating), int16p(tier), int16p(score)
		if dh != nil {
			v := uint64(*dh)
			x.DHash = &v
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func int16p(p *int16) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

type PhotoInput struct {
	MediaKey        string
	BaseURL         string
	Width           int
	Height          int
	TakenAt         time.Time
	TZOffsetMinutes int
	Position        int
	SourcePhotoID   *int64  // source_photos.id, not photosort's source_id
	MatchMethod     *string // "time", "time_hash" or "hash"
}

// ReplaceAlbumPhotos upserts the photos by (album_id, media_key) and deletes the album's rows whose media_key
// is not in the input, in one transaction. hidden keeps its value on update. googleAlbumID is stored on the
// album when not empty.
func (s *Store) ReplaceAlbumPhotos(ctx context.Context, albumID int64, googleAlbumID string, in []PhotoInput) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if googleAlbumID != "" {
			if _, err := tx.Exec(ctx, `UPDATE albums SET google_album_id = $2 WHERE id = $1`, albumID, googleAlbumID); err != nil {
				return err
			}
		}
		keys := make([]string, len(in))
		batch := &pgx.Batch{}
		for i, p := range in {
			keys[i] = p.MediaKey
			batch.Queue(`INSERT INTO photos (album_id, media_key, base_url, width, height, taken_at, tz_offset_minutes,
					position, source_photo_id, match_method)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT (album_id, media_key) DO UPDATE SET base_url = EXCLUDED.base_url, width = EXCLUDED.width,
					height = EXCLUDED.height, taken_at = EXCLUDED.taken_at, tz_offset_minutes = EXCLUDED.tz_offset_minutes,
					position = EXCLUDED.position, source_photo_id = EXCLUDED.source_photo_id,
					match_method = EXCLUDED.match_method`,
				albumID, p.MediaKey, p.BaseURL, p.Width, p.Height, p.TakenAt, p.TZOffsetMinutes, p.Position,
				p.SourcePhotoID, p.MatchMethod)
		}
		br := tx.SendBatch(ctx, batch)
		for range in {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return err
			}
		}
		if err := br.Close(); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM photos WHERE album_id = $1 AND media_key <> ALL($2)`, albumID, keys)
		return err
	})
}
