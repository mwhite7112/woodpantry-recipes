package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/events"
	"github.com/mwhite7112/woodpantry-recipes/internal/mocks"
	"github.com/mwhite7112/woodpantry-recipes/internal/service"
)

func TestHandleRecipeImportedEvent_DefaultsToStaged(t *testing.T) {
	t.Parallel()

	mockQ := mocks.NewMockQuerier(t)
	svc := service.New(mockQ, nil, nil, nil)

	jobID := uuid.New()
	staged := service.StagedRecipe{
		Title: "Pasta",
		Ingredients: []service.StagedIngredient{
			{Name: "salt"},
		},
	}
	stagedJSON, err := json.Marshal(staged)
	require.NoError(t, err)
	stagedRaw := json.RawMessage(stagedJSON)

	mockQ.EXPECT().UpdateIngestionJobStaged(
		mock.Anything,
		db.UpdateIngestionJobStagedParams{
			ID:         jobID,
			StagedData: &stagedRaw,
		},
	).Return(db.IngestionJob{ID: jobID, Status: "staged"}, nil)

	err = svc.HandleRecipeImportedEvent(context.Background(), events.RecipeImportedEvent{
		JobID:      jobID,
		StagedData: stagedJSON,
	})
	require.NoError(t, err)
}

func TestHandleRecipeImportedEvent_FailedStatus(t *testing.T) {
	t.Parallel()

	mockQ := mocks.NewMockQuerier(t)
	svc := service.New(mockQ, nil, nil, nil)

	jobID := uuid.New()
	mockQ.EXPECT().UpdateIngestionJobStatus(
		mock.Anything,
		db.UpdateIngestionJobStatusParams{
			ID:     jobID,
			Status: "failed",
		},
	).Return(db.IngestionJob{ID: jobID, Status: "failed"}, nil)

	err := svc.HandleRecipeImportedEvent(context.Background(), events.RecipeImportedEvent{
		JobID:  jobID,
		Status: "failed",
		Error:  "llm timeout",
	})
	require.NoError(t, err)
}

func TestHandleRecipeImportedEvent_UnknownJob(t *testing.T) {
	t.Parallel()

	mockQ := mocks.NewMockQuerier(t)
	svc := service.New(mockQ, nil, nil, nil)

	jobID := uuid.New()
	stagedJSON := json.RawMessage(`{"title":"Soup","ingredients":[{"name":"water"}]}`)
	stagedRaw := stagedJSON
	mockQ.EXPECT().UpdateIngestionJobStaged(
		mock.Anything,
		db.UpdateIngestionJobStagedParams{
			ID:         jobID,
			StagedData: &stagedRaw,
		},
	).Return(db.IngestionJob{}, sql.ErrNoRows)

	err := svc.HandleRecipeImportedEvent(context.Background(), events.RecipeImportedEvent{
		JobID:      jobID,
		Status:     "staged",
		StagedData: stagedJSON,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
