# Photo gallery v1: public event gallery fed by photosort and Google Photos albums

## Context

Build a public web gallery that shows event photos with photosort's ratings, focus tiers, descriptions, keywords and EXIF, and has tag search. The images stay in public Google Photos albums (Lightroom-edited exports). The gallery scrapes the album pages and links each album photo to its photosort row. Use the same stack as photosort (Go 1.27 stdlib `net/http` + cobra + pgx/v5, React 19 + Vite 8 + Tailwind 4 + react-query + zustand, buildctl CI, Helm on `mononen-library-chart`), the homelab cluster (StackGres Postgres, SOPS secrets, Traefik `internal`/`external` ingress), and `tk` tickets. The work runs as orchestrated agent tickets with one human gate before the public ingress.

The app replaces the old Next.js gallery (`mononen/photo-gallery`, Flux HelmRelease in homelab, `replicas: 0`) and takes over its public host `gallery.monomotion.org` and its 6 events. Face groups, similar-face search, rider name claims, opt-out, and Redis are **not** in this build; do not create face or identity tables.


## Approach

### 0. Orchestration rules (apply to every step)

- **Repos and owners**
  - New repo `mononendev/photo-gallery` (GitHub org repo, so the org `runs-on: [homelab]` runners build it; a user repo cannot). Local path `~/mononendev/photo-gallery` (today a non-git folder that holds only `PLAN.md`).
  - `mononendev/photosort` at `~/mononendev/photosort` (step 7 only).
  - `mononen/homelab` at `~/homelab` (step 1 only).
- **Tickets:** in each repo, `tk` tickets in `.tickets/` (gitignored, never committed). Create one epic "Photo gallery v1" in the gallery repo. Create one ticket per step below with `--parent <epic>`, and record the dependencies with `tk dep`. The homelab step goes in `~/homelab/.tickets`, the photosort step in `~/mononendev/photosort/.tickets`. Each ticket body names its plan step and its acceptance checks. Use `tk start`/`tk add-note`/`tk close`.
- **Waves.** Each ticket is one `task` subagent; tickets in the same wave run in parallel.

  | Wave | Tickets |
  |---|---|
  | A | step 1 (homelab), step 2 (scaffold) |
  | B | step 3 (schema), step 4 (dhash), step 5 (album client), step 8 (public web), step 9 (admin web) |
  | C | step 6 (admin API + sync), step 7 (photosort publish), step 10 (public API) |
  | D | step 11 (legacy seed) |
  | E | step 12 (deploy + real data) |
  | F | step 13 (human gate, public ingress) |

  The API contracts in this plan are fixed, so the web tickets do not wait for the API tickets.
- **Git flow (gallery and photosort)**
  - Branch `<ticket-id>-<kebab-summary>` from `master`. Open a draft PR (`gh pr create --draft`).
  - When CI is green, run a `reviewer` agent on the diff. Fix the findings.
  - Run `gh pr ready`, then `gh pr merge --squash`.
  - The squash title uses the photosort style `Area: what changed` (for example `Sync: match album photos to photosort rows by capture time`). The body explains why. End with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Merges to `master` auto-deploy (LAN only until step 13).
  - The orchestrator rebases a PR on conflicts in the shared files listed in step 2 before merge.
- **Homelab flow:** commit straight to `main` (the repo history does this), with a plain sentence subject. Flux applies it.
- **DEVLOG:** every gallery ticket appends a dated entry to `docs/DEVLOG.md` in the photosort DEVLOG style: `### Title (YYYY-MM-DD, <ticket-id>)`, with decisions, measurements, and surprises.
- **Human gate:** step 13 is the only step that needs the user. The orchestrator stops there and asks.

### 1. Homelab: database, secrets, and removal of the old release (wave A, repo `~/homelab`)

1. Find the `prd-db-cluster` primary pod: label `stackgres.io/cluster-name=prd-db-cluster,role=primary`, else `role=master` (same lookup as `scripts/pg-rotate-password.sh`). Run as `postgres` in container `postgres-util`, with stdin SQL and `-v ON_ERROR_STOP=1`:
   ```sql
   CREATE ROLE photo_gallery LOGIN;
   CREATE DATABASE photo_gallery OWNER photo_gallery;
   ```
   If a role or database exists already, skip that statement. The extensions needed (`plpgsql` and the built-in full text search) need no `CREATE EXTENSION`.
2. Set the app password and write the SOPS secret. Run `scripts/pg-rotate-password.sh prd-db-cluster photo_gallery kubernetes/apps/production/photo-gallery/app/secret-db.sops.yaml photo-gallery-db password production`. The result is Secret `photo-gallery-db`, key `password`.
3. Edit `kubernetes/apps/production/photo-gallery/app/kustomization.yaml`:
   - Set `resources` to `./secret-harbor-pull.sops.yaml` and `./secret-db.sops.yaml`.
   - Delete `app/helmrelease.yaml` and `app/ocirepository.yaml`.
   - Add `- name: prd-db-cluster` to `dependsOn` in `photo-gallery/ks.yaml`.
   - Flux prune uninstalls the old Helm release `photo-gallery`. This frees the Service `photo-gallery` and the ingresses for `gallery.monomotion.org` and `gallery.adoah.dev`.
   - Keep Secret `photo-gallery-harbor-pull`, because the new chart uses it.
4. Commit to `main` with subject `photo-gallery: drop the old Next.js release, add the database secret` and push. Run `flux reconcile ks photo-gallery --with-source`.
5. **Done when:**
   - `kubectl -n production get helmrelease photo-gallery` returns NotFound.
   - `kubectl -n production get svc photo-gallery` returns NotFound. If the Service or old ingresses stay, delete them with `kubectl delete`; they belong to the old release.
   - `kubectl -n production get secret photo-gallery-db photo-gallery-harbor-pull` lists both.

### 2. Repo scaffold, CI, and chart (wave A; push to `master` only after step 1 is done)

In `~/mononendev/photo-gallery`:

1. Move `PLAN.md` to `docs/PLAN.md` and overwrite it with this approved plan, then `git init -b master`.
2. Create the files below. Copy the photosort file named in each row and adapt it. Do not invent other conventions.

