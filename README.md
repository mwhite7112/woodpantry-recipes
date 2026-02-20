# woodpantry-recipes

Recipe Service for WoodPantry. Owns the recipe corpus — CRUD, free-text LLM ingest, and semantic search. All recipe ingredients are normalized against the Ingredient Dictionary.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/recipes` | List recipes (`?tags=italian&cook_time_max=45&title=pasta`) |
| POST | `/recipes` | Create a structured recipe directly |
| GET | `/recipes/:id` | Full recipe detail |
| PUT | `/recipes/:id` | Update recipe |
| DELETE | `/recipes/:id` | Delete recipe |
| POST | `/recipes/ingest` | Submit free text or URL for async LLM extraction |
| GET | `/recipes/ingest/:job_id` | Check ingest status / get staged recipe for review |
| POST | `/recipes/ingest/:job_id/confirm` | Commit staged recipe after review |
| POST | `/recipes/search` | Semantic search via natural language prompt (Phase 3) |

### POST /recipes/ingest

Accepts a free-text recipe body (as you'd write it in a notes app) or a URL. Triggers LLM extraction and returns a job ID for polling.

```json
// Request
{
  "type": "text_blob",
  "content": "Weeknight Pasta\n\nIngredients:\n- 2 cloves garlic\n- 1 lb pasta\n- olive oil, salt\n\nInstructions:\n1. Boil pasta. 2. Sauté garlic in oil. 3. Combine."
}

// Response
{ "job_id": "uuid", "status": "pending" }
```

### GET /recipes/ingest/:job_id

Returns the staged recipe when extraction is complete. Review this before confirming.

```json
{
  "job_id": "uuid",
  "status": "staged",
  "recipe": {
    "title": "Weeknight Pasta",
    "ingredients": [
      { "raw_text": "garlic", "ingredient_id": "uuid", "quantity": 2, "unit": "clove" }
    ],
    "steps": ["Boil pasta.", "Sauté garlic in oil.", "Combine."]
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
  → LLM extracts structured recipe
  → Each ingredient resolved via POST /ingredients/resolve
  → Staged as IngestionJob
GET /recipes/ingest/:job_id     ← user reviews staged recipe
POST /recipes/ingest/:job_id/confirm
  → Recipe committed to DB
  → Embedding generated in background (Phase 3)
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DB_URL` | required | PostgreSQL `recipe_db` connection string |
| `DICTIONARY_URL` | required | Ingredient Dictionary service base URL |
| `OPENAI_API_KEY` | required | OpenAI API key (extraction + embeddings) |
| `EXTRACT_MODEL` | `gpt-5-mini` | OpenAI model for text extraction |
| `EMBED_MODEL` | `text-embedding-3-small` | OpenAI embedding model (Phase 3) |
| `RABBITMQ_URL` | optional | Enables async queue-based ingest (Phase 2+) |
| `LOG_LEVEL` | `info` | Log level |

## Development

```bash
go run ./cmd/recipes/main.go
sqlc generate -f internal/db/sqlc.yaml
```
