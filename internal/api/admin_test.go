package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mononendev/photo-gallery/internal/db/dbtest"
	"github.com/mononendev/photo-gallery/internal/gphotos"
)

// rewriteTransport sends every request to the fake server, so share URLs can keep their real prefix.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func fakeGoogle(t *testing.T) *gphotos.Client {
	t.Helper()
	html, err := os.ReadFile("../gphotos/testdata/album.html")
	if err != nil {
		t.Fatal(err)
	}
	page2, err := os.ReadFile("../gphotos/testdata/page2.txt")
	if err != nil {
		t.Fatal(err)
	}
	var jpg bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for x := 0; x < 40; x++ {
		for y := 0; y < 30; y++ {
			img.Set(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	jpeg.Encode(&jpg, img, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/abc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/share/ALBUM?key=K", http.StatusFound)
	})
	mux.HandleFunc("/share/ALBUM", func(w http.ResponseWriter, r *http.Request) { w.Write(html) })
	mux.HandleFunc("/batch", func(w http.ResponseWriter, r *http.Request) { w.Write(page2) })
	mux.HandleFunc("/pw/", func(w http.ResponseWriter, r *http.Request) { w.Write(jpg.Bytes()) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return &gphotos.Client{HTTP: &http.Client{Transport: rewriteTransport{u}}, BatchURL: "https://photos.google.com/batch"}
}

func call(t *testing.T, h http.Handler, method, path, body string, want int, out any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	res, _ := io.ReadAll(rec.Body)
	if rec.Code != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, rec.Code, want, res)
	}
	if out != nil {
		if err := json.Unmarshal(res, out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, res)
		}
	}
}

func detail(t *testing.T, h http.Handler, method, path, body string, want int) string {
	t.Helper()
	var d struct {
		Detail string `json:"detail"`
	}
	call(t, h, method, path, body, want, &d)
	return d.Detail
}