| Path | Content |
|---|---|
| `go.mod` | `module github.com/mononendev/photo-gallery`, `go 1.27.1`. Deps: `github.com/spf13/cobra`, `github.com/jackc/pgx/v5` (use `pgxpool`; Postgres only, no SQLite), `golang.org/x/image` (only if step 4 needs it). |
| `.tool-versions` | `nodejs 24.21.0`, `golang 1.27.1` |
| `.gitignore` | `.tickets/`, `bin/`, `web/node_modules/`, `web/dist/`, `.ci/chart/charts/`, `.ci/chart/Chart.lock`, `CLAUDE.md`, `.claude/`, `.scratch/` |
| `LICENSE` | AGPL-3.0, the same text as photosort |
| `cmd/photo-gallery/main.go` | cobra root `photo-gallery` with `var version = "dev"` (set by `-ldflags "-X main.version=..."`) and subcommands `serve`, `migrate`, `sync` (step 6), `seed-legacy` (step 11). Pattern: photosort `cmd/photosort/main.go` `root.AddCommand(...)`, one `func xCmd() *cobra.Command` per file. |
| `cmd/photo-gallery/serve.go` | Opens the DB (`GALLERY_DB`, required: `GALLERY_DB is not set`), runs migrations, and starts two `http.Server`s: public on `:$GALLERY_PORT` (default `8080`) serving `api.Server.Public()`, and admin on `:$GALLERY_ADMIN_PORT` (default `8081`) serving `api.Server.Admin()`. Graceful shutdown on SIGTERM with 20 s. |
| `internal/api/server.go` | Copy the photosort `internal/api/server.go` helpers: `type handler func(r *http.Request) (any, error)`, `HTTPError`, `errf(code, format, ...)`, `wrap` (body `{"detail": ...}`), `writeJSON` (`SetEscapeHTML(false)`). `Public()` builds a `ServeMux` and calls `s.publicRoutes(route)`; `Admin()` builds another and calls `s.adminRoutes(route)`. Put `publicRoutes` (in `public.go`) and `adminRoutes` (in `admin.go`) in their own files so steps 6 and 10 do not edit the same function. Scaffold registers only `GET /api/health` → `{"ok":true,"version":...,"database":<ping ok bool>}` on both muxes. Use `log/slog` with a JSON handler on stderr. |
| `web/` | Copy photosort `web/` config files exactly: `package.json` (name `photo-gallery-web`, same deps and versions, `packageManager pnpm@12.6.0`), `pnpm-workspace.yaml`, `tsconfig*.json`, `eslint.config.js`, `vite.config.ts` (proxy `/api` → `VITE_API_URL` or `http://localhost:8080`, and `/admin-api` → `VITE_ADMIN_API_URL` or `http://localhost:8081`), `index.html` (`<html class="dark">`, title `Photo Gallery`), `src/main.tsx`, `src/index.css` (`@import "tailwindcss";` + dark variant), and `src/App.tsx` with `BrowserRouter`, `QueryClient({defaultOptions:{queries:{refetchOnWindowFocus:false}}})`, and a pathless `<Layout/>` route. Add `src/api/client.ts` with the photosort `request<T>` + exported `api` object pattern. |
| `docker/api.Dockerfile` | photosort `docker/api.Dockerfile` without exiftool. Binary `photo-gallery`, user/group `568` named `gallery`, `ENV GALLERY_PORT=8080 GALLERY_ADMIN_PORT=8081`, `EXPOSE 8080 8081`, `ENTRYPOINT ["photo-gallery"]`, `CMD ["serve"]`. |
| `docker/ui.Dockerfile` | photosort `docker/ui.Dockerfile`. In `production` (`nginx:alpine`), copy `docker/nginx/default.conf.template` to `/etc/nginx/templates/default.conf.template` and `docker/nginx/public.conf` + `docker/nginx/admin.conf` to `/etc/nginx/snippets/`. Set `ENV GALLERY_UI_MODE=public` and `NGINX_ENVSUBST_FILTER=GALLERY_` so only `GALLERY_*` variables are substituted. |
| `docker/nginx/default.conf.template` | `server { listen 80; root /usr/share/nginx/html; index index.html; include /etc/nginx/snippets/${GALLERY_UI_MODE}.conf; location / { try_files $uri $uri/ /index.html; } }` |
| `docker/nginx/public.conf` | `location /api/ { proxy_pass http://photo-gallery-api:8080; proxy_http_version 1.1; }`, `location /admin-api/ { return 404; }`, `location /admin { return 404; }` |
| `docker/nginx/admin.conf` | `location /api/ { proxy_pass http://photo-gallery-api:8080; proxy_http_version 1.1; }`, `location /admin-api/ { proxy_pass http://photo-gallery-api:8081; proxy_http_version 1.1; proxy_read_timeout 120s; }` |
| `.dockerignore` | photosort pattern: ignore `*`, then allow `go.mod go.sum cmd/ internal/ web/ docker/`, exclude `**/*_test.go web/node_modules web/dist` |
| `Makefile` | photosort Makefile with `IMAGES := api ui`, image names `$(REGISTRY)/photo-gallery-$*`, and these targets. `db`: `docker run -d --name photo-gallery-pg -p 5433:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=gallery postgres:16-alpine` (skip when the container exists). `dev`: `GALLERY_DB=$(GALLERY_DB) go run ./cmd/photo-gallery serve` with `GALLERY_DB ?= postgres://postgres:postgres@localhost:5433/gallery?sslmode=disable`. `web`, `test` (`go vet ./... && go test -race ./...`; then `cd web && pnpm run build && pnpm run lint`), `bin`, `build-%`, `push`, `deploy` (`helm upgrade --install photo-gallery .ci/chart --namespace production --set mononen-library-chart.global.imageDefaults.tag=$(VERSION)`). |
| `.github/workflows/ci.yml` | photosort `ci.yml` without `test-analyzer`. `test-go` gets `services: postgres: {image: postgres:16-alpine, env: {POSTGRES_PASSWORD: postgres, POSTGRES_DB: gallery}, ports: ["5432:5432"]}` and env `GALLERY_TEST_PG: postgres://postgres:postgres@localhost:5432/gallery?sslmode=disable`, `GALLERY_REQUIRE_PG: "1"`. CI never touches `prd-db-cluster`. Build matrix `[api, ui]`, image `${REGISTRY}/projects/photo-gallery-${image}`, cache ref `...:buildcache`. Deploy on `master`/`main`: `helm upgrade --install photo-gallery .ci/chart --namespace production --set mononen-library-chart.global.imageDefaults.tag=<version>`. |
| `.ci/chart/` | See the chart rows below. |
| `docs/DEPLOY.md` | Shape of the deployment, hosts, the secrets and where they come from (step 1), the per-event runbook from step 12, and troubleshooting for `sync_error`. |
| `docs/DEVLOG.md` | Header like the photosort DEVLOG. The first entry records the album page format from the "Album page format" section of step 5. |
| `README.md` | What it is, how to run locally (`make db`, `make dev`, `make web`), and links to `docs/`. |

