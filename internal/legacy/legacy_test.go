package legacy

import (
	"regexp"
	"strings"
	"testing"
)

func TestEventsDecode(t *testing.T) {
	events, err := Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("got %d events, want 6", len(events))
	}
	slugRe := regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	seen := map[string]bool{}
	for _, e := range events {
		if !slugRe.MatchString(e.Slug) || seen[e.Slug] {
			t.Errorf("bad or repeated slug %q", e.Slug)
		}
		seen[e.Slug] = true
		if e.Title == "" || e.EventDate.IsZero() || !e.Published {
			t.Errorf("%s: title %q, date %v, published %v", e.Slug, e.Title, e.EventDate, e.Published)
		}
		if len(e.Albums) == 0 {
			t.Errorf("%s: no albums", e.Slug)
		}
		for _, a := range e.Albums {
			if a.Name == "" || !strings.HasPrefix(a.ShareURL, "https://photos.app.goo.gl/") {
				t.Errorf("%s: bad album %+v", e.Slug, a)
			}
		}
	}
}
