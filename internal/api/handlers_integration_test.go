//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/service"
	"github.com/mwhite7112/woodpantry-recipes/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExtractorIntegration is a no-op extractor for integration tests.
type stubExtractorIntegration struct{}

func (e *stubExtractorIntegration) ExtractRecipe(_ context.Context, _ string) (*service.StagedRecipe, error) {
	return &service.StagedRecipe{Title: "test"}, nil
}

// stubResolverIntegration is a no-op resolver for integration tests.
type stubResolverIntegration struct{}

func (r *stubResolverIntegration) ResolveIngredient(_ context.Context, _ string) (uuid.UUID, error) {
	return uuid.New(), nil
}

type trackingResolverIntegration struct {
	resolvedID uuid.UUID
	names      []string
	err        error
}

func (r *trackingResolverIntegration) ResolveIngredient(_ context.Context, name string) (uuid.UUID, error) {
	r.names = append(r.names, name)
	if r.err != nil {
		return uuid.UUID{}, r.err
	}
	return r.resolvedID, nil
}

func setupIntegrationRouter(t *testing.T, resolver service.IngredientResolver) http.Handler {
	t.Helper()
	sqlDB := testutil.SetupDB(t)
	q := db.New(sqlDB)
	if resolver == nil {
		resolver = &stubResolverIntegration{}
	}
	svc := service.New(q, sqlDB, &stubExtractorIntegration{}, resolver)
	return NewRouter(svc)
}

func TestIntegration_CRUDCycle(t *testing.T) {
	router := setupIntegrationRouter(t, nil)

	ingredientID := uuid.New()

	// Create a recipe.
	createBody := `{
		"title": "Pasta Carbonara",
		"description": "Classic Italian pasta",
		"servings": 4,
		"prep_minutes": 10,
		"cook_minutes": 20,
		"tags": ["dinner", "italian"],
		"steps": [
			{"step_number": 1, "instruction": "Boil pasta"},
			{"step_number": 2, "instruction": "Cook bacon"}
		],
		"ingredients": [
			{"ingredient_id": "` + ingredientID.String() + `", "quantity": 400, "unit": "g"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "Pasta Carbonara", created["title"])
	assert.NotContains(t, created, "Title")
	recipeID := created["id"].(string)

	// Get the recipe.
	req = httptest.NewRequest(http.MethodGet, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var detail map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "Pasta Carbonara", detail["title"])
	assert.NotContains(t, detail, "Title")
	assert.Len(t, detail["steps"], 2)
	assert.Len(t, detail["ingredients"], 1)

	steps := detail["steps"].([]interface{})
	firstStep := steps[0].(map[string]interface{})
	assert.Equal(t, float64(1), firstStep["step_number"])
	assert.NotContains(t, firstStep, "StepNumber")

	ingredients := detail["ingredients"].([]interface{})
	firstIngredient := ingredients[0].(map[string]interface{})
	assert.Equal(t, ingredientID.String(), firstIngredient["ingredient_id"])
	assert.NotContains(t, firstIngredient, "IngredientID")

	// List recipes.
	req = httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, 1)
	assert.Equal(t, "Pasta Carbonara", list[0]["title"])
	assert.NotContains(t, list[0], "Title")

	// Update the recipe.
	updateBody := `{
		"title": "Pasta Carbonara Updated",
		"servings": 6,
		"tags": ["dinner"],
		"steps": [
			{"step_number": 1, "instruction": "Boil water"},
			{"step_number": 2, "instruction": "Cook pasta"},
			{"step_number": 3, "instruction": "Mix"}
		],
		"ingredients": [
			{"ingredient_id": "` + ingredientID.String() + `", "quantity": 500, "unit": "g"}
		]
	}`
	req = httptest.NewRequest(http.MethodPut, "/recipes/"+recipeID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var updated map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Pasta Carbonara Updated", updated["title"])
	assert.NotContains(t, updated, "Title")

	// Verify update.
	req = httptest.NewRequest(http.MethodGet, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "Pasta Carbonara Updated", detail["title"])
	assert.Len(t, detail["steps"], 3)

	// Delete the recipe.
	req = httptest.NewRequest(http.MethodDelete, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deleted.
	req = httptest.NewRequest(http.MethodGet, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIntegration_ListByTag(t *testing.T) {
	router := setupIntegrationRouter(t, nil)

	// Create two recipes with different tags.
	createBody1 := `{
		"title": "Vegan Salad",
		"tags": ["vegan", "healthy"],
		"steps": [],
		"ingredients": []
	}`
	req := httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(createBody1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	createBody2 := `{
		"title": "Meat Stew",
		"tags": ["comfort", "meat"],
		"steps": [],
		"ingredients": []
	}`
	req = httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(createBody2))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// List by tag "vegan".
	req = httptest.NewRequest(http.MethodGet, "/recipes?tag=vegan", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, 1)
	assert.Equal(t, "Vegan Salad", list[0]["title"])
	assert.NotContains(t, list[0], "Title")

	// List by tag "nonexistent".
	req = httptest.NewRequest(http.MethodGet, "/recipes?tag=nonexistent", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Empty(t, list)
}

func TestIntegration_CreateRecipe_PreservesProvidedIngredientID(t *testing.T) {
	resolver := &trackingResolverIntegration{resolvedID: uuid.New()}
	router := setupIntegrationRouter(t, resolver)
	ingredientID := uuid.New()

	body := `{
		"title": "Structured ID Create",
		"steps": [{"step_number": 1, "instruction": "Mix"}],
		"ingredients": [
			{"ingredient_id": "` + ingredientID.String() + `", "name": "flour", "quantity": 1, "unit": "cup"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Empty(t, resolver.names)

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "Structured ID Create", created["title"])
	recipeID := created["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var detail recipeDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	require.Len(t, detail.Ingredients, 1)
	assert.Equal(t, ingredientID, detail.Ingredients[0].IngredientID)
}

func TestIntegration_CreateRecipe_ResolvesIngredientNameWhenIDAbsent(t *testing.T) {
	resolvedID := uuid.New()
	resolver := &trackingResolverIntegration{resolvedID: resolvedID}
	router := setupIntegrationRouter(t, resolver)

	body := `{
		"title": "Structured Name Create",
		"steps": [{"step_number": 1, "instruction": "Mix"}],
		"ingredients": [
			{"name": "flour", "quantity": 1, "unit": "cup"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, []string{"flour"}, resolver.names)

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "Structured Name Create", created["title"])
	recipeID := created["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/recipes/"+recipeID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var detail recipeDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	require.Len(t, detail.Ingredients, 1)
	assert.Equal(t, resolvedID, detail.Ingredients[0].IngredientID)
}

func TestIntegration_CreateRecipe_ReturnsServerErrorWhenIngredientResolveFails(t *testing.T) {
	resolver := &trackingResolverIntegration{err: errors.New("dictionary unavailable")}
	router := setupIntegrationRouter(t, resolver)

	body := `{
		"title": "Structured Resolve Failure",
		"steps": [{"step_number": 1, "instruction": "Mix"}],
		"ingredients": [
			{"name": "flour", "quantity": 1, "unit": "cup"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/recipes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"failed to resolve ingredient"}`, rec.Body.String())
	assert.Equal(t, []string{"flour"}, resolver.names)
}