3. Chart `.ci/chart/Chart.yaml`: `name: photo-gallery`, `version: 1.0.0`, dependency `mononen-library-chart` `1.2.0` from `https://mononen.github.io/charts/`.
4. Chart `values.yaml` under `mononen-library-chart:`:
   - `global`: `namespace: production`, `env: prd`, `domain: adoah.dev`, `imageDefaults.pullSecrets: [{name: photo-gallery-harbor-pull}]`.
   - Component `photo-gallery` (`primaryComponent: true`, public UI):
     - Image `registry.adoah.dev/projects/photo-gallery-ui`, port `http` 80, env `GALLERY_UI_MODE=public`, resources 10m/32Mi request and 200m/128Mi limit, service port 80.
     - Ingress: `className: internal`, `subdomain: gallery`, annotations `cert-manager.io/cluster-issuer: letsencrypt-production` and `external-dns.home.arpa/lan-public: "true"`, `internal.tls: [{secretName: photo-gallery-tls, hosts: [gallery.adoah.dev]}]`, external disabled.
   - Component `gallery-admin`: the same image with env `GALLERY_UI_MODE=admin`. Internal ingress `subdomain: gallery-admin`, the same annotations, tls secret `photo-gallery-admin-tls`, host `gallery-admin.adoah.dev`.
   - Component `api`:
     - `fullnameOverride: photo-gallery-api`, `replicaCount: 1`, security 568/568/568, `generate.ingress: false`.
     - Image `registry.adoah.dev/projects/photo-gallery-api`, ports `http` 8080 and `admin` 8081. Service port 8080 with `additionalPorts: [{name: admin, port: 8081, targetPort: 8081}]`.
     - Env (stasharr DSN pattern): `DB_PASSWORD` from `secretKeyRef photo-gallery-db/password`, then `GALLERY_DB=postgres://photo_gallery:$(DB_PASSWORD)@prd-db-cluster.production.svc.cluster.local:5432/photo_gallery`.
     - readinessProbe `GET /api/health :8080`. Resources 50m/64Mi request, 1000m/512Mi limit.
5. Chart top-level `publicIngress: {enabled: false, host: gallery.monomotion.org}` and a custom template `templates/ingress-public.yaml`. The library chart builds every host as `<subdomain>.<global.domain>`, so it cannot make a `monomotion.org` host. When `.Values.publicIngress.enabled`, the template renders one `networking.k8s.io/v1` Ingress:
   - name `photo-gallery-public`, `ingressClassName: external`;
   - annotation `cert-manager.io/cluster-issuer: letsencrypt-production`;
   - rule host `{{ .Values.publicIngress.host }}`, path `/` Prefix → service `photo-gallery` port 80;
   - tls `photo-gallery-public-tls`.
   - Confirm the generated UI Service name with `helm template` before you hard-code it.
6. Create the GitHub repo:
   - Run `gh repo create mononendev/photo-gallery --public --source . --remote origin`.
   - Make the initial commit `Scaffold: Go API, React UI, chart and CI` straight on `master` (bootstrap only). Push it after step 1 is done.
7. Shared files that later steps touch:
   - `web/src/App.tsx`: routes from steps 8 and 9.
   - `web/src/api/client.ts`: types and methods from steps 8 and 9.
   - `cmd/photo-gallery/main.go`: `AddCommand` lines from steps 6 and 11.
8. **Done when:**
   - `make test` passes with the local DB.
   - CI on `master` is green and deploys.
   - `kubectl -n production rollout status deploy/photo-gallery-api` succeeds.
   - `curl -s https://gallery.adoah.dev/api/health` returns `"ok":true` and `"database":true`.
   - `curl -s -o /dev/null -w '%{http_code}' https://gallery.adoah.dev/admin-api/events` returns `404`.

### 3. Schema and migrations (wave B)

- **Migration runner:** `internal/db/migrations/0001_init.sql` plus `migrations.go` with `//go:embed *.sql` (the stasharr pattern, up-only like photosort). `internal/db/db.go`:
  - `Open(ctx, url) (*pgxpool.Pool, error)`.
  - `Migrate(ctx, pool) error`: create `schema_migrations(version INT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`. Apply each `NNNN_*.sql` file in name order whose version is not yet recorded. Each file runs in its own transaction, under `pg_advisory_xact_lock(7262)`.
  - `photo-gallery migrate` runs `Migrate` and exits.
- **`0001_init.sql`** (exact):
  ```sql
  CREATE TABLE events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    title TEXT NOT NULL,
    series TEXT,
    event_date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    published BOOLEAN NOT NULL DEFAULT FALSE,
    sync_state TEXT NOT NULL DEFAULT 'idle' CHECK (sync_state IN ('idle','running','done','failed')),
    sync_started_at TIMESTAMPTZ,
    sync_finished_at TIMESTAMPTZ,
    sync_result JSONB,
    sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  );
  CREATE TABLE albums (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    share_url TEXT NOT NULL,
    position INT NOT NULL,
    google_album_id TEXT,
    UNIQUE (event_id, share_url)
  );
  CREATE TABLE source_photos (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL,
    rel_path TEXT NOT NULL,
    taken TEXT,
    dhash BIGINT,
    rating SMALLINT,
    focus_tier SMALLINT,
    quality_score SMALLINT,
    keeper BOOLEAN,
    subject TEXT,
    composition TEXT,
    action TEXT,
    description TEXT,
    keywords TEXT[] NOT NULL DEFAULT '{}',
    adjectives TEXT[] NOT NULL DEFAULT '{}',
    exif JSONB NOT NULL DEFAULT '{}',
    search_text TEXT NOT NULL DEFAULT '',
    search TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', search_text)) STORED,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, source_id)
  );
  CREATE INDEX source_photos_taken ON source_photos (event_id, taken);
  CREATE INDEX source_photos_keywords ON source_photos USING GIN (keywords);
  CREATE INDEX source_photos_search ON source_photos USING GIN (search);
  CREATE TABLE photos (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    album_id BIGINT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    media_key TEXT NOT NULL,
    base_url TEXT NOT NULL,
    width INT NOT NULL,
    height INT NOT NULL,
    taken_at TIMESTAMPTZ NOT NULL,
    tz_offset_minutes INT NOT NULL,
    position INT NOT NULL,
    source_photo_id BIGINT REFERENCES source_photos(id) ON DELETE SET NULL,
    match_method TEXT CHECK (match_method IN ('time','time_hash','hash')),
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (album_id, media_key)
  );
  CREATE INDEX photos_source ON photos (source_photo_id);
  ```
  Column notes:
  - `taken` is the EXIF `DateTimeOriginal` text `YYYY:MM:DD HH:MM:SS`, as photosort stores it. `taken_ms` (INT, nullable) is the EXIF `SubSecTimeOriginal` as milliseconds (0-999); add the column `taken_ms SMALLINT` to `source_photos` and index `(event_id, taken, taken_ms)`.
  - `dhash` is the uint64 dHash cast to int64.
  - `rating`: 0-3 focus tiers, 4 banger (photosort `export.Banger`).
