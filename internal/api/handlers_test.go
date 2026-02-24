package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/mocks"
	"github.com/mwhite7112/woodpantry-recipes/internal/service"
)

// stubExtractor is a no-op LLM extractor for handler tests.
type stubExtractor struct{}

func (e *stubExtractor) ExtractRecipe(_ context.Context, _ string) (*service.StagedRecipe, error) {
	return &service.StagedRecipe{Title: "stub"}, nil
}

// stubResolver is a no-op ingredient resolver for handler tests.
type stubResolver struct{}

func (s *stubResolver) ResolveIngredient(_ context.Context, _ string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func setupRouter(t *testing.T) (*mocks.MockQuerier, http.Handler) {
	t.Helper()
	mockQ := mocks.NewMockQuerier(t)
	svc := service.New(mockQ, nil, &stubExtractor{}, &stubResolver{})
	router := NewRouter(svc)
	return mockQ, router
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	_, router := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestListRecipes_Default(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	now := time.Now()
	recipes := []db.Recipe{
		{
			ID:        uuid.New(),
			Title:     "Pasta",
			Tags:      []string{"dinner"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	mockQ.EXPECT().ListRecipes(mock.Anything).Return(recipes, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Len(t, got, 1)
	assert.Equal(t, "Pasta", got[0]["Title"])
}

func TestListRecipes_ByTag(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	mockQ.EXPECT().ListRecipesByTag(mock.Anything, []string{"vegan"}).Return([]db.Recipe{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes?tag=vegan", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListRecipes_ByCookTime(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	mockQ.EXPECT().
		ListRecipesByCookTime(mock.Anything, sql.NullInt32{Int32: 30, Valid: true}).
		Return([]db.Recipe{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes?cook_time_max=30", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListRecipes_ByCookTime_Invalid(t *testing.T) {
	t.Parallel()
	_, router := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/recipes?cook_time_max=abc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListRecipes_ByTitle(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	mockQ.EXPECT().
		ListRecipesByTitle(mock.Anything, sql.NullString{String: "pasta", Valid: true}).
		Return([]db.Recipe{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes?title=pasta", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGetRecipe_Success(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	id := uuid.New()
	now := time.Now()
	recipe := db.Recipe{
		ID:        id,
		Title:     "Pasta Carbonara",
		Tags:      []string{"dinner"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	steps := []db.RecipeStep{
		{ID: uuid.New(), RecipeID: id, StepNumber: 1, Instruction: "Boil pasta"},
	}
	ingredients := []db.RecipeIngredient{
		{ID: uuid.New(), RecipeID: id, IngredientID: uuid.New()},
	}

	mockQ.EXPECT().GetRecipe(mock.Anything, id).Return(recipe, nil)
	mockQ.EXPECT().ListStepsByRecipe(mock.Anything, id).Return(steps, nil)
	mockQ.EXPECT().ListIngredientsByRecipe(mock.Anything, id).Return(ingredients, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes/"+id.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Pasta Carbonara", got["Title"])
	assert.Len(t, got["steps"], 1)
	assert.Len(t, got["ingredients"], 1)
}

func TestGetRecipe_NotFound(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	id := uuid.New()
	mockQ.EXPECT().GetRecipe(mock.Anything, id).Return(db.Recipe{}, sql.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/recipes/"+id.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetRecipe_InvalidID(t *testing.T) {
	t.Parallel()
	_, router := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/recipes/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteRecipe_Success(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	id := uuid.New()
	mockQ.EXPECT().DeleteRecipe(mock.Anything, id).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/recipes/"+id.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestDeleteRecipe_NotFound(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	id := uuid.New()
	mockQ.EXPECT().DeleteRecipe(mock.Anything, id).Return(sql.ErrNoRows)

	req := httptest.NewRequest(http.MethodDelete, "/recipes/"+id.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetIngestJob_Success(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	jobID := uuid.New()
	now := time.Now()
	job := db.IngestionJob{
		ID:        jobID,
		Type:      "text_blob",
		RawInput:  "recipe text",
		Status:    "staged",
		CreatedAt: now,
	}
	mockQ.EXPECT().GetIngestionJob(mock.Anything, jobID).Return(job, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes/ingest/"+jobID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGetIngestJob_NotFound(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	jobID := uuid.New()
	mockQ.EXPECT().GetIngestionJob(mock.Anything, jobID).Return(db.IngestionJob{}, sql.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/recipes/ingest/"+jobID.String(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPostIngest_MissingText(t *testing.T) {
	t.Parallel()
	_, router := setupRouter(t)

	body := `{"text":""}`
	req := httptest.NewRequest(http.MethodPost, "/recipes/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var errBody map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Contains(t, errBody["error"], "text is required")
}

func TestPostIngest_InvalidBody(t *testing.T) {
	t.Parallel()
	_, router := setupRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/recipes/ingest", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConfirmIngest_NotStaged(t *testing.T) {
	t.Parallel()
	mockQ, router := setupRouter(t)

	jobID := uuid.New()
	now := time.Now()
	mockQ.EXPECT().GetIngestionJob(mock.Anything, jobID).Return(db.IngestionJob{
		ID:        jobID,
		Type:      "text_blob",
		RawInput:  "test",
		Status:    "pending",
		CreatedAt: now,
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/recipes/ingest/"+jobID.String()+"/confirm", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}
