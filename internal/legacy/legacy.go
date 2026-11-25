// Package legacy holds the events of the old Next.js gallery (mononen/photo-gallery, content/events/*.md),
// converted by hand to JSON, so seed-legacy can recreate them here.
package legacy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mononendev/photo-gallery/internal/store"
)

//go:embed events.json
var eventsJSON []byte

type album struct {
	Name     string `json:"name"`
	ShareURL string `json:"share_url"`
}

type event struct {
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Series      string  `json:"series"`
	EventDate   string  `json:"event_date"`
	Description string  `json:"description"`
	Albums      []album `json:"albums"`
}

// Events returns the old gallery's events as published event inputs, in file order.
func Events() ([]store.EventInput, error) {
	var raw []event
	if err := json.Unmarshal(eventsJSON, &raw); err != nil {
		return nil, fmt.Errorf("legacy events.json: %w", err)
	}
	out := make([]store.EventInput, 0, len(raw))
	for _, e := range raw {
		date, err := time.Parse(time.DateOnly, e.EventDate)
		if err != nil {
			return nil, fmt.Errorf("legacy event %s: event_date: %w", e.Slug, err)
		}
		in := store.EventInput{
			Slug:        e.Slug,
			Title:       e.Title,
			EventDate:   date,
			Description: e.Description,
			Published:   true,
		}
		if e.Series != "" {
			in.Series = &e.Series
		}
		for _, a := range e.Albums {
			in.Albums = append(in.Albums, store.AlbumInput{Name: a.Name, ShareURL: a.ShareURL})
		}
		out = append(out, in)
	}
	return out, nil
}
