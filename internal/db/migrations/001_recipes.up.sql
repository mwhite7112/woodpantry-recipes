CREATE TABLE IF NOT EXISTS recipes (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  title        TEXT        NOT NULL,
  description  TEXT,
  source_url   TEXT,
  servings     INT,
  prep_minutes INT,
  cook_minutes INT,
  tags         TEXT[]      NOT NULL DEFAULT '{}',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recipe_steps (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  recipe_id   UUID        NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  step_number INT         NOT NULL,
  instruction TEXT        NOT NULL,
  UNIQUE(recipe_id, step_number)
);

CREATE TABLE IF NOT EXISTS recipe_ingredients (
  id                UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
  recipe_id         UUID    NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  ingredient_id     UUID    NOT NULL,
  quantity          FLOAT8,
  unit              TEXT,
  is_optional       BOOL    NOT NULL DEFAULT false,
  preparation_notes TEXT
);

CREATE TABLE IF NOT EXISTS ingestion_jobs (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  type        TEXT        NOT NULL,
  raw_input   TEXT        NOT NULL,
  status      TEXT        NOT NULL DEFAULT 'pending',
  staged_data JSONB,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
