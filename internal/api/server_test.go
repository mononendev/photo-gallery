package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthOnBothMuxes(t *testing.T) {
	// Nothing listens on port 1: the pool is created lazily, so only the ping fails.
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Server{Pool: pool, Version: "test"}
	for name, h := range map[string]http.Handler{"public": s.Public(), "admin": s.Admin()} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
		var got struct {
			OK       bool   `json:"ok"`
			Version  string `json:"version"`
			Database bool   `json:"database"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !got.OK || got.Version != "test" || got.Database {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}
