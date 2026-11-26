# Dev log

A working journal kept while building photo-gallery: decisions, measurements, and what the first real runs showed,
in the order they happened. For how to use the project, start with the [README](../README.md).

### Album page format (2026-11-25, pg-3r5h)

Verified against the public album `https://photos.app.goo.gl/TysU32DNqBb1fKjU6` ("OCSF6 Selects", 320 items). The
album client (plan step 5) parses this; if it breaks, probe the live album again and update this entry.

1. GET the share URL with `User-Agent: Mozilla/5.0` and follow redirects. The final URL is
   `https://photos.google.com/share/<albumID>?key=<authKey>`.
2. The HTML holds `AF_initDataCallback({key: 'ds:1', ... data:<JSON>, sideChannel ...})`. The JSON `d` is a
   6-element array: `d[1]` is the first page of items (up to 300), `d[2]` the next-page token (null or empty at
   the end), `d[3][1]` the album title, `d[3][21]` the item count (320 observed).
3. Item `it`: `it[0]` media key; `it[1][0]` base URL (`https://lh3.googleusercontent.com/pw/...`, no size suffix);
   `it[1][1]`/`it[1][2]` width/height; `it[2]` capture time as UTC ms; `it[4]` timezone offset in ms (`-14400000`,
   `-18000000` observed). Local capture time is `it[2] + it[4]` read as UTC. 284 of 300 timestamps had non-zero
   milliseconds, and 43 seconds held more than one item (up to 10).
4. Further pages: POST `https://photos.google.com/_/PhotosUi/data/batchexecute` with form field
   `f.req=[[["snAcKc","[\"<albumID>\",\"<token>\",null,\"<authKey>\"]",null,"generic"]]]`. The response has a line
   starting `[["wrb.fr"`; decode it, then decode `outer[0][2]`. The result has the shape of `d` (items at `[1]`,
   token at `[2]`). Page 2 returned the last 20 items and no token.
5. Size suffixes on `lh3` URLs: `=w400`, `=w2048-h2048`, `=d` (the original, with EXIF). The gallery never uses `=d`.

### Scaffold (2026-11-25, pg-3r5h)

- Copied the photosort conventions (cobra, `wrap`/`errf`/`writeJSON`, buildctl CI, library-chart values) and dropped
  what Postgres-only and no-analyzer make unnecessary.
