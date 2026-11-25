# photo-gallery

A public web gallery for event photos. The images stay in public Google Photos albums (Lightroom exports); the
gallery scrapes the album pages and links each album photo to the matching [photosort](https://github.com/mononendev/photosort)
row, so photos show photosort's ratings, focus tiers, descriptions, keywords and EXIF, and can be searched by tag.

Go (`net/http`, cobra, pgx) API on Postgres, React 19 + Vite + Tailwind UI. Two HTTP servers: the public read-only
API on `:8080` and the admin API on `:8081` (no login, LAN and in-cluster only).

## Run locally

```sh
make db     # Postgres on :5433 (Docker)
make dev    # API: public :8080, admin :8081
make web    # Vite dev server on :5173, proxies /api and /admin-api
```

`GALLERY_DB` is the Postgres URL (default for `make dev`: the `make db` container). Run the tests with
`GALLERY_TEST_PG='postgres://postgres:postgres@localhost:5433/gallery?sslmode=disable' make test`; database tests skip
when `GALLERY_TEST_PG` is unset.

## Docs

- [docs/PLAN.md](docs/PLAN.md): the v1 plan
- [docs/DEPLOY.md](docs/DEPLOY.md): deployment, secrets, per-event runbook
- [docs/DEVLOG.md](docs/DEVLOG.md): decisions and measurements

License: AGPL-3.0.
