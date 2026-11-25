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

1. Upload the Lightroom exports to a Google Photos album (they must keep EXIF `DateTimeOriginal`; keep
   `SubSecTimeOriginal` too, it tells burst frames apart). Share the album and copy the public link
   (`https://photos.app.goo.gl/...`).
2. At `https://gallery-admin.adoah.dev/admin`, create the event (slug, title, date, series) and paste the album
   link(s). Leave Published off.
3. Send photosort's metadata and run the album sync from the photosort-api pod:

   ```bash
   kubectl -n production exec deploy/photosort-api -c photosort-api -- \
     photosort publish --event <slug> --folder "RAW/<event folder>"
   ```

   It prints `sent N photos to <slug>` and then
   `synced <slug>: <items> album photos, <n> matched by time, <n> by time+hash, <n> by hash, <n> unmatched`.
   Without photosort data, press "Sync albums" on the event page instead (or
   `kubectl -n production exec deploy/photo-gallery-api -- photo-gallery sync --event <slug>`).
4. Check the sync panel on the event page (counts, unmatched thumbnails). In the Photos grid below it, check a few
   `time_hash` matches: hover shows the photosort `rel_path`; compare the image with that file in photosort (or with
   `https://photosort.adoah.dev/media/thumb/<source_id>`, `source_id` from
   `GET /admin-api/events/<slug>/photos`). Hide photos that should not be public.
5. Set Published and save. The event appears on `gallery.adoah.dev` (and on `gallery.monomotion.org` once
   `publicIngress.enabled` is true) within a minute; public responses are cached for 60 s.

Re-running `publish` or a sync is safe: sources upsert by photosort id and album photos upsert by media key, so
hidden flags survive. Changing an album's share link replaces that album: its photos are deleted and synced again,
and hidden flags in it must be set again.

## Troubleshooting

`sync_error` on an event (admin page or `GET /admin-api/events/<slug>/sync`) comes from the album sync. Every album
is fetched before anything is written, so a failed fetch changes no photos. An error while storing (database error,
shutdown) can leave the albums stored before it updated; run the sync again.

- `album "<name>": album page: HTTP 404`: the share link no longer works (album unshared or deleted). Re-share it
  and update the link, or remove the album from the event. OCSF5 lost two albums this way.
- `album link did not resolve to a shared album: <url>`: the link does not redirect to
  `photos.google.com/share/<id>?key=...`; copy the public share link again.
- `album page format changed: ...` or `batchexecute page N: ...` with a parse error: Google changed the page.
  Probe the album again with the procedure in `docs/DEVLOG.md` ("Album page format"), update `internal/gphotos`
  and its fixtures, and record the new format in the DEVLOG. `batchexecute page N: HTTP 429` is rate limiting;
  retry later.
- `... context canceled`: the API pod got SIGTERM during a sync (every deploy to `master` sends one). Start it
  again. `interrupted by a restart` is the same after a hard kill (OOM, crash), found at the next start.
- `sync already running` (409): wait for the running sync; the sync panel polls it.