- One UI image serves two hosts. nginx renders `docker/nginx/default.conf.template` at start, and
  `NGINX_ENVSUBST_FILTER=GALLERY_` keeps it from touching nginx's own `$uri`. `GALLERY_UI_MODE` includes
  `public.conf` (admin routes 404) or `admin.conf` (proxy `/admin-api` to the API's port 8081).
- Chart: `global.security.enabled: false` because the nginx UI pods bind :80 and write `/var/cache/nginx` (as
  photosort and stasharr do); the `api` component turns it back on with 568/568/568. The admin component has
  `fullnameOverride: photo-gallery-admin` so its Service is not `photo-gallery-gallery-admin`.
- The public Service is named `photo-gallery` (primary component: release name), checked with `helm template`.
- The `serve` command opens the pool with pgxpool directly; step 3 moves that to `internal/db.Open` and adds `Migrate`.
- `.tool-versions` pins Node 24.21.0, which was not installed on the dev machine; local runs used 24.14.0.
- CI Postgres: the `homelab` runners are ARC pods without a Docker daemon, so a `services:` container fails at
  `docker version` ("Value cannot be null. (Parameter 'network')"). `test-go` installs `postgresql-16` from apt
  (Ubuntu 24.04 runner image, passwordless sudo) and starts it with `pg_ctlcluster`.


### dHash package (2026-11-25, pg-hqk4)

- `internal/dhash` is stdlib only (the test also uses `golang.org/x/image/draw`), so photosort can take both files byte-for-byte.
- Golden: the 64x48 gradient `R=G=B=(x*7+y*13)%256` hashes to `f3ce3c73cf9c73e7`. Both repos must assert this value.
- A JPEG q85 round trip plus a 50% `ApproxBiLinear` resize stays within Hamming distance 4 of the original.
- Adding `golang.org/x/image` bumped `x/sync` and `x/text` through `go get`.

### Google Photos album client (2026-11-25, pg-dqqc)

- `internal/gphotos` implements the "Album page format" above unchanged: the live format matched the plan, no index or
  fixture changes were needed.
- Live run against `https://photos.app.goo.gl/TysU32DNqBb1fKjU6` ("OCSF6 Selects"): 320 items for `Count` 320, in
  2 pages (300 from `ds:1`, 20 from one batchexecute call), 2 s total. A `Thumb` of item 0 decoded as 400x600.
- Parsing uses `json.RawMessage` arrays and checks each index, so a format change produces an error that names the
  index (`album page format changed: item 12: expected [1][0] string`) rather than a panic.
- Fixtures in `internal/gphotos/testdata` are synthetic: `album.html` (3 items, next token) and `page2.txt`
  (`)]}'` prefix, length line, `wrb.fr` line with 1 item and a null token).
- The live test `TestLiveAlbum` runs only with `GALLERY_LIVE_ALBUM_URL` set.

### Schema and store (2026-11-25, pg-0kis)

- `0001_init.sql` is the plan's schema plus `source_photos.taken_ms SMALLINT` and the index `(event_id, taken, taken_ms)`
  for the sub-second matcher.
- `db.Migrate` creates `schema_migrations` in its own locked transaction, then runs each file in its own transaction
  under `pg_advisory_xact_lock(7262)`. The version check runs after the lock, so two replicas starting together apply
  a file once.
- `dbtest.New` isolates each test in schema `t_<random>` through the pool's `search_path` runtime param. The advisory
  lock is database-wide, so tests migrating in parallel serialize briefly; measured whole store suite: about 4 s with -race.
- `ReplaceAlbumPhotos` takes the Google album id as an extra argument (stored on the album) so step 6 needs no second call.
- `SourcesForEvent` and `UpsertSources` store the dHash as `int64(uint64)`; callers see `*uint64`.
- `Event` from `GetEvent`/`ListEvents` carries `Photos`, `Matched`, `Sources` counts for the admin list (computed by subqueries).

### Public web pages (2026-11-25, pg-kyop)

- Added `/`, `/events/:slug`, `/events/:slug/photos/:id`, `/search`, `/about` against the step 10 contract. Types and
  `publicApi` live in `web/src/api/client.ts`; `request`/`qs` are unchanged (`qs` cannot repeat keys, so photo queries
  use their own builder).
- Event filter state is the URL query using the API's own parameter names (`q`, `keyword`, `rating`, `tier`,
  `min_score`, `keeper`, `lens`, `album`, `sort`), so links share and `parseFilters` feeds the API call directly.
  Paging is `useInfiniteQuery` with offset = items loaded so far, 60 per page.
- Disclaimer: ported with the same text and `disclaimerAcceptedAt` key (15 min). The MUI dialog became a plain
  Tailwind modal because the app has no MUI. `Layout` mounts it only under `/events/`.
- About: the old page's image is an external `lh3` URL, not a repo asset, so it is linked directly.
- Not browser-checked; `pnpm run build` and `pnpm run lint` pass.

### Admin web pages (2026-11-25, pg-oi92)

- Admin pages live in `web/src/pages/admin/` with their own `AdminLayout` (no public nav), registered as a sibling
  of the public `Layout` route so the public-web task's header edits do not touch them.
- `/admin/events/:slug` reads its event from the `['admin-events']` list query; the API has no single-event GET.
- Sync status is its own query with `refetchInterval` 2000 ms while `running`. When a run leaves `running`, the events
  list and photo grid are invalidated when `sync_finished_at` changes, and after saving the event form.
