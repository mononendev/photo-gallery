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
  taken_ms SMALLINT,
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
CREATE INDEX source_photos_taken ON source_photos (event_id, taken, taken_ms);
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
