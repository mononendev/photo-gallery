// Package gphotos reads public Google Photos shared albums by scraping the share page and the
// batchexecute paging endpoint. The format is undocumented; see docs/DEVLOG.md for the observed shape.
package gphotos

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const (
	defaultBatchURL = "https://photos.google.com/_/PhotosUi/data/batchexecute"
	userAgent       = "Mozilla/5.0"
	maxPages        = 100
	maxBody         = 64 << 20
)

type Item struct {
	MediaKey   string
	BaseURL    string
	Width      int
	Height     int
	TakenMS    int64 // UTC milliseconds
	TZOffsetMS int64
}

type Album struct {
	ID    string
	Title string
	Count int
	Items []Item
}

type Client struct {
	HTTP     *http.Client
	BatchURL string // default https://photos.google.com/_/PhotosUi/data/batchexecute
}

var dsRe = regexp.MustCompile(`(?s)AF_initDataCallback\(\{key: 'ds:1'.*?data:(.*?), sideChannel`)

func (c *Client) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) batchURL() string {
	if c.BatchURL != "" {
		return c.BatchURL
	}
	return defaultBatchURL
}

func readBody(resp *http.Response) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBody {
		return nil, fmt.Errorf("response larger than 64 MiB")
	}
	return b, nil
}

// number decodes a JSON number and rejects null, which encoding/json would accept silently.
func number[T int | int64](r json.RawMessage, out *T) error {
	if string(r) == "null" {
		return fmt.Errorf("null")
	}
	return json.Unmarshal(r, out)
}

// FetchAlbum resolves a share link and returns every item of the album.
func (c *Client) FetchAlbum(ctx context.Context, shareURL string) (Album, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shareURL, nil)
	if err != nil {
		return Album{}, fmt.Errorf("album page: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpc().Do(req)
	if err != nil {
		return Album{}, fmt.Errorf("album page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Album{}, fmt.Errorf("album page: HTTP %d", resp.StatusCode)
	}
	final := resp.Request.URL
	albumID := path.Base(final.Path)
	authKey := final.Query().Get("key")
	if albumID == "" || albumID == "." || albumID == "/" || authKey == "" {
		return Album{}, fmt.Errorf("album link did not resolve to a shared album: %s", final)
	}
	body, err := readBody(resp)
	if err != nil {
		return Album{}, fmt.Errorf("album page: %w", err)
	}
	m := dsRe.FindSubmatch(body)
	if m == nil {
		return Album{}, fmt.Errorf("album page format changed: ds:1 block not found")
	}
	var d []json.RawMessage
	if err := json.Unmarshal(m[1], &d); err != nil {
		return Album{}, fmt.Errorf("album page format changed: ds:1 data: %w", err)
	}
	if len(d) < 4 {
		return Album{}, fmt.Errorf("album page format changed: ds:1 data has %d elements, want at least 4", len(d))
	}
	items, token, err := parsePage(d)
	if err != nil {
		return Album{}, err
	}
	var meta []json.RawMessage
	if err := json.Unmarshal(d[3], &meta); err != nil || len(meta) < 22 {
		return Album{}, fmt.Errorf("album page format changed: d[3] is not an album info array")
	}
	alb := Album{ID: albumID}
	if err := json.Unmarshal(meta[1], &alb.Title); err != nil {
		return Album{}, fmt.Errorf("album page format changed: d[3][1] expected string title")
	}
	if err := number(meta[21], &alb.Count); err != nil {
		return Album{}, fmt.Errorf("album page format changed: d[3][21] expected item count")
	}
	alb.Items = items
	for page := 2; token != ""; page++ {
		if page > maxPages {
			return Album{}, fmt.Errorf("batchexecute: more than %d pages", maxPages)
		}
		more, next, err := c.fetchPage(ctx, albumID, token, authKey, page)
		if err != nil {
			return Album{}, err
		}
		alb.Items = append(alb.Items, more...)
		token = next
	}
	return alb, nil
}

func (c *Client) fetchPage(ctx context.Context, albumID, token, authKey string, page int) ([]Item, string, error) {
	inner, _ := json.Marshal([]any{albumID, token, nil, authKey})
	freq, _ := json.Marshal([]any{[]any{[]any{"snAcKc", string(inner), nil, "generic"}}})
	form := url.Values{"f.req": {string(freq)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.batchURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: %w", page, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpc().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: %w", page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("batchexecute page %d: HTTP %d", page, resp.StatusCode)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: %w", page, err)
	}
	var line string
	for _, l := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(l, `[["wrb.fr"`) {
			line = l
			break
		}
	}
	if line == "" {
		return nil, "", fmt.Errorf("batchexecute page %d: format changed: wrb.fr line not found", page)
	}
	var outer [][]json.RawMessage
	if err := json.Unmarshal([]byte(line), &outer); err != nil || len(outer) == 0 || len(outer[0]) < 3 {
		return nil, "", fmt.Errorf("batchexecute page %d: format changed: bad wrb.fr envelope", page)
	}
	var payload string
	if err := json.Unmarshal(outer[0][2], &payload); err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: format changed: outer[0][2] expected string", page)
	}
	var d []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: format changed: payload: %w", page, err)
	}
	items, next, err := parsePage(d)
	if err != nil {
		return nil, "", fmt.Errorf("batchexecute page %d: %w", page, err)
	}
	return items, next, nil
}

