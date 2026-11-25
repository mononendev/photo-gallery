package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The public queries show only published events and photos with hidden = false. A picture that is in more than
// one album of an event (same source row, or same media key) is returned once, from the album with the lowest
// position. Hidden photos are dropped before the dedupe, so a visible copy in another album still shows.

const dedupeKey = `COALESCE('s' || p.source_photo_id, 'm' || p.media_key)`

const photoCols = `p.id, e.id AS event_id, e.slug AS event_slug, a.name AS album, p.base_url AS url, p.width, p.height,
	p.taken_at, sp.rating, sp.focus_tier, sp.quality_score, sp.keeper, sp.subject, sp.composition, sp.action,
	sp.description, COALESCE(sp.keywords, '{}') AS keywords, COALESCE(sp.adjectives, '{}') AS adjectives,
	COALESCE(sp.exif, '{}'::jsonb) AS exif, ` + dedupeKey + ` AS dkey, a.position AS apos, p.position AS ppos`

const photoFrom = `FROM photos p JOIN albums a ON a.id = p.album_id JOIN events e ON e.id = a.event_id
	LEFT JOIN source_photos sp ON sp.id = p.source_photo_id`

const takenOrder = `taken_at, apos, ppos, id`

// PublicEvent is one published event with its album links.
type PublicEvent struct {
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Series      *string     `json:"series"`
	EventDate   string      `json:"event_date"`
	Description string      `json:"description"`
	PhotoCount  int         `json:"photo_count"`
	CoverURL    *string     `json:"cover_url"`
	Albums      []AlbumLink `json:"albums"`
}

type AlbumLink struct {
	Name     string `json:"name"`
	ShareURL string `json:"share_url"`
}

type KeywordCount struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count"`
}

type Facets struct {
	Keywords []KeywordCount `json:"keywords"`
	Lenses   []string       `json:"lenses"`
	Albums   []string       `json:"albums"`
}

// PublicEventDetail is a published event with the filter facets of its public photos.
type PublicEventDetail struct {
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Series      *string     `json:"series"`
	EventDate   string      `json:"event_date"`
	Description string      `json:"description"`
	Albums      []AlbumLink `json:"albums"`
	Facets      Facets      `json:"facets"`
}

