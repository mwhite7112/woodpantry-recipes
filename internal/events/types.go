package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	exchangeName = "woodpantry.topic"

	recipeImportRequestedRoutingKey = "recipe.import.requested"
	recipeImportedRoutingKey        = "recipe.imported"
)

// RecipeImportRequestedEvent is published by Recipe Service when an ingest job is submitted.
type RecipeImportRequestedEvent struct {
	JobID     uuid.UUID `json:"job_id"`
	JobType   string    `json:"job_type"`
	RawInput  string    `json:"raw_input"`
	Timestamp string    `json:"timestamp"`
}

func NewRecipeImportRequestedEvent(jobID uuid.UUID, jobType, rawInput string) RecipeImportRequestedEvent {
	return RecipeImportRequestedEvent{
		JobID:     jobID,
		JobType:   jobType,
		RawInput:  rawInput,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// RecipeImportedEvent is published by Ingestion Pipeline after extraction.
// Status is expected to be "staged" (default if empty) or "failed".
type RecipeImportedEvent struct {
	JobID      uuid.UUID       `json:"job_id"`
	Status     string          `json:"status,omitempty"`
	Error      string          `json:"error,omitempty"`
	StagedData json.RawMessage `json:"staged_data,omitempty"`
}
