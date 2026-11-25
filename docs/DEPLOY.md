# Deploying photo-gallery

## Shape

Namespace `production`, Helm release `photo-gallery` from `.ci/chart` (library chart `mononen-library-chart`).
CI deploys every push to `master`.

| Component | Image | Service | Role |
|---|---|---|---|
| `photo-gallery` | `photo-gallery-ui`, `GALLERY_UI_MODE=public` | `photo-gallery:80` | React app; proxies `/api` to the API; `/admin` and `/admin-api` answer 404 |
| `gallery-admin` | `photo-gallery-ui`, `GALLERY_UI_MODE=admin` | `photo-gallery-admin:80` | Same app; also proxies `/admin-api` to the API's admin port |
| `api` | `photo-gallery-api` | `photo-gallery-api:8080` (public API), `:8081` (admin API) | Go server, one replica |

## Hosts

| Host | Ingress | Reaches |
|---|---|---|
| `gallery.adoah.dev` | class `internal` (LAN/tailnet) | public UI |
| `gallery-admin.adoah.dev` | class `internal` | admin UI and `/admin-api` |
| `gallery.monomotion.org` | class `external`, `photo-gallery-public`, only when `publicIngress.enabled: true` in `.ci/chart/values.yaml` | public UI only |

The library chart builds hosts as `<subdomain>.<global.domain>`, so the `monomotion.org` Ingress is a custom template
(`.ci/chart/templates/ingress-public.yaml`). It points at Service `photo-gallery`, the primary component's Service,
which the library chart names after the release.

## Secrets

Both live in the homelab repo (`kubernetes/apps/production/photo-gallery/app/`), not here.

- `photo-gallery-db`, key `password`: password of Postgres role `photo_gallery` (database `photo_gallery` on
  `prd-db-cluster`). Written with `scripts/pg-rotate-password.sh prd-db-cluster photo_gallery
  kubernetes/apps/production/photo-gallery/app/secret-db.sops.yaml photo-gallery-db password production`. The API pod
  builds `GALLERY_DB` from it (`$(DB_PASSWORD)` expansion in `values.yaml`).
- `photo-gallery-harbor-pull`: pull secret for `registry.adoah.dev`.

CI needs no secrets of its own beyond what the `homelab` runners already carry (buildkit client certificates in
`BUILDKIT_CLIENT_*`, cluster access for `helm`). The `test-go` job installs `postgresql-16` from apt and runs the
tests against that throwaway server; the runner pods have no Docker daemon for a `services:` container.

## Adding an event

Filled in by the real-data run (plan step 12).

## Troubleshooting

`sync_error` on an event (admin page or `GET /admin-api/events/<slug>/sync`) comes from the album sync. Filled in
once the sync exists (plan step 6); album-page parse errors start with `album page format changed`.
