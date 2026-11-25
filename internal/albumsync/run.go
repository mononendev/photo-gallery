package albumsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mononendev/photo-gallery/internal/dhash"
	"github.com/mononendev/photo-gallery/internal/gphotos"
	"github.com/mononendev/photo-gallery/internal/store"
)

const maxUnmatchedItems = 200

type UnmatchedItem struct {
	Album    string `json:"album"`
	MediaKey string `json:"media_key"`
	URL      string `json:"url"`
}

// Result is stored in events.sync_result.
type Result struct {
	Albums          int             `json:"albums"`
	Items           int             `json:"items"`
	MatchedTime     int             `json:"matched_time"`
	MatchedTimeHash int             `json:"matched_time_hash"`
	MatchedHash     int             `json:"matched_hash"`
	Unmatched       int             `json:"unmatched"`
	UnmatchedItems  []UnmatchedItem `json:"unmatched_items"`
}

// Run fetches every album of the event, matches the photos to the event's sources and stores them. If any
// fetch fails it returns the error and changes no photos. The event's sync state is not touched; see
// RunStarted for that.
func Run(ctx context.Context, st *store.Store, gp *gphotos.Client, slug string) (Result, error) {
	ev, err := st.GetEvent(ctx, slug)
	if err != nil {
		return Result{}, err
	}
	fetched := make([]gphotos.Album, len(ev.Albums))
	for i, a := range ev.Albums {
		fetched[i], err = gp.FetchAlbum(ctx, a.ShareURL)
		if err != nil {
			return Result{}, fmt.Errorf("album %q: %w", a.Name, err)
		}
	}
	rows, err := st.SourcesForEvent(ctx, ev.ID)
	if err != nil {
		return Result{}, err
	}
	sources := make([]Source, len(rows))
	for i, r := range rows {
		s := Source{ID: r.ID, TakenMS: r.TakenMS, DHash: r.DHash}
		if r.Taken != nil {
			s.Taken = *r.Taken
		}
		sources[i] = s
	}
	thumbHash := func(it gphotos.Item) (uint64, error) {
		img, err := gp.Thumb(ctx, it.BaseURL)
		if err != nil {
			return 0, err
		}
		return dhash.Hash(img), nil
	}

	res := Result{Albums: len(ev.Albums)}
	for i, a := range ev.Albums {
		alb := fetched[i]
		matches, merr := MatchAlbum(alb.Items, sources, thumbHash)
		if merr != nil {
			slog.Warn("thumbnail hashing failed", "event", slug, "album", a.Name, "err", merr)
		}
		in := make([]store.PhotoInput, len(alb.Items))
		for j, it := range alb.Items {
			p := store.PhotoInput{
				MediaKey: it.MediaKey, BaseURL: it.BaseURL, Width: it.Width, Height: it.Height,
				TakenAt: time.UnixMilli(it.TakenMS).UTC(), TZOffsetMinutes: int(it.TZOffsetMS / 60000), Position: j,
			}
			res.Items++
			if m := matches[j]; m != nil {
				id, method := m.SourceID, m.Method
				p.SourcePhotoID, p.MatchMethod = &id, &method
				switch method {
				case "time":
					res.MatchedTime++
				case "time_hash":
					res.MatchedTimeHash++
				default:
					res.MatchedHash++
				}
			} else {
				res.Unmatched++
				if len(res.UnmatchedItems) < maxUnmatchedItems {
					res.UnmatchedItems = append(res.UnmatchedItems, UnmatchedItem{Album: a.Name, MediaKey: it.MediaKey, URL: it.BaseURL})
				}
			}
			in[j] = p
		}
		if err := st.ReplaceAlbumPhotos(ctx, a.ID, alb.ID, in); err != nil {
			return Result{}, fmt.Errorf("album %q: storing photos: %w", a.Name, err)
		}
	}
	if res.UnmatchedItems == nil {
		res.UnmatchedItems = []UnmatchedItem{}
	}
	return res, nil
}

// RunStarted runs a sync for an event whose state StartSync already set to running, and records the end
// state (done with the result, or failed with the error) even when ctx is cancelled.
func RunStarted(ctx context.Context, st *store.Store, gp *gphotos.Client, eventID int64, slug string) (Result, error) {
	res, err := Run(ctx, st, gp, slug)
	var raw json.RawMessage
	if err == nil {
		raw, err = json.Marshal(res)
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if ferr := st.FinishSync(fctx, eventID, raw, err); ferr != nil {
		slog.Error("recording sync end failed", "event", slug, "err", ferr)
		if err == nil {
			err = ferr
		}
	}
	return res, err
}
