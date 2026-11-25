// Package albumsync links the photos of Google Photos albums to photosort rows and stores the result.
package albumsync

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mononendev/photo-gallery/internal/dhash"
	"github.com/mononendev/photo-gallery/internal/gphotos"
)

// maxHashDistance is the largest dHash distance that still counts as the same picture when no capture
// time matches.
const maxHashDistance = 10

// Source is one photosort row as the matcher sees it. ID is source_photos.id.
type Source struct {
	ID      int64
	Taken   string  // EXIF DateTimeOriginal "YYYY:MM:DD HH:MM:SS"; empty when unknown
	TakenMS *int    // sub-second of the capture time, from SubSecTimeOriginal
	DHash   *uint64 // dHash of the photosort thumbnail
}

type Match struct {
	SourceID int64
	Method   string // "time", "time_hash" or "hash"
}

// MatchAlbum returns one entry per item; nil means unmatched. A source matches at most one item. thumbHash is
// called at most once per item and only when the capture time is not decisive; when it fails the item stays
// unmatched. The returned error joins those failures; the matches are valid even when it is non-nil.
func MatchAlbum(items []gphotos.Item, sources []Source, thumbHash func(gphotos.Item) (uint64, error)) ([]*Match, error) {
	out := make([]*Match, len(items))
	byTaken := map[string][]int{}
	anyHash := false
	for i, s := range sources {
		if s.Taken != "" {
			byTaken[s.Taken] = append(byTaken[s.Taken], i)
		}
		if s.DHash != nil {
			anyHash = true
		}
	}
	used := make([]bool, len(sources))

	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	local := func(i int) int64 { return items[i].TakenMS + items[i].TZOffsetMS }
	sort.SliceStable(order, func(a, b int) bool { return local(order[a]) < local(order[b]) })

	var errs []error
	for _, idx := range order {
		it := items[idx]
		ms := local(idx)
		key := time.UnixMilli(ms).UTC().Format("2006:01:02 15:04:05")
		sub := int((ms%1000 + 1000) % 1000)

		var cands []int
		for _, si := range byTaken[key] {
			if !used[si] {
				cands = append(cands, si)
			}
		}
		withMS := false
		for _, si := range cands {
			if sources[si].TakenMS != nil {
				withMS = true
				break
			}
		}
		if withMS {
			kept := cands[:0:0]
			for _, si := range cands {
				if t := sources[si].TakenMS; t != nil && *t/10 == sub/10 {
					kept = append(kept, si)
				}
			}
			cands = kept
		}

		if len(cands) == 1 {
			used[cands[0]] = true
			out[idx] = &Match{SourceID: sources[cands[0]].ID, Method: "time"}
			continue
		}
		if len(cands) == 0 && !anyHash {
			continue
		}
		h, err := thumbHash(it)
		if err != nil {
			errs = append(errs, fmt.Errorf("item %s: %w", it.MediaKey, err))
			continue
		}
		if len(cands) > 1 {
			if best := nearest(sources, cands, h); best >= 0 {
				used[best] = true
				out[idx] = &Match{SourceID: sources[best].ID, Method: "time_hash"}
			}
			continue
		}
		var pool []int
		for si, s := range sources {
			if !used[si] && s.DHash != nil {
				pool = append(pool, si)
			}
		}
		if best := nearest(sources, pool, h); best >= 0 && dhash.Distance(h, *sources[best].DHash) <= maxHashDistance {
			used[best] = true
			out[idx] = &Match{SourceID: sources[best].ID, Method: "hash"}
		}
	}
	return out, errors.Join(errs...)
}

// nearest returns the candidate with the smallest distance to h; candidates without a hash rank last and
// ties go to the lower ID. It returns -1 for an empty list.
func nearest(sources []Source, cands []int, h uint64) int {
	best, bestDist := -1, 1<<30
	for _, si := range cands {
		d := 1 << 29
		if sources[si].DHash != nil {
			d = dhash.Distance(h, *sources[si].DHash)
		}
		if best < 0 || d < bestDist || (d == bestDist && sources[si].ID < sources[best].ID) {
			best, bestDist = si, d
		}
	}
	return best
}