// PublicPhoto is the public photo JSON; metadata fields are null (lists [], exif {}) when unmatched.
type PublicPhoto struct {
	ID           int64           `json:"id"`
	EventSlug    string          `json:"event_slug"`
	Album        string          `json:"album"`
	URL          string          `json:"url"`
	Width        int             `json:"width"`
	Height       int             `json:"height"`
	TakenAt      time.Time       `json:"taken_at"`
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

// PublicPhotoDetail adds the neighbors in the event's deduped taken order.
type PublicPhotoDetail struct {
	PublicPhoto
	PrevID *int64 `json:"prev_id"`
	NextID *int64 `json:"next_id"`
}

// PhotoFilter is the validated query of ListPublicPhotos. Empty fields do not filter.
type PhotoFilter struct {
	Event      string
	Q          string
	Keywords   []string // all must be present
	Ratings    []int    // any of
	RatingNone bool     // include photos with no rating, matched or not
	Tiers      []int
	MinScore   *int
	KeeperOnly bool
	Lens       string
	Album      string
	Sort       string // "taken" (default), "score", "rating"
	Offset     int
	Limit      int
}

func (s *Store) publicAlbums(ctx context.Context, eventIDs []int64) (map[int64][]Album, error) {
	return s.albumsFor(ctx, s.Pool, eventIDs)
}

func albumLinks(as []Album) []AlbumLink {
	out := make([]AlbumLink, 0, len(as))
	for _, a := range as {
		out = append(out, AlbumLink{a.Name, a.ShareURL})
	}
	return out
}

// ListPublicEvents returns published events, newest first. photo_count counts deduped visible photos.
func (s *Store) ListPublicEvents(ctx context.Context) ([]PublicEvent, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH pub AS (
			SELECT DISTINCT ON (e.id, `+dedupeKey+`) e.id AS event_id, p.base_url, p.taken_at, a.position AS apos,
				p.position AS ppos, p.id
			`+photoFrom+`
			WHERE e.published AND NOT p.hidden
			ORDER BY e.id, `+dedupeKey+`, a.position, p.position
		), agg AS (
			SELECT event_id, count(*) AS n, (array_agg(base_url ORDER BY taken_at, apos, ppos, id))[1] AS cover
			FROM pub GROUP BY event_id
		)
		SELECT e.id, e.slug, e.title, e.series, e.event_date, e.description, COALESCE(agg.n, 0), agg.cover
		FROM events e LEFT JOIN agg ON agg.event_id = e.id
		WHERE e.published ORDER BY e.event_date DESC, e.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicEvent{}
	var ids []int64
	for rows.Next() {
		var e PublicEvent
		var id int64
		var d time.Time
		if err := rows.Scan(&id, &e.Slug, &e.Title, &e.Series, &d, &e.Description, &e.PhotoCount, &e.CoverURL); err != nil {
			return nil, err
		}
		e.EventDate = d.Format("2006-01-02")
		out = append(out, e)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	albums, err := s.publicAlbums(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Albums = albumLinks(albums[ids[i]])
	}
	return out, nil
}

// GetPublicEvent returns ErrNotFound for unknown and unpublished events.
func (s *Store) GetPublicEvent(ctx context.Context, slug string) (PublicEventDetail, error) {
	var d PublicEventDetail
	var id int64
	var date time.Time
	err := s.Pool.QueryRow(ctx, `SELECT id, slug, title, series, event_date, description FROM events
		WHERE slug = $1 AND published`, slug).Scan(&id, &d.Slug, &d.Title, &d.Series, &date, &d.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.EventDate = date.Format("2006-01-02")
	albums, err := s.publicAlbums(ctx, []int64{id})
	if err != nil {
		return d, err
	}
	d.Albums = albumLinks(albums[id])
	d.Facets = Facets{Keywords: []KeywordCount{}, Lenses: []string{}, Albums: []string{}}
	for _, a := range albums[id] {
		d.Facets.Albums = append(d.Facets.Albums, a.Name)
	}

	const pub = `WITH pub AS (
		SELECT DISTINCT ON (` + dedupeKey + `) sp.keywords, sp.exif ` + photoFrom + `
		WHERE e.id = $1 AND NOT p.hidden
		ORDER BY ` + dedupeKey + `, a.position, p.position
	)`
	rows, err := s.Pool.Query(ctx, pub+` SELECT k, count(*) AS n FROM pub, unnest(keywords) AS k
		GROUP BY k ORDER BY n DESC, k LIMIT 30`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var k KeywordCount
		if err := rows.Scan(&k.Keyword, &k.Count); err != nil {
			rows.Close()
			return d, err
		}
		d.Facets.Keywords = append(d.Facets.Keywords, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}
	rows, err = s.Pool.Query(ctx, pub+` SELECT DISTINCT exif->>'lens' AS l FROM pub
		WHERE exif->>'lens' IS NOT NULL AND exif->>'lens' <> '' ORDER BY l`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return d, err
		}
		d.Facets.Lenses = append(d.Facets.Lenses, l)
	}
	return d, rows.Err()
}

func scanPhoto(row pgx.Row, extra ...any) (PublicPhoto, error) {
	var p PublicPhoto
	var eid int64
	var dkey string
	var apos, ppos int
	var rating, tier, score *int16
	dest := []any{&p.ID, &eid, &p.EventSlug, &p.Album, &p.URL, &p.Width, &p.Height, &p.TakenAt, &rating, &tier, &score,
		&p.Keeper, &p.Subject, &p.Composition, &p.Action, &p.Description, &p.Keywords, &p.Adjectives, &p.Exif,
		&dkey, &apos, &ppos}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return p, err
	}
	p.Rating, p.FocusTier, p.QualityScore = int16p(rating), int16p(tier), int16p(score)
	return p, nil
}

func (f PhotoFilter) where() (string, []any) {
	conds := []string{"e.published", "NOT p.hidden"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Event != "" {
		conds = append(conds, "e.slug = "+arg(f.Event))
	}
	if f.Q != "" {
		conds = append(conds, "sp.search @@ websearch_to_tsquery('english', "+arg(f.Q)+")")
	}
	if len(f.Keywords) > 0 {
		conds = append(conds, "sp.keywords @> "+arg(f.Keywords)+"::text[]")
	}
	if len(f.Ratings) > 0 || f.RatingNone {
		var parts []string
		if len(f.Ratings) > 0 {
			parts = append(parts, "sp.rating = ANY("+arg(f.Ratings)+"::int[])")
		}
		if f.RatingNone {
			parts = append(parts, "sp.rating IS NULL")
		}
		conds = append(conds, "("+strings.Join(parts, " OR ")+")")
	}
	if len(f.Tiers) > 0 {
		conds = append(conds, "sp.focus_tier = ANY("+arg(f.Tiers)+"::int[])")
	}
	if f.MinScore != nil {
		conds = append(conds, "sp.quality_score >= "+arg(*f.MinScore))
	}
	if f.KeeperOnly {
		conds = append(conds, "sp.keeper IS TRUE")
	}
	if f.Lens != "" {
		conds = append(conds, "sp.exif->>'lens' = "+arg(f.Lens))
	}
	if f.Album != "" {
		conds = append(conds, "a.name = "+arg(f.Album))
	}
	return strings.Join(conds, " AND "), args
}

// ListPublicPhotos returns one page of deduped photos and the total number of matches.
func (s *Store) ListPublicPhotos(ctx context.Context, f PhotoFilter) ([]PublicPhoto, int, error) {
	where, args := f.where()
	inner := `SELECT DISTINCT ON (e.id, ` + dedupeKey + `) ` + photoCols + ` ` + photoFrom + ` WHERE ` + where +
		` ORDER BY e.id, ` + dedupeKey + `, a.position, p.position`
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM (`+inner+`) d`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := takenOrder
	switch f.Sort {
	case "score":
		order = "quality_score DESC NULLS LAST, " + takenOrder
	case "rating":
		order = "rating DESC NULLS LAST, " + takenOrder
	}
	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT * FROM (%s) d ORDER BY %s LIMIT $%d OFFSET $%d`, inner, order, len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PublicPhoto{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// GetPublicPhoto returns ErrNotFound for unknown or hidden photos and photos of unpublished events.
// prev/next follow the event's deduped taken order.
func (s *Store) GetPublicPhoto(ctx context.Context, id int64) (PublicPhotoDetail, error) {
	row := s.Pool.QueryRow(ctx, `
		WITH me AS (
			SELECT `+photoCols+` `+photoFrom+` WHERE p.id = $1 AND e.published AND NOT p.hidden
		), ranked AS (
			SELECT dkey, lag(id) OVER w AS prev_id, lead(id) OVER w AS next_id FROM (
				SELECT DISTINCT ON (`+dedupeKey+`) `+photoCols+` `+photoFrom+`
				WHERE e.id = (SELECT event_id FROM me) AND NOT p.hidden
				ORDER BY `+dedupeKey+`, a.position, p.position
			) d WINDOW w AS (ORDER BY `+takenOrder+`)
		)
		SELECT me.*, ranked.prev_id, ranked.next_id FROM me LEFT JOIN ranked ON ranked.dkey = me.dkey`, id)
	var d PublicPhotoDetail
	p, err := scanPhoto(row, &d.PrevID, &d.NextID)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	d.PublicPhoto = p
	return d, err
}
