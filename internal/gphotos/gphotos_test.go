package gphotos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFetchAlbum(t *testing.T) {
	html, err := os.ReadFile("testdata/album.html")
	if err != nil {
		t.Fatal(err)
	}
	page2, err := os.ReadFile("testdata/page2.txt")
	if err != nil {
		t.Fatal(err)
	}
	var gotReq string
	mux := http.NewServeMux()
	mux.HandleFunc("/s/abc", func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "Mozilla/5.0" {
			t.Errorf("user agent %q", r.UserAgent())
		}
		http.Redirect(w, r, "/share/ALBUM?key=K", http.StatusFound)
	})
	mux.HandleFunc("/share/ALBUM", func(w http.ResponseWriter, r *http.Request) { w.Write(html) })
	mux.HandleFunc("/batch", func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded;charset=UTF-8" {
			t.Errorf("content type %q", ct)
		}
		r.ParseForm()
		gotReq = r.PostForm.Get("f.req")
		w.Write(page2)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), BatchURL: srv.URL + "/batch"}
	alb, err := c.FetchAlbum(context.Background(), srv.URL+"/s/abc")
	if err != nil {
		t.Fatal(err)
	}
	if alb.ID != "ALBUM" || alb.Title != "Test Album" || alb.Count != 4 || len(alb.Items) != 4 {
		t.Fatalf("album: %+v", alb)
	}
	loc := func(it Item) string {
		return time.UnixMilli(it.TakenMS + it.TZOffsetMS).UTC().Format("2006-01-02 15:04:05.000")
	}
	for i, w := range []string{"2025-04-23 05:03:23.420", "2025-04-23 05:03:23.420", "2025-04-23 05:03:30.000", "2025-04-23 04:03:40.000"} {
		if got := loc(alb.Items[i]); got != w {
			t.Errorf("item %d local time %s, want %s", i, got, w)
		}
	}
	if it := alb.Items[0]; it.MediaKey != "K1" || it.Width != 4000 || it.Height != 3000 || !strings.HasSuffix(it.BaseURL, "/K1") {
		t.Errorf("item 0: %+v", it)
	}
	if alb.Items[3].MediaKey != "K4" {
		t.Errorf("item 3: %+v", alb.Items[3])
	}
	for _, s := range []string{"snAcKc", "ALBUM", "TOKEN2", `"K"`} {
		if !strings.Contains(gotReq, strings.ReplaceAll(s, `"`, `\"`)) {
			t.Errorf("f.req %q missing %q", gotReq, s)
		}
	}
}

func TestFetchAlbumErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hi")) })
	mux.HandleFunc("/share/A", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html></html>")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	_, err := c.FetchAlbum(context.Background(), srv.URL+"/plain")
	if err == nil || !strings.Contains(err.Error(), "did not resolve to a shared album") {
		t.Errorf("err = %v", err)
	}
	_, err = c.FetchAlbum(context.Background(), srv.URL+"/share/A?key=K")
	if err == nil || !strings.Contains(err.Error(), "ds:1 block not found") {
		t.Errorf("err = %v", err)
	}
}

func TestLiveAlbum(t *testing.T) {
	u := os.Getenv("GALLERY_LIVE_ALBUM_URL")
	if u == "" {
		t.Skip("GALLERY_LIVE_ALBUM_URL not set")
	}
	c := &Client{HTTP: &http.Client{Timeout: 60 * time.Second}}
	alb, err := c.FetchAlbum(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("album %q id=%s count=%d items=%d", alb.Title, alb.ID, alb.Count, len(alb.Items))
	if len(alb.Items) != alb.Count {
		t.Fatalf("items %d != count %d", len(alb.Items), alb.Count)
	}
	img, err := c.Thumb(context.Background(), alb.Items[0].BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("thumb %v", img.Bounds())
}

func TestParseItemNull(t *testing.T) {
	good := `["K",["https://x/K",1,2],1000,null,0]`
	if _, err := parseItem([]byte(good)); err != nil {
		t.Fatal(err)
	}
	bad := `["K",["https://x/K",1,2],null,null,0]`
	_, err := parseItem([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "expected [2] number") {
		t.Errorf("err = %v", err)
	}
}