- **Test helper** `internal/db/dbtest/dbtest.go`: `New(t) *pgxpool.Pool`. Use the photosort `internal/db/dbtest` pattern:
  - Create schema `t_<random>` on `GALLERY_TEST_PG`.
  - Open the pool with `search_path=t_<random>` and run `Migrate`.
  - Drop the schema at cleanup.
  - When `GALLERY_TEST_PG` is unset: `t.Skip`, but `t.Fatal("GALLERY_TEST_PG is required")` when `GALLERY_REQUIRE_PG=1`.
- **Store** `internal/store/` (hand-written SQL with pgx; no sqlc, no ORM, because photosort uses neither). Types and functions used by steps 6, 10, and 11:
  - `UpsertEvent(ctx, EventInput) (Event, error)`: insert or update by slug. Replace the album set: delete albums whose `share_url` is not in the input (photos cascade), and upsert the others with `name` and `position` = index.
  - `GetEvent(ctx, slug)` and `ListEvents(ctx, includeUnpublished bool)`.
  - `DeleteEvent(ctx, slug)`.
  - `UpsertSources(ctx, eventID, []SourceInput) (int, error)`: upsert by `(event_id, source_id)`. Build `search_text` in Go: `description`, `keywords`, `adjectives`, `subject`, `composition`, `action` joined with spaces, with `_` replaced by a space.
  - `SourcesForEvent(ctx, eventID)`.
  - `ReplaceAlbumPhotos(ctx, albumID, []PhotoInput)`, in one transaction: upsert by `(album_id, media_key)`, set `source_photo_id`/`match_method` from the input, and delete the rows of the album whose `media_key` is not in the input. `hidden` keeps its value on update.
  - `SetPhotoHidden(ctx, id, hidden)`.
  - The query functions of step 10.
- **Done when:** `go test ./internal/db/... ./internal/store/...` with `GALLERY_TEST_PG` set passes these tests:
  - Running `Migrate` twice is a no-op.
  - `UpsertEvent` with one album removed deletes that album's photos.
  - `UpsertSources` twice keeps one row per `source_id`.
  - A `search @@ websearch_to_tsquery('english','carving')` query finds a source whose `action` is `carving`.

### 4. dHash package, shared by both repos (wave B, gallery repo first)

- **API:** `internal/dhash/dhash.go`:
  - `Hash(img image.Image) uint64`
  - `Distance(a, b uint64) int` (`bits.OnesCount64(a^b)`)
  - `Hex(h uint64) string` (16 lowercase hex digits, zero-padded)
  - `ParseHex(s string) (uint64, error)`
- **Algorithm** (exact, so both repos agree):
  1. Convert each pixel to gray with `color.GrayModel.Convert(c).(color.Gray).Y`.
  2. Average the pixels into a 9×8 grid (9 columns, 8 rows). Cell `(col, row)` covers x in `[col*W/9, (col+1)*W/9)` and y in `[row*H/8, (row+1)*H/8)`, integer division on bounds relative to `img.Bounds().Min`. Average in `float64`. A cell with no pixels (image narrower than 9 or shorter than 8) takes the nearest pixel at its start bound.
  3. Bit `row*8+col` (col 0..7) is 1 when `cell(col,row) < cell(col+1,row)`.
- **Golden test:** build a deterministic `image.RGBA` 64×48 with `R=G=B=uint8((x*7+y*13)%256)`, `A=255`. Assert `Hex(Hash(img))` equals a constant. Compute the constant once in this ticket and write it into the test and into the ticket note. Add a second test: a JPEG encode at quality 85, a decode, and a 50% resize (`golang.org/x/image/draw.ApproxBiLinear`) of the same image must give `Distance` ≤ 4 from the original.
- **Copy to photosort (step 7):** copy `internal/dhash/dhash.go` and `dhash_test.go` byte-for-byte into photosort `internal/dhash/`, with the same golden constant.

### 5. Google Photos album client (wave B)

- **API:** `internal/gphotos/` with:
  ```go
  type Item struct {
      MediaKey string
      BaseURL  string
      Width    int
      Height   int
      TakenMS  int64
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
  func (c *Client) FetchAlbum(ctx context.Context, shareURL string) (Album, error)
  func (c *Client) Thumb(ctx context.Context, baseURL string) (image.Image, error) // GET baseURL+"=w400", decode JPEG
  ```
- **Album page format** (verified 2026-11-25 against the public album `https://photos.app.goo.gl/TysU32DNqBb1fKjU6` "OCSF6 Selects", 320 items):
  1. GET the share URL with header `User-Agent: Mozilla/5.0` and follow redirects. The final URL is `https://photos.google.com/share/<albumID>?key=<authKey>`. Take `albumID` from the last path segment and `authKey` from query `key`. If either is missing, return `album link did not resolve to a shared album: <url>`.
  2. In the HTML, take the JSON from regex `(?s)AF_initDataCallback\(\{key: 'ds:1'.*?data:(.*?), sideChannel`. Call it `d` (a 6-element array):
     - `d[1]` is the item list (first page, up to 300 items).
     - `d[2]` is the next-page token (string, or null/empty at the end).
     - `d[3][1]` is the album title. `d[3][21]` is the item count (320 observed).
  3. Item `it`:
     - `it[0]` is the media key.
     - `it[1][0]` is the base URL (`https://lh3.googleusercontent.com/pw/...`, no size suffix).
     - `it[1][1]`/`it[1][2]` are width/height.
     - `it[2]` is the capture time as UTC ms.
     - `it[4]` is the timezone offset in ms (observed `-14400000`/`-18000000`).
     - Local capture time = `it[2] + it[4]` read as UTC. 284 of 300 timestamps had non-zero milliseconds (sub-seconds kept). 43 seconds had more than one item, up to 10.
  4. Next pages: POST `BatchURL`, `Content-Type: application/x-www-form-urlencoded;charset=UTF-8`, form field `f.req` = `[[["snAcKc","[\"<albumID>\",\"<token>\",null,\"<authKey>\"]",null,"generic"]]]`.
     - In the response body, find the line that starts with `[["wrb.fr"`. JSON-decode it, then JSON-decode `outer[0][2]`. The result has the shape of `d`: items at `[1]`, token at `[2]`.
     - Loop until the token is null or empty. Stop with an error after 100 pages. (Verified: page 2 returned the last 20 items and no token.)
  5. Each error names the step and the cause, for example `album page format changed: ds:1 block not found`, `album page format changed: item 12: expected [1][0] string`, `batchexecute page 3: HTTP 429`.
  6. Size suffixes verified on `lh3` URLs: `=w400`, `=w2048-h2048`, `=d` (original, with EXIF). The gallery never uses `=d`.
