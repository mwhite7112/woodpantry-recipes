# woodpantry-recipes

Recipe Service for WoodPantry. Owns the recipe corpus and staged recipe ingest review flow. In Phase 2, extraction is async through RabbitMQ (Ingestion Pipeline), not direct OpenAI calls from this service.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/recipes` | List recipes (`?tags=italian&cook_time_max=45&title=pasta`) |
| POST | `/recipes` | Create a structured recipe directly |
| GET | `/recipes/:id` | Full recipe detail |
| PUT | `/recipes/:id` | Update recipe |
| DELETE | `/recipes/:id` | Delete recipe |
| POST | `/recipes/ingest` | Submit free text for async extraction (publishes `recipe.import.requested`) |
| GET | `/recipes/ingest/:job_id` | Check ingest status / get staged recipe for review |
| POST | `/recipes/ingest/:job_id/confirm` | Commit staged recipe after review |
| POST | `/recipes/search` | Semantic search via natural language prompt (Phase 3) |

### POST /recipes/ingest

Accepts free-text recipe input, creates an `ingestion_jobs` row, and publishes `recipe.import.requested`. Returns immediately with a job ID for polling.

```json
// Request
{ "text": "Weeknight Pasta\n\nIngredients:\n- 2 cloves garlic\n- 1 lb pasta\n- olive oil, salt\n\nInstructions:\n1. Boil pasta. 2. Saute garlic in oil. 3. Combine." }

// Response
{ "ID": "uuid", "Status": "pending" }
```

### GET /recipes/ingest/:job_id

Returns the persisted `ingestion_jobs` record. When the ingestion worker publishes `recipe.imported`, this job is updated to `staged` with `staged_data`.

```json
{
  "ID": "uuid",
  "Type": "text_blob",
  "RawInput": "Weeknight Pasta...",
  "Status": "staged",
  "StagedData": {
    "title": "Weeknight Pasta",
    "ingredients": [
      { "name": "garlic", "ingredient_id": "uuid", "quantity": 2, "unit": "clove" }
    ],
    "steps": ["Boil pasta.", "Saute garlic in oil.", "Combine."]
  }
}
```

### POST /recipes/search (Phase 3)

```json
// Request
{ "prompt": "something warming and Italian, under 45 minutes" }
```

## Ingest Flow

```
POST /recipes/ingest
  → Create ingestion job (pending)
  → Publish recipe.import.requested
Ingestion Pipeline (async)
  → Extract + normalize recipe payload
  → Publish recipe.imported {job_id, status, staged_data}
Recipe Service subscriber
  → Update ingestion job to staged (or failed)
GET /recipes/ingest/:job_id     ← user reviews staged recipe
POST /recipes/ingest/:job_id/confirm
  → Recipe committed to DB
  → Ingredients resolved if needed (fallback when ingredient_id absent)
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_URL` | required | PostgreSQL `recipe_db` connection string |
| `DICTIONARY_URL` | required | Ingredient Dictionary service base URL |
| `RABBITMQ_URL` | optional | Enables publish/subscribe for async ingest (Phase 2+) |
| `LOG_LEVEL` | `info` | Log level |

## Development

### Prerequisites

- Go 1.23+
- Docker or Podman (for integration tests — testcontainers-go pulls Postgres automatically)
- [sqlc](https://sqlc.dev) (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)
- [mockery](https://vektra.github.io/mockery/) v2 (`go install github.com/vektra/mockery/v2@latest`)

### Local Setup

```bash
export DB_URL="postgres://user:pass@localhost:5432/recipe_db?sslmode=disable"
export DICTIONARY_URL="http://localhost:8081"
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export LOG_LEVEL=debug
```

### Run

```bash
go run ./cmd/recipes/main.go
```

### Test

```bash
make test                  # unit tests
make test-integration      # integration tests (requires Docker)
make test-all              # unit + integration
make test-coverage         # unit tests with coverage report
make test-coverage-html    # HTML coverage report (opens coverage.html)
```

### CI

- Pull requests run `.github/workflows/ci.yaml` with:
- blocking lint (`.golangci.yaml`)
- advisory lint (`.golangci-advisory.yaml`, non-blocking)
- Docker build validation
- unit tests and integration tests

### Code Generation

```bash
make sqlc                  # regenerate DB layer from SQL queries in internal/db/queries/
make generate-mocks        # regenerate mocks from interfaces via mockery
```