- Photo grid uses `useInfiniteQuery` (100 per page; API returns `{items,total}`). Hide/unhide patches the cached pages in place.
- Types and `adminApi` were appended to `web/src/api/client.ts` (admin calls use `/admin-api`).
- Verified with `pnpm run build && pnpm run lint`; not exercised against a live admin API (step 6 is a separate ticket).

### Public API (2026-11-25, pg-0vap)

- Queries are in `internal/store/public.go`; handlers in `internal/api/public.go`. The photo JSON comes straight
  from `store.PublicPhoto`, so the field set is fixed in one place.
- Dedupe is `DISTINCT ON (event, COALESCE('s'||source_photo_id, 'm'||media_key))` keeping the lowest album position.
  Hidden photos and all filters (including `album`) are applied before the dedupe, so a visible copy in another album
  still shows and `album=All` returns the copy in All. Sorting happens in the outer query.
- `rating=none` means `source_photos.rating IS NULL`, which covers unmatched photos and matched photos without a
  rating. Every other metadata filter is false for unmatched rows (NULL comparison).
- `prev_id`/`next_id` use `lag`/`lead` over the event's deduped taken order (`taken_at, album position, position, id`).
  A photo that lost the dedupe still gets the neighbors of its kept twin.
- `Cache-Control: public, max-age=60` is set by a wrapper around the public mux, on 200 responses only, so 404 and
  500 answers are not cached.
- Invalid `rating`, `tier`, `min_score` (0-5), `keeper`, `sort`, `offset`, `limit` answer 400; `limit` above 200 is
  clamped to 200.

### Admin API and album sync (2026-11-25, pg-c76p)

- `albumsync.MatchAlbum` follows the plan. `Source.TakenMS` is in milliseconds (photosort's `publish` converts
  `SubSecTimeOriginal` "94" to 940); the hundredths filter compares `TakenMS/10` with the album millisecond `/10`.
  When the filter leaves no candidate, the item falls to the hash step like any item without a time hit.
- The hash step only downloads a thumbnail when it can use it: a time tie, or at least one unmatched source with a
  dHash. An event synced before `photosort publish` ran therefore does no thumbnail requests.
- `MatchAlbum` returns the matches and a joined error for failed thumbnail hashes. The matches stay valid; `Run`
  logs the error and counts those items as unmatched.
- `albumsync.RunStarted` runs a sync after `Store.StartSync` and records `done`/`failed` with a context that survives
  cancellation, so a SIGTERM during a sync leaves `failed`, not `running`. `serve` also fails leftover `running`
  rows at start. The CLI `sync --event` and the admin API share it.
- Handlers needing a non-200 code (202, 204) return `status{Code, Body}` from `wrap`.
- The e2e test routes the real share-URL prefix to a fake server with a rewriting `http.RoundTripper`, so URL
  validation stays on the real prefixes.

### Old gallery events (2026-11-25, pg-e7o6)

- `internal/legacy/events.json` holds the 6 events of the old Next.js gallery (`content/events/*.md` in
  `mononen/photo-gallery`), converted with a YAML parser so the `\n` breaks and markdown links in the descriptions
  match the source. `thumbnails` are dropped; covers come from the synced photos.
- FLF8 lists "All Group Photos" as a Google Drive folder, which is not a Photos album and fails the share-link check.
  It moved to the end of the FLF8 description as `All Group Photos: <url>` so the link is not lost.
- `seed-legacy` upserts all 6 events (published) first, then syncs them one after another with the same state
  updates as `sync`; one failed event does not stop the others, and the exit code is 1 if any failed.