- **Tests:** synthetic fixtures (no real album data) in `internal/gphotos/testdata/`:
  - `album.html`: three items, a `ds:1` block with the shape above, and a next token.
  - `page2.txt`: a `)]}'` prefix line, a length line, and a `wrb.fr` line with one item and a null token.
  - Serve both from `httptest` (the share URL redirects to `/share/ALBUM?key=K`). Assert 4 items, the title, local times, and that the POST body has `snAcKc` with `ALBUM`, the token, and `K`.
  - Live test `TestLiveAlbum` runs only when `GALLERY_LIVE_ALBUM_URL` is set. It asserts `len(Items) == Count`.

### 6. Admin API and album sync (wave C; needs steps 3, 4, 5)

- **Matcher:** `internal/albumsync/match.go`. Pure function, no I/O except through the callback:
  ```go
  type Source struct {
      ID    int64
      Taken   string
      TakenMS *int // sub-second of the capture time, from SubSecTimeOriginal
      DHash *uint64
  }
  type Match struct {
      SourceID int64
      Method   string // "time", "time_hash" or "hash"
  }
  func MatchAlbum(items []gphotos.Item, sources []Source, thumbHash func(gphotos.Item) (uint64, error)) ([]*Match, error)
  ```
  The result has one entry per item; nil means unmatched. Process the items in order of local time, then input order. Keep a set of sources already matched in this album, so one source matches at most one item per album. For each item:
  1. `key = time.UnixMilli(it.TakenMS + it.TZOffsetMS).UTC().Format("2006:01:02 15:04:05")` and `ms = (it.TakenMS + it.TZOffsetMS) mod 1000`. Candidates = unmatched sources whose `Taken == key`. If any candidate has a `TakenMS`, keep only candidates with `TakenMS / 10 == ms / 10` (same hundredth of a second; Canon writes hundredths and the album value is that hundredth × 10).
  2. Exactly 1 candidate → method `time`.
  3. More than 1 → hash the item with `thumbHash`. Pick the candidate with the smallest `dhash.Distance`; candidates without a dHash rank last; equal distance → lower `ID`. Method `time_hash`.
  4. 0 candidates → hash the item. Compare it with all unmatched sources that have a dHash. If the best distance is ≤ 10 → method `hash`; else unmatched.
  5. `thumbHash` is called at most once per item. When it fails, cases 3 and 4 return no match for that item, and the error goes in the result as an unmatched item (no abort).
- **Runner:** `albumsync.Run(ctx, st *store.Store, gp *gphotos.Client, slug string) (Result, error)`:
  1. Fetch every album of the event first. If any fetch fails, return the error and change no photos.
  2. Load the sources and run `MatchAlbum` per album.
  3. Call `ReplaceAlbumPhotos` per album with `position` = item index and `taken_at` = `TakenMS`. Also store `google_album_id`.
  4. `Result` JSON (stored in `events.sync_result`): `{"albums":N,"items":N,"matched_time":N,"matched_time_hash":N,"matched_hash":N,"unmatched":N,"unmatched_items":[{"album":"<name>","media_key":"...","url":"<base>"}]}`. `unmatched_items` is capped at 200.
- **Sync states:**
  - `POST` sets `sync_state='running'` with `UPDATE ... WHERE slug=$1 AND sync_state <> 'running' RETURNING id`. No row: 409 `sync already running` if the event exists, else 404.
  - The run goes in a goroutine with the server's base context.
  - At the end, set `done` + `sync_result` or `failed` + `sync_error`, and `sync_finished_at`.
  - At serve start, `UPDATE events SET sync_state='failed', sync_error='interrupted by a restart' WHERE sync_state='running'`.
- **CLI:** `photo-gallery sync --event <slug>` runs `albumsync.Run` in the foreground with the same state updates, and prints the result JSON.
- **Admin routes** in `adminRoutes`, all JSON, errors `{"detail": ...}`, no auth (LAN-only, like photosort):

  | Route | Behavior |
  |---|---|
  | `GET /admin-api/events` | All events. Each has `slug, title, series, event_date, description, published, albums[{name,share_url}], sync_state, sync_started_at, sync_finished_at, sync_result, sync_error`, and counts `photos`, `matched`, `sources`. |
  | `PUT /admin-api/events/{slug}` | Body `{"title","series","event_date":"YYYY-MM-DD","description","published","albums":[{"name","share_url"}]}`. Validate: slug regex, title non-empty, date parses, each `share_url` starts with `https://photos.app.goo.gl/` or `https://photos.google.com/share/`. Fail → 400. Calls `UpsertEvent` and returns the event. |
  | `DELETE /admin-api/events/{slug}` | 204, or 404. |
  | `POST /admin-api/events/{slug}/sources` | Body `{"photos":[SourceWire...]}`, max 500 items (more → 413 `at most 500 photos per request`). Unknown event → 404 `event <slug> not found`. Returns `{"upserted":N}`. |
  | `POST /admin-api/events/{slug}/sync` | 202 `{"sync_state":"running"}`, or 409/404. |
  | `GET /admin-api/events/{slug}/sync` | `{sync_state, sync_started_at, sync_finished_at, sync_result, sync_error}` |
  | `GET /admin-api/events/{slug}/photos?offset=&limit=` | All photos, including hidden ones, ordered `taken_at, album position, position`. Each has `id, album, base_url, width, height, taken_at, hidden, match_method, source_id, rel_path`. Default limit 100, max 500. |
  | `PATCH /admin-api/photos/{id}` | Body `{"hidden":bool}` → 204. |

- **`SourceWire`** (the photosort → gallery contract; JSON field names are exact):
  ```json
  {"source_id":47048,"rel_path":"RAW/Oak City Shred Fest/2026/April/23/AA4I6853.CR2","taken":"2026:04:23 09:03:23","taken_ms":420,"dhash":"a1b2c3d4e5f60718",
   "rating":3,"focus_tier":3,"quality_score":4,"keeper":true,"subject":"rider_action","composition":"full_body","action":"carving",
   "description":"...","keywords":["onewheel","trail"],"adjectives":["dynamic"],
   "exif":{"camera":"Canon EOS-1D X Mark II","lens":"EF85mm f/1.2L II USM","f_number":1.2,"shutter_s":0.00025,"iso":100,"focal_mm":85}}
  ```
  Every field except `source_id` and `rel_path` may be null or missing. `keywords`/`adjectives` missing → `[]`. A `dhash` that does not parse → 400.