func TestAdminSyncFlow(t *testing.T) {
	pool := dbtest.New(t)
	s := &Server{Pool: pool, Version: "test", GPhotos: fakeGoogle(t)}
	h := s.Admin()

	put := `{"title":"Test","series":"S","event_date":"2025-04-23","description":"d","published":false,
		"albums":[{"name":"All","share_url":"https://photos.app.goo.gl/abc"}]}`
	call(t, h, "PUT", "/admin-api/events/test-ev", put, 200, nil)

	// K3 local second is 05:03:30. Both dhashes are far from the fake thumbnail (hash 0), so only time can match.
	sources := `{"photos":[
		{"source_id":1,"rel_path":"RAW/a.CR2","taken":"2025:04:23 05:03:30","dhash":"ffffffffffffffff","rating":3,"keywords":["x"]},
		{"source_id":2,"rel_path":"RAW/b.CR2","dhash":"ffffffffffffffff"}]}`
	var up struct{ Upserted int }
	call(t, h, "POST", "/admin-api/events/test-ev/sources", sources, 200, &up)
	if up.Upserted != 2 {
		t.Fatalf("upserted %d", up.Upserted)
	}

	var started struct {
		SyncState string `json:"sync_state"`
	}
	call(t, h, "POST", "/admin-api/events/test-ev/sync", "", 202, &started)
	if started.SyncState != "running" {
		t.Fatalf("state %q", started.SyncState)
	}
	var st struct {
		SyncState  string  `json:"sync_state"`
		SyncError  *string `json:"sync_error"`
		SyncResult struct {
			Albums, Items, Unmatched int
			MatchedTime              int `json:"matched_time"`
			UnmatchedItems           []struct {
				MediaKey string `json:"media_key"`
			} `json:"unmatched_items"`
		} `json:"sync_result"`
	}
	for i := 0; ; i++ {
		call(t, h, "GET", "/admin-api/events/test-ev/sync", "", 200, &st)
		if st.SyncState != "running" {
			break
		}
		if i > 100 {
			t.Fatal("sync did not finish")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st.SyncState != "done" {
		t.Fatalf("sync %s: %v", st.SyncState, st.SyncError)
	}
	if r := st.SyncResult; r.Albums != 1 || r.Items != 4 || r.MatchedTime != 1 || r.Unmatched != 3 || len(r.UnmatchedItems) != 3 {
		t.Fatalf("result %+v", r)
	}

	var page struct {
		Items []struct {
			ID          int64
			Album       string
			MatchMethod *string `json:"match_method"`
			SourceID    *int64  `json:"source_id"`
			RelPath     *string `json:"rel_path"`
			TakenAt     string  `json:"taken_at"`
		}
		Total int
	}
	call(t, h, "GET", "/admin-api/events/test-ev/photos", "", 200, &page)
	if page.Total != 4 || len(page.Items) != 4 {
		t.Fatalf("photos %+v", page)
	}
	var matched int
	for _, p := range page.Items {
		if p.MatchMethod != nil {
			matched++
			if *p.MatchMethod != "time" || *p.SourceID != 1 || *p.RelPath != "RAW/a.CR2" {
				t.Errorf("match %+v", p)
			}
		}
	}
	if matched != 1 {
		t.Errorf("matched photos %d", matched)
	}
	call(t, h, "GET", "/admin-api/events/test-ev/photos?offset=3&limit=1", "", 200, &page)
	if len(page.Items) != 1 || page.Total != 4 {
		t.Errorf("paged %+v", page)
	}

	call(t, h, "PATCH", fmt.Sprintf("/admin-api/photos/%d", page.Items[0].ID), `{"hidden":true}`, 204, nil)
	call(t, h, "PATCH", "/admin-api/photos/999999", `{"hidden":true}`, 404, nil)

	var evs []struct {
		Slug                     string
		Photos, Matched, Sources int
		SyncState                string `json:"sync_state"`
	}
	call(t, h, "GET", "/admin-api/events", "", 200, &evs)
	if len(evs) != 1 || evs[0].Photos != 4 || evs[0].Matched != 1 || evs[0].Sources != 2 || evs[0].SyncState != "done" {
		t.Errorf("events %+v", evs)
	}

	// A second sync is allowed after the first finished and is idempotent.
	call(t, h, "POST", "/admin-api/events/test-ev/sync", "", 202, nil)

	call(t, h, "DELETE", "/admin-api/events/test-ev", "", 204, nil)
	call(t, h, "DELETE", "/admin-api/events/test-ev", "", 404, nil)
}

func TestAdminErrors(t *testing.T) {
	pool := dbtest.New(t)
	h := (&Server{Pool: pool, GPhotos: &gphotos.Client{}}).Admin()
	ok := `{"title":"T","event_date":"2025-04-23","albums":[]}`
	call(t, h, "PUT", "/admin-api/events/Bad_Slug", ok, 400, nil)
	call(t, h, "PUT", "/admin-api/events/e1", `{"title":" ","event_date":"2025-04-23"}`, 400, nil)
	call(t, h, "PUT", "/admin-api/events/e1", `{"title":"T","event_date":"23.04.2025"}`, 400, nil)
	call(t, h, "PUT", "/admin-api/events/e1", `{"title":"T","event_date":"2025-04-23","albums":[{"name":"a","share_url":"http://evil/x"}]}`, 400, nil)
	call(t, h, "PUT", "/admin-api/events/e1", ok, 200, nil)

	if d := detail(t, h, "POST", "/admin-api/events/nope/sources", `{"photos":[]}`, 404); d != "event nope not found" {
		t.Errorf("detail %q", d)
	}
	call(t, h, "POST", "/admin-api/events/nope/sync", "", 404, nil)
	call(t, h, "GET", "/admin-api/events/nope/sync", "", 404, nil)
	call(t, h, "GET", "/admin-api/events/nope/photos", "", 404, nil)

	var big strings.Builder
	big.WriteString(`{"photos":[`)
	for i := 0; i < 501; i++ {
		if i > 0 {
			big.WriteString(",")
		}
		fmt.Fprintf(&big, `{"source_id":%d,"rel_path":"a"}`, i+1)
	}
	big.WriteString(`]}`)
	if d := detail(t, h, "POST", "/admin-api/events/e1/sources", big.String(), 413); d != "at most 500 photos per request" {
		t.Errorf("detail %q", d)
	}
	call(t, h, "POST", "/admin-api/events/e1/sources", `{"photos":[{"source_id":1,"rel_path":"a","dhash":"zz"}]}`, 400, nil)

	// Missing keywords/adjectives and a null dhash are fine.
	call(t, h, "POST", "/admin-api/events/e1/sources", `{"photos":[{"source_id":1,"rel_path":"a","dhash":null}]}`, 200, nil)
	var kw []string
	if err := pool.QueryRow(t.Context(), `SELECT keywords FROM source_photos`).Scan(&kw); err != nil || kw == nil || len(kw) != 0 {
		t.Errorf("keywords %v err %v", kw, err)
	}

	// A running sync blocks a second start.
	if _, err := pool.Exec(t.Context(), `UPDATE events SET sync_state='running'`); err != nil {
		t.Fatal(err)
	}
	if d := detail(t, h, "POST", "/admin-api/events/e1/sync", "", 409); d != "sync already running" {
		t.Errorf("detail %q", d)
	}
}
