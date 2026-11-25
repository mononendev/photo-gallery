package albumsync

import (
	"errors"
	"testing"
	"time"

	"github.com/mononendev/photo-gallery/internal/gphotos"
)

func item(key string, local string, ms int64) gphotos.Item {
	t, err := time.Parse("2006:01:02 15:04:05", local)
	if err != nil {
		panic(err)
	}
	return gphotos.Item{MediaKey: key, TakenMS: t.UnixMilli() + ms - 3600_000, TZOffsetMS: 3600_000}
}

func u64(v uint64) *uint64 { return &v }
func ip(v int) *int        { return &v }

func TestMatchAlbum(t *testing.T) {
	const sec = "2025:04:23 09:03:23"
	noHash := func(gphotos.Item) (uint64, error) { t.Error("thumbHash called"); return 0, nil }
	type want struct {
		id     int64
		method string
	}
	tests := []struct {
		name    string
		items   []gphotos.Item
		sources []Source
		hash    func(gphotos.Item) (uint64, error)
		want    []want // id 0 = unmatched
	}{
		{"single same-second source", []gphotos.Item{item("a", sec, 0)}, []Source{{ID: 1, Taken: sec}}, noHash, []want{{1, "time"}}},
		{"three same-second sources, hash picks the second",
			[]gphotos.Item{item("a", sec, 0)},
			[]Source{{ID: 1, Taken: sec, DHash: u64(0xFFFF)}, {ID: 2, Taken: sec, DHash: u64(0x1)}, {ID: 3, Taken: sec, DHash: u64(0xFF00FF00FF00FF00)}},
			func(gphotos.Item) (uint64, error) { return 0x3, nil }, []want{{2, "time_hash"}}},
		{"hash tie goes to the lower id, no-hash ranks last",
			[]gphotos.Item{item("a", sec, 0)},
			[]Source{{ID: 9, Taken: sec}, {ID: 5, Taken: sec, DHash: u64(0)}, {ID: 4, Taken: sec, DHash: u64(0)}},
			func(gphotos.Item) (uint64, error) { return 0, nil }, []want{{4, "time_hash"}}},
		{"no taken, distance 8",
			[]gphotos.Item{item("a", sec, 0)},
			[]Source{{ID: 1, DHash: u64(0xFF)}},
			func(gphotos.Item) (uint64, error) { return 0, nil }, []want{{1, "hash"}}},
		{"distance 11 stays unmatched",
			[]gphotos.Item{item("a", sec, 0)},
			[]Source{{ID: 1, DHash: u64(0x7FF)}},
			func(gphotos.Item) (uint64, error) { return 0, nil }, []want{{0, ""}}},
		{"two items same second, one source",
			[]gphotos.Item{item("a", sec, 0), item("b", sec, 0)},
			[]Source{{ID: 1, Taken: sec}},
			func(gphotos.Item) (uint64, error) { return 0, errors.New("boom") }, []want{{1, "time"}, {0, ""}}},
		{"hundredths filter picks the burst frame",
			[]gphotos.Item{item("a", sec, 940)},
			[]Source{{ID: 1, Taken: sec, TakenMS: ip(760)}, {ID: 2, Taken: sec, TakenMS: ip(940)}, {ID: 3, Taken: sec, TakenMS: ip(840)}},
			noHash, []want{{2, "time"}}},
		{"hundredths filter drops all candidates, no hashes: unmatched",
			[]gphotos.Item{item("a", sec, 500)},
			[]Source{{ID: 1, Taken: sec, TakenMS: ip(760)}},
			noHash, []want{{0, ""}}},
		{"album drops the sub-second's leading zero: \"04\" (40 ms) shows as 400 ms",
			[]gphotos.Item{item("a", sec, 400)},
			[]Source{{ID: 1, Taken: sec, TakenMS: ip(940)}, {ID: 2, Taken: sec, TakenMS: ip(40)}, {ID: 3, Taken: sec, TakenMS: ip(140)}},
			noHash, []want{{2, "time"}}},
		{"\"40\" and \"04\" in one second both fit 400 ms: hash decides",
			[]gphotos.Item{item("a", sec, 400)},
			[]Source{{ID: 1, Taken: sec, TakenMS: ip(400), DHash: u64(0xFF)}, {ID: 2, Taken: sec, TakenMS: ip(40), DHash: u64(0)}},
			func(gphotos.Item) (uint64, error) { return 0, nil }, []want{{2, "time_hash"}}},
		{"items processed in time order",
			[]gphotos.Item{item("late", sec, 900), item("early", sec, 100)},
			[]Source{{ID: 1, Taken: sec}},
			noHash, []want{{0, ""}, {1, "time"}}},
		{"hash failure leaves the item unmatched",
			[]gphotos.Item{item("a", sec, 0)},
			[]Source{{ID: 1, Taken: sec}, {ID: 2, Taken: sec}},
			func(gphotos.Item) (uint64, error) { return 0, errors.New("boom") }, []want{{0, ""}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := MatchAlbum(tc.items, tc.sources, tc.hash)
			if len(got) != len(tc.want) {
				t.Fatalf("len %d, want %d", len(got), len(tc.want))
			}
			for i, w := range tc.want {
				switch {
				case w.id == 0 && got[i] != nil:
					t.Errorf("item %d: got %+v, want unmatched", i, *got[i])
				case w.id != 0 && (got[i] == nil || got[i].SourceID != w.id || got[i].Method != w.method):
					t.Errorf("item %d: got %+v, want %+v", i, got[i], w)
				}
			}
		})
	}
}

func TestMatchAlbumHashCalledOnce(t *testing.T) {
	calls := 0
	items := []gphotos.Item{item("a", "2025:04:23 09:03:23", 0)}
	_, err := MatchAlbum(items, []Source{{ID: 1, Taken: "2025:04:23 09:03:23"}, {ID: 2, Taken: "2025:04:23 09:03:23"}},
		func(gphotos.Item) (uint64, error) { calls++; return 0, nil })
	if err != nil || calls != 1 {
		t.Fatalf("calls %d err %v", calls, err)
	}
}