- **Tests:**
  - `MatchAlbum` table tests:
    1. One same-second source → `time`.
    2. Three same-second sources; the callback hash is nearest to the second → `time_hash` picks the second.
    3. No `taken`, hash distance 8 → `hash`.
    4. Distance 11 → unmatched.
    5. Two items with the same second and one source → only one item matches.
  - An admin API test with `httptest`, the dbtest DB, and a fake `gphotos` server (the step 5 fixtures):
    1. PUT the event, then POST 2 sources (one whose `taken` equals a fixture item's local second).
    2. POST sync and poll GET until `done`.
    3. Assert `matched_time == 1` and `unmatched == 3`.

### 7. photosort `publish` command (wave C, repo `~/mononendev/photosort`; needs step 4's file and constant)

- **New file** `cmd/photosort/publish.go`, `func publishCmd() *cobra.Command`. Register it in the `root.AddCommand(...)` list in `cmd/photosort/main.go`.
  - `Use: "publish"`.
  - Flags: `--event` (required), `--folder` (required), `--gallery` (default `$PHOTOSORT_GALLERY`), `--no-sync` (bool).
- **Behavior:**
  1. Resolve the folder. A relative `--folder` is joined to `$PHOTOSORT_PHOTOS`; if that is unset, error `--folder is relative and PHOTOSORT_PHOTOS is not set`. An empty gallery URL → `set --gallery or PHOTOSORT_GALLERY`.
  2. `a := openApp()`. Rows = `a.db.RowsUnder([]string{folder}, "local_json IS NOT NULL", nil, "")`. Zero rows → `no analyzed photos under <folder>`.
  3. Source = `pj.Str(a.cfg.Get()["focus_source"])`, as in `sortCmd`.
  4. Per row:
     - `rec, err := export.FinalRecord(exportRow(r), source)`. Reuse the existing `exportRow` in `cmd/photosort/dbcmd.go`.
     - Parse `*r.LocalJSON` with `encoding/json` to take `exif` (keys `camera, lens, f_number, shutter_s, iso, focal_mm`; `taken` goes to the top-level field).
     - `taken_ms`: read EXIF `SubSecTimeOriginal` (tag 0x9291) from the original file with photosort's `internal/exif` reader (add the tag there). The value is the digits after the decimal point, so parse it as a string, never as an integer: trim spaces, keep only leading digits, right-pad with `0` to 3 characters, cut to 3, then parse (`"94"` → 940, `"5"` → 500, `"05"` → 50, `"1234"` → 123). Empty or no digits → `null`. Table test with exactly these cases. Canon CR2 has the tag; Sony ARW does not (measured 2026-11-25).
     - `rel_path` = path relative to `$PHOTOSORT_PHOTOS` when it is a prefix, else the absolute path.
     - `dhash` = `dhash.Hex(dhash.Hash(img))` of `filepath.Join(a.cacheDir(), id+"_thumb.jpg")`. If the file is missing or does not decode → `null`.
     - Convert the `Record` fields (`any`, Python-style values) into `SourceWire`. Non-integer or nil values become `null`. `Banger` is already in `rating` (4).
  5. POST to `<gallery>/admin-api/events/<slug>/sources` in batches of 500. Non-2xx → error `gallery: HTTP <code>: <detail>`.
  6. Unless `--no-sync`: POST `.../sync`, then poll GET `.../sync` every 2 s for up to 10 min.
     - `failed` → error with `sync_error`.
     - `done` → print `synced <slug>: <items> album photos, <matched_time> matched by time, <matched_time_hash> by time+hash, <matched_hash> by hash, <unmatched> unmatched`.
- **Copy the dHash files** from step 4 into `internal/dhash/`.
- **Chart:** in photosort `.ci/chart/values.yaml` api container env, add `PHOTOSORT_GALLERY: "http://photo-gallery-api.production.svc.cluster.local:8081"`.
- **Docs:** add a `publish` section to photosort `docs/CLI.md`.
- **Test** `cmd/photosort/publish_test.go`:
  1. Create a SQLite workdir DB with two image rows under a temp folder. Write `local_json` with `exif.taken`, and a `_thumb.jpg` for one row.
  2. Run a fake gallery on `httptest` that records the bodies and answers sync with `done`.
  3. Run the command with `--gallery` set to the fake.
  4. Assert one sources request with 2 photos, the correct `taken`, a 16-hex `dhash` for one photo and `null` for the other, and the printed summary line.

### 8. Public web pages (wave B; API contract in step 10)

- **Look:** Tailwind with the photosort dark theme (`bg-gray-950 text-gray-100`, panels `bg-gray-900` with `border-gray-800`, rounded corners, `text-gray-400` for labels). Rating colors: 0 red `#e5484d`, 1 orange `#f76b15`, 2 yellow `#ffc53d`, 3 green `#46a758`, 4 blue `#3e63dd`; names `Missed, Soft, Slightly soft, Sharp, Banger`. Focus tiers 0-3 use the same names and colors. A sticky header has the site name "Photo Gallery" (links to `/`) and nav links Events, Search, About. Photo cards: image with `aspect-ratio`, a rating pill top-left (color dot + name; absent when not rated), model score stars (`★` ×5, filled = `quality_score`) top-right, and up to 4 keywords joined by ` · ` under the image.
- **Routes** in `App.tsx`:
  - `/`: event list. Cards with a cover (first photo `url + "=w800"`), title, date, series chip, photo count. Series filter chips. Sorted by date, newest first.
  - `/events/:slug`: header (title, date, description, links "Open in Google Photos" per album), a left filter sidebar (260 px wide, sticky; stacks above the grid below 900 px), a grid (`repeat(auto-fill, minmax(240px, 1fr))`), a result count, a sort select (time taken, model score, your rating), and a "Load more" button (offset paging, 60 per page).
    - Filters: search, rating (incl. Not rated), focus tier, min score, keepers only, keyword chips from facets, lens, album.
    - Filter state is in the URL query (`useSearchParams`), so links are shareable.
    - Photo cards without metadata show only the image.
  - `/events/:slug/photos/:id`: two columns (image left, 380 px side panel right; one column below 900 px). Image `url + "=w2048-h2048"`. Side panels: Ratings (your rating, focus tier, model score + Keeper badge), Description (text, keyword chips linking to `/search?keyword=<k>`, adjectives as muted chips, subject/composition/action with `_` shown as spaces), Camera (camera, lens, focal mm, `f/<f_number>`, shutter as `1/<round(1/shutter_s)> s`, ISO). Hide a panel when all its fields are null. Prev/next buttons and ArrowLeft/ArrowRight.
  - `/search`: cross-event results for `q` and `keyword` with the same grid.
  - `/about`: port the text and the image of `content/about.md` from `mononen/photo-gallery` (`gh api repos/mononen/photo-gallery/contents/content/about.md`) as static TSX.
- **Disclaimer:** port `src/components/DisclaimerModal.tsx` from `mononen/photo-gallery`: the same text and the same `localStorage` key `disclaimerAcceptedAt`, shown again after 15 minutes. Show it on `/events/*`.
- **Grid images:** `<img loading="lazy" src={url+"=w800"} srcSet={`${url}=w400 400w, ${url}=w800 800w, ${url}=w1200 1200w`}>`, with `aspect-ratio` from `width/height`.
- **React Query keys:** `['events']`, `['event', slug]`, `['photos', params]`, `['photo', id]`.
- **Done when:** `pnpm run build && pnpm run lint` pass. Step 12 checks the browser.

### 9. Admin web pages (wave B; admin contract in step 6)

- **Routes** under `/admin`:
  - `/admin`: a table of all events (title, slug, date, published, photos/matched/sources counts, sync state) and a "New event" form.
  - `/admin/events/:slug`, sections:
    1. Edit form for the event fields, and an album list with add/remove rows (`name`, `share_url`). Save → PUT.
    2. A sync panel with a "Sync albums" button → POST, polled with `refetchInterval` 2000 ms while `running`. It shows the result counts, `sync_error`, and the unmatched items as `=w200` thumbnails.
    3. A photo grid with `=w400` thumbnails, the match method badge, the source `rel_path` in the tooltip, and a hide/unhide toggle → PATCH.
    4. A Delete event button with `confirm()`.
- **Done when:** `pnpm run build && pnpm run lint` pass.

### 10. Public API (wave C; needs step 3)

- **Routes** in `publicRoutes`:
  - Read-only. Only `published` events and photos with `hidden = false` are visible. An unpublished or unknown event → 404 `event not found`.
  - Add header `Cache-Control: public, max-age=60` to the `GET /api/events*` and `GET /api/photos*` responses.

  | Route | Response |
  |---|---|
  | `GET /api/events` | `[{slug,title,series,event_date,description,photo_count,cover_url,albums:[{name,share_url}]}]`, newest first. `cover_url` = base URL of the event's first public photo, or null. |
  | `GET /api/events/{slug}` | The event fields plus `facets:{keywords:[{keyword,count}] (top 30 by count, then by name), lenses:[string], albums:[name]}` |
  | `GET /api/photos` | Query: `event`, `q`, `keyword` (repeatable, AND), `rating` (repeatable; `0`-`4` or `none`), `tier` (repeatable), `min_score`, `keeper=1`, `lens`, `album`, `sort` (`taken` default, `score`, `rating`), `offset` (default 0), `limit` (default 60, max 200). Returns `{items:[Photo], total}`. |
  | `GET /api/photos/{id}` | A `Photo` plus `prev_id`, `next_id`: neighbors in the event's `taken` order after dedupe, null at the ends. |

- **`Photo`** = `{id, event_slug, album, url, width, height, taken_at, rating, focus_tier, quality_score, keeper, subject, composition, action, description, keywords, adjectives, exif}`. The metadata fields come from the joined `source_photos` row and are null (keywords/adjectives `[]`, exif `{}`) when unmatched.
- **Dedupe:** within an event, the same picture can be in more than one album (for example Highlights and All Photos). Return one row per `COALESCE('s'||source_photo_id, 'm'||media_key)`, keeping the album with the lowest `position` (`DISTINCT ON`, then the outer sort). Without `event`, search all published events.
- **Search and filters:** `q` uses `source_photos.search @@ websearch_to_tsquery('english', $q)`. `keyword` uses `keywords @> $arr`. `lens` matches `exif->>'lens'`. Rows without a source fail every metadata filter except `rating=none`.
- **Tests** (httptest + dbtest), with fixture rows inserted through `store`:
  1. `q=carving` returns only the matched photo whose action is carving.
  2. A hidden photo is absent.
  3. An unpublished event → 404.
  4. A photo in two albums with the same `source_photo_id` appears once.
  5. `rating=none` returns the unmatched photo.
  6. `prev_id`/`next_id` follow `taken_at`.

### 11. Old events (wave D; needs step 6)

- **Seed file:** `internal/legacy/events.json` (embedded). It has the 6 files `content/events/{OCSF4,OCSF5,OCSF6,FLF7,FLF8,FLFX}.md` of `mononen/photo-gallery` (read with `gh api repos/mononen/photo-gallery/contents/content/events/<F>.md --jq .content | base64 -d`), converted by hand to `[{"slug","title","series","event_date","description","albums":[{"name","share_url"}]}]`:
  - slug = lowercase file stem (`ocsf6`);
  - series = frontmatter `event`;
  - event_date = frontmatter `date`;
  - description = frontmatter `description` with `\n` kept;
  - albums = frontmatter `albums` (`name`, `url`). Drop `thumbnails`; the covers come from the synced photos.
- **CLI:** `photo-gallery seed-legacy`:
  1. For each entry, `UpsertEvent` with `published: true`.
  2. Run `albumsync.Run` for each, one after another. Print one summary line per event. A failed sync of one event is printed and does not stop the others; the exit code is 1 if any failed.
- **Test:** a store test that the embedded JSON decodes into 6 entries, each with at least one album and a valid slug and date.

### 12. Deploy and real-data run (wave E; needs all of the above merged)

1. Confirm that CI deployed `master`. Then run `kubectl -n production exec deploy/photo-gallery-api -- photo-gallery seed-legacy`.
2. Merge and deploy the photosort PR (its CI deploys the new `PHOTOSORT_GALLERY` env). Then run `kubectl -n production exec deploy/photosort-api -c photosort-api -- photosort publish --event ocsf6 --folder "RAW/Oak City Shred Fest/2026"`.
3. Write the printed summary in `docs/DEVLOG.md`.
4. Check 10 photos with `match_method` `time` or `time_hash` in `https://gallery-admin.adoah.dev/admin/events/ocsf6`:
   - In a browser tab, compare each album `=w400` image with photosort's `/media/thumb/<source_id>` (photosort UI `https://photosort.adoah.dev`).
   - Record the result in the ticket.
5. Add the per-event runbook to `docs/DEPLOY.md`:
   1. Upload the Lightroom exports to a Google Photos album and make a public share link.
   2. Create the event and paste the link at `gallery-admin.adoah.dev`.
   3. Run `photosort publish --event <slug> --folder <RAW folder>` in the photosort-api pod.
   4. Check the sync result.
   5. Set Published.

### 13. Human gate, then the public ingress (wave F)

1. Stop and ask the user to approve the public launch. Show the counts from step 12, the spot-check result, and screenshots of `/`, `/events/ocsf6`, and one photo detail page at `gallery.adoah.dev`.
2. After approval, branch, set `publicIngress.enabled: true` in `.ci/chart/values.yaml`, and use the step 0 PR flow.
3. **Done when:**
   - `curl -s -o /dev/null -w '%{http_code}' https://gallery.monomotion.org/` returns `200`.
   - `curl -s https://gallery.monomotion.org/api/events | jq length` returns `6`.
   - `https://gallery.monomotion.org/admin-api/events` and `https://gallery.monomotion.org/admin` both return `404`.
4. Close the epic.

## Critical files & anchors

- `~/mononendev/photosort/internal/api/server.go`: `handler`, `errf`, `wrap`, `writeJSON`, the `route(...)` registration. Copy this HTTP pattern.
- `~/mononendev/photosort/cmd/photosort/sort.go` `sortCmd` and `dbcmd.go` `exportRow`: the template for `publishCmd` (rows → `export.FinalRecord`).
- `~/mononendev/photosort/internal/db/db.go` `RowsUnder` (~line 186) and `internal/exif/exif.go` `fields`: folder selection and the `taken` text format `YYYY:MM:DD HH:MM:SS`.
- `~/mononendev/photosort/.github/workflows/ci.yml` and `.ci/chart/values.yaml`: the CI and chart to copy. Do not copy the chart's `className: nginx` ingress; the homelab only has `internal`/`external` Traefik classes, as in `~/mononendev/stasharr/.ci/chart/values.yaml`.
- `~/homelab/kubernetes/apps/production/photo-gallery/` and `~/homelab/scripts/pg-rotate-password.sh`: the old release to remove and the secret tool.

## Verification

1. **Per ticket:** its "Done when" checks. Gallery Go tests run with a local DB:
   ```bash
   cd ~/mononendev/photo-gallery
   make db
   GALLERY_TEST_PG='postgres://postgres:postgres@localhost:5433/gallery?sslmode=disable' make test
   ```
   CI runs the same tests with `GALLERY_REQUIRE_PG=1`.
2. **photosort:**
   ```bash
   cd ~/mononendev/photosort
   go test ./internal/dhash/ ./cmd/photosort/ -run 'Hash|Publish'
   ```
3. **Local end-to-end** before step 12 (the orchestrator does this once after wave D):
   - Run `make db`, `make dev`, and `make web`.
   - Run `GALLERY_DB=... go run ./cmd/photo-gallery seed-legacy`. Expected: 6 summary lines, and `ocsf6` shows at least 320 album photos (the "OCSF6 Selects" album has 320).
   - In a browser at `http://localhost:5173/`, expect 6 event cards with covers.
   - On `/events/ocsf6`, expect a grid of Google-hosted images and working "Load more".
   - A photo detail page shows only the image, because no photosort sources exist locally.
   - `http://localhost:5173/admin` lists the 6 events with `sync_state` `done`.
4. **Production:** steps 12 and 13. The new behavior to prove:
   - `photosort publish` prints a summary with `matched by time` > 0 for OCSF6.
   - `/api/photos?event=ocsf6&q=<a keyword from one matched photo>` returns that photo with its description and rating.

## Assumptions & contingencies

- **Scope:** gallery, event pages, tag search, admin, and the old-gallery migration only. Faces, similar faces, name claims, opt-out, and Redis come later. Postgres does any caching need; there is no Redis.
- **Hosts:** the public host is `gallery.monomotion.org` (takes over the old app). LAN hosts are `gallery.adoah.dev` (public UI) and `gallery-admin.adoah.dev` (admin).
- **Access:** admin pages and `/admin-api` have no login and are reachable only through the internal ingress and the in-cluster Service, the same model as photosort.
- **Repo:** `mononendev/photo-gallery` is public with AGPL-3.0, like photosort. If the user wants it private, run `gh repo edit --visibility private`; nothing else changes.
- **Album files:** they are Lightroom exports that keep EXIF `DateTimeOriginal`. Album photos with no photosort match are still shown, without metadata.
- **Album page format changes** (parse errors in step 5): probe the live album again with the step 5 procedure, update the indices and fixtures, and record the new format in `docs/DEVLOG.md`.
- **Time offsets (measured 2026-11-25, OCSF6):** 300 album items ("OCSF6 Selects") against 8386 photosort rows under `RAW/Oak City Shred Fest/2026`:
  - 286 items have a real date; all 286 have a photosort row at the same second with **no** offset. The hits at −1 h/+1 h/−2 h are coincidences inside bursts. No offset correction is needed.
  - 14 items are dated 2015-01-01: they are Sony A7RII `_DSC*.ARW` frames from a camera with an unset clock. photosort stores the same wrong date, so they still match on the second.
  - Seconds alone are ambiguous: 260 of the 286 items have 3 or more photosort rows in the same second (bursts, up to 14 per second); 19 have 2; only 7 have 1. That is why the matcher uses `SubSecTimeOriginal` (the album keeps milliseconds: 284 of 300 non-zero).
  - **SubSec check (2026-11-25, exiftool in the photosort-api pod):** 8 Canon album items spread over the album, with 3-8 same-second CR2 frames each. In all 8, exactly one CR2 has `SubSecTimeOriginal` × 10 equal to the album millisecond value (for example album 940 → `94`, neighbors `76`, `84`). The album milliseconds are the EXIF sub-seconds, so the Canon bursts match exactly.
  - Sony ARW files have no `SubSecTimeOriginal`, and the album shows `000` for them. Sony bursts (2-5 frames per second in the sample) fall to `time_hash`. Near-identical burst frames can pick the wrong neighbor there. The admin page marks these as `time_hash`, and step 12 spot-checks 10 Sony `time_hash` matches separately.
- **Certificates and DNS:** if cert-manager or external-dns does not issue for `gallery.monomotion.org` in step 13, compare with the removed old HelmRelease (git history of `~/homelab/kubernetes/apps/production/photo-gallery/app/helmrelease.yaml`, which used the same class and annotation). Report to the user at the gate instead of changing the hostname.
