-- name: ListIngredientsByRecipe :many
SELECT id, recipe_id, ingredient_id, quantity, unit, is_optional, preparation_notes
FROM recipe_ingredients
WHERE recipe_id = $1;

-- name: CreateRecipeIngredient :one
INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit, is_optional, preparation_notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, recipe_id, ingredient_id, quantity, unit, is_optional, preparation_notes;

-- name: DeleteIngredientsByRecipe :exec
DELETE FROM recipe_ingredients WHERE recipe_id = $1;