// parsePage reads items at d[1] and the next-page token at d[2].
func parsePage(d []json.RawMessage) ([]Item, string, error) {
	if len(d) < 3 {
		return nil, "", fmt.Errorf("album page format changed: page has %d elements, want at least 3", len(d))
	}
	var rawItems []json.RawMessage
	if string(d[1]) != "null" {
		if err := json.Unmarshal(d[1], &rawItems); err != nil {
			return nil, "", fmt.Errorf("album page format changed: d[1] expected item list")
		}
	}
	items := make([]Item, 0, len(rawItems))
	for i, r := range rawItems {
		it, err := parseItem(r)
		if err != nil {
			return nil, "", fmt.Errorf("album page format changed: item %d: %w", i, err)
		}
		items = append(items, it)
	}
	var token string
	if string(d[2]) != "null" {
		if err := json.Unmarshal(d[2], &token); err != nil {
			return nil, "", fmt.Errorf("album page format changed: d[2] expected string token")
		}
	}
	return items, token, nil
}

func parseItem(r json.RawMessage) (Item, error) {
	var it []json.RawMessage
	if err := json.Unmarshal(r, &it); err != nil || len(it) < 5 {
		return Item{}, fmt.Errorf("expected array of at least 5 elements")
	}
	var out Item
	if err := json.Unmarshal(it[0], &out.MediaKey); err != nil || out.MediaKey == "" {
		return Item{}, fmt.Errorf("expected [0] string")
	}
	var info []json.RawMessage
	if err := json.Unmarshal(it[1], &info); err != nil || len(info) < 3 {
		return Item{}, fmt.Errorf("expected [1] array")
	}
	if err := json.Unmarshal(info[0], &out.BaseURL); err != nil || out.BaseURL == "" {
		return Item{}, fmt.Errorf("expected [1][0] string")
	}
	if err := number(info[1], &out.Width); err != nil {
		return Item{}, fmt.Errorf("expected [1][1] number")
	}
	if err := number(info[2], &out.Height); err != nil {
		return Item{}, fmt.Errorf("expected [1][2] number")
	}
	if err := number(it[2], &out.TakenMS); err != nil {
		return Item{}, fmt.Errorf("expected [2] number")
	}
	if err := number(it[4], &out.TZOffsetMS); err != nil {
		return Item{}, fmt.Errorf("expected [4] number")
	}
	return out, nil
}

// Thumb downloads the 400 px wide JPEG of an item.
func (c *Client) Thumb(ctx context.Context, baseURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"=w400", nil)
	if err != nil {
		return nil, fmt.Errorf("thumb: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpc().Do(req)
	if err != nil {
		return nil, fmt.Errorf("thumb: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumb: HTTP %d", resp.StatusCode)
	}
	img, err := jpeg.Decode(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("thumb: decode: %w", err)
	}
	return img, nil
}
