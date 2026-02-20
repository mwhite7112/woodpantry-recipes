-- name: ListRecipes :many
SELECT id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at
FROM recipes
ORDER BY created_at DESC;

-- name: GetRecipe :one
SELECT id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at
FROM recipes WHERE id = $1;

-- name: CreateRecipe :one
INSERT INTO recipes (title, description, source_url, servings, prep_minutes, cook_minutes, tags)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at;

-- name: UpdateRecipe :one
UPDATE recipes
SET title = $2, description = $3, source_url = $4, servings = $5,
    prep_minutes = $6, cook_minutes = $7, tags = $8, updated_at = now()
WHERE id = $1
RETURNING id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at;

-- name: DeleteRecipe :exec
DELETE FROM recipes WHERE id = $1;

-- name: ListRecipesByTag :many
SELECT id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at
FROM recipes
WHERE $1 = ANY(tags)
ORDER BY created_at DESC;

-- name: ListRecipesByCookTime :many
SELECT id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at
FROM recipes
WHERE cook_minutes <= $1
ORDER BY created_at DESC;

-- name: ListRecipesByTitle :many
SELECT id, title, description, source_url, servings, prep_minutes, cook_minutes, tags, created_at, updated_at
FROM recipes
WHERE title ILIKE '%' || $1 || '%'
ORDER BY created_at DESC;
