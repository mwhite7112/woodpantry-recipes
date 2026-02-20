-- name: ListStepsByRecipe :many
SELECT id, recipe_id, step_number, instruction
FROM recipe_steps
WHERE recipe_id = $1
ORDER BY step_number;

-- name: CreateStep :one
INSERT INTO recipe_steps (recipe_id, step_number, instruction)
VALUES ($1, $2, $3)
RETURNING id, recipe_id, step_number, instruction;

-- name: DeleteStepsByRecipe :exec
DELETE FROM recipe_steps WHERE recipe_id = $1;