- First local run (no photosort sources, so everything is unmatched; 117 s total): ocsf4 5677 album photos (2
  albums), ocsf6 4384 (3), flf7 6692 (3), flf8 3310 (3), flfx 5843 (5). ocsf5 failed: its "Highlights"
  (`https://photos.app.goo.gl/1sAcynuYV6H4GyQP6`) and "All Photos" (`https://photos.app.goo.gl/s6fHwKThu6gddX548`)
  links resolve to a share page that answers HTTP 404, so those albums are no longer shared; "Dave's Photos" still
  works. A sync changes nothing when any album fails, and every `seed-legacy` rerun would put the dead links back,
  so both are dropped from the seed. Re-share them and add them again in the admin page to restore them.

### Google Photos drops the sub-second's leading zero (2026-11-25, pg-88rf)

- First production `photosort publish` for OCSF6 (8387 sources, 4384 album photos): 3802 matched by time, 192 by
  time+hash, 382 by hash, 8 unmatched. The 382 hash matches were all Canon frames with a photosort row at the same
  second, so the time step should have found them.
- In 376 of the 382 the album's milliseconds are 10x photosort's: `SubSecTimeOriginal` "04" (40 ms; exiftool on
  `AA4I6892.CR2` shows `04`) appears in the album as 400 ms. Google drops the leading zero and reads "4" as .4 s.
  Values without a leading zero ("94" → 940) match exactly (3761 of 3761 time matches).
- The matcher now also accepts `source < 100 && source*10 == album` as the same sub-second. "40" and "04" in the same
  second both fit 400 ms; that is a normal tie and the dHash decides (`time_hash`).

### Real-data run (2026-11-25, pg-88rf)

- `seed-legacy` in the production pod: all 6 events synced in 89 s. ocsf4 5677 album photos (2 albums), ocsf5 1780
  (1), ocsf6 4384 (3), flf7 6692 (3), flf8 3310 (3), flfx 5843 (5).
- `photosort publish --event ocsf6 --folder "RAW/Oak City Shred Fest/2026"` in the photosort-api pod (746 s, most of
  it the sync's thumbnail hashing): `sent 8387 photos to ocsf6`, then
  `synced ocsf6: 4384 album photos, 3802 matched by time, 192 by time+hash, 382 by hash, 8 unmatched`.
- The 382 hash matches led to the leading-zero fix above. Re-sync after the fix (87 s):
  4170 by time, 213 by time+hash, 0 by hash, 1 unmatched. The one unmatched item is a 3414x1000 panorama in
  "Small Group Photo", a stitched export with no single source frame.
- Spot checks, album `=w400` next to photosort's `/media/thumb/<source_id>`:
  - Before the fix: 10 random Canon `time` matches, 10 random Sony `time_hash` matches and 6 `hash` matches. All 26
    show the same frame.
  - After the fix: 10 Canon `time` matches (6 with an album sub-second that is a multiple of 100, the case the fix
    adds) and 10 more Sony `time_hash` matches. All 20 show the same frame.
  - At thumbnail size, neighbouring burst frames can look alike, so a Sony `time_hash` match one frame off would not
    show up in this check.
- Search works on real data: `/api/photos?event=ocsf6&q=orange jersey dust&keyword=forest_edge` returns photo 7778
  with its description, focus tier 3 and model score 4. No OCSF6 photo has a cull rating yet (photosort `rating` is
  the manual one), so `rating` is null everywhere and `rating=none` returns all photos.

### Public launch (2026-11-26, pg-6db1)

- Launch approved after the step 12 review. `publicIngress.enabled: true` renders Ingress `photo-gallery-public`
  (class `external`, backend `photo-gallery:80`, TLS `photo-gallery-public-tls`); the certificate was Ready within
  seconds of the deploy.
- Checks through Cloudflare: `/` 200, `/events/ocsf6` 200, `/api/events` 6 events, `/admin` and `/admin-api/events`
  404.
- Surprise: the dev machine's resolver still answered NXDOMAIN for `gallery.monomotion.org` for minutes after
  1.1.1.1 had the record, a negative-cache entry from the time the old release was gone. Check with
  `curl --resolve` or another resolver before suspecting external-dns.
