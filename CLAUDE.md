# woodpantry-recipes — Recipe Service

## Role in Architecture

Owns the recipe corpus. Responsible for CRUD on hand-crafted recipes, staged free-text ingest review, and semantic search via pgvector embeddings. This service is the source of truth for what recipes exist in the system.

During ingest, every recipe ingredient is resolved through the Ingredient Dictionary via `POST /ingredients/resolve` before being stored — ensuring all recipe ingredients reference canonical Dictionary IDs, never raw strings.

All ingest flows follow the **staged commit pattern**: free text in → async extraction via Ingestion Pipeline → staged result for review → user confirms → structured recipe committed.

## Technology

- Language: Go
- HTTP: chi
- Database: PostgreSQL (`recipe_db`) via sqlc
- pgvector extension (Phase 3): `embedding vector(1536)` on `recipes` table for semantic search
- RabbitMQ (Phase 2+): publishes `recipe.import.requested`, subscribes to `recipe.imported`
- LLM: no direct LLM calls in this service during Phase 2; Ingestion Pipeline handles extraction

## Service Dependencies

- **Calls**: Ingredient Dictionary (`/ingredients/resolve` per recipe ingredient on ingest)
- **Called by**: Matching Service (recipe list + ingredient data), Shopping List Service (recipe ingredients)
- **Publishes** (Phase 2+): `recipe.import.requested`
- **Subscribes to** (Phase 2+): `recipe.imported`

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/recipes` | List recipes — filters: `tags`, `cook_time_max`, `title` |
| POST | `/recipes` | Create a hand-crafted recipe (structured JSON) |
| GET | `/recipes/:id` | Full recipe detail |
| PUT | `/recipes/:id` | Update recipe |
| DELETE | `/recipes/:id` | Delete recipe |
| POST | `/recipes/ingest` | Submit free text for async extraction |
| GET | `/recipes/ingest/:job_id` | Check ingest job status / get staged recipe |
| POST | `/recipes/ingest/:job_id/confirm` | Commit staged recipe after review |
| POST | `/recipes/search` | Semantic search (Phase 3) — body: natural language prompt |

## Key Patterns

### Staged Ingest
`POST /recipes/ingest` does not immediately create a recipe. It creates an `IngestionJob` with status `pending` and publishes `recipe.import.requested`. The Ingestion Pipeline later publishes `recipe.imported`, and the local subscriber updates the job to `staged` (or `failed`). The user reviews via `GET /recipes/ingest/:job_id` and confirms via the confirm endpoint. Only then is the recipe committed.

### Write-Through to Dictionary
At confirm time, ingredients are persisted as canonical IDs. If `ingredient_id` is already present in staged data (resolved by Ingestion Pipeline), it is used directly. Otherwise, fallback resolution calls `POST /ingredients/resolve` by name.

### Embeddings (Phase 3)
After a recipe is committed, a background goroutine generates an embedding via the OpenAI API and stores it in the `embedding` column. The recipe is fully usable before the embedding is ready. `POST /recipes/search` uses pgvector cosine similarity to rank results.

## Data Models

```
recipes
  id              UUID  PK
  title           TEXT
  description     TEXT
  source_url      TEXT  NULLABLE
  servings        INT
  prep_minutes    INT
  cook_minutes    INT
  tags            TEXT[]
  embedding       VECTOR(1536)  NULLABLE  -- pgvector, Phase 3
  created_at      TIMESTAMPTZ
  updated_at      TIMESTAMPTZ

recipe_steps
  id              UUID  PK
  recipe_id       UUID  FK
  step_number     INT
  instruction     TEXT

recipe_ingredients
  id                  UUID  PK
  recipe_id           UUID  FK
  ingredient_id       UUID  -- canonical ID from Dictionary
  quantity            FLOAT8
  unit                TEXT
  is_optional         BOOL
  preparation_notes   TEXT  NULLABLE

ingestion_jobs
  id              UUID  PK
  type            TEXT  -- text_blob|url
  raw_input       TEXT
  status          TEXT  -- pending|processing|staged|confirmed|failed
  staged_data     JSONB -- extracted recipe before commit
  created_at      TIMESTAMPTZ
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_URL` | required | PostgreSQL connection string for `recipe_db` |
| `DICTIONARY_URL` | required | Base URL of Ingredient Dictionary service |
| `OPENAI_API_KEY` | optional | Reserved for Phase 3 embedding/search work |
| `EXTRACT_MODEL` | n/a | Extraction moved to Ingestion Pipeline in Phase 2 |
| `EMBED_MODEL` | `text-embedding-3-small` | OpenAI embedding model (Phase 3) |
| `RABBITMQ_URL` | optional | If set, enables async ingest via queue (Phase 2+) |
| `LOG_LEVEL` | `info` | Log level |

## Directory Layout

```
woodpantry-recipes/
├── cmd/recipes/main.go
├── internal/
│   ├── api/
│   │   ├── handlers.go
│   │   └── ingest.go          ← staged ingest handler logic
│   ├── db/
│   │   ├── migrations/
│   │   ├── queries/
│   │   └── sqlc.yaml
│   ├── service/
│   │   ├── service.go
│   │   ├── ingest.go          ← staged commit flow + resolve fallback
│   │   └── ingest_events.go   ← recipe.imported event handling
│   └── events/
│       ├── publisher.go       ← publish recipe.import.requested (Phase 2+)
│       └── subscriber.go      ← consume recipe.imported (Phase 2+)
├── kubernetes/
├── Dockerfile
├── go.mod
└── go.sum
```

## Testing

```bash
make test                # Unit tests
make test-integration    # Integration tests (requires Docker)
make test-coverage       # Unit tests with coverage
make generate-mocks      # Regenerate mocks from .mockery.yaml
make sqlc                # Regenerate sqlc
```

### CI Checks

- `.github/workflows/ci.yaml` runs blocking lint (`.golangci.yaml`)
- `.github/workflows/ci.yaml` runs advisory lint (`.golangci-advisory.yaml`) as non-blocking
- `.github/workflows/ci.yaml` runs Docker build, unit tests, and integration tests

- Unit tests: `internal/service/` (DictionaryResolver, ingest event handling), `internal/api/` (list, get, delete, ingest validation, job status)
- Integration tests: `internal/api/` (full CRUD cycle, list by tag with real Postgres)
- Mocks: `internal/mocks/` (Querier), `internal/service/` (LLMExtractor, IngredientResolver — in-package to avoid import cycle)
- Service uses `db.Querier`, publisher/subscriber interfaces, and `IngredientResolver` for testability
- Handlers that use `svc.DB().BeginTx()` (create, update, confirm) are covered by integration tests

## What to Avoid

- Do not store raw ingredient strings in `recipe_ingredients` — always resolve to a Dictionary ID first.
- Do not skip the staged review step — the LLM makes mistakes and users need to verify.
- Do not re-introduce direct extraction calls in this service during Phase 2+.
- Do not block recipe creation on embedding generation — embeddings are generated asynchronously.
- Do not replicate Ingredient Dictionary data locally — always call `/ingredients/resolve` on ingest.
