package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/events"
)

// HandleRecipeImportedEvent applies queue results from the ingestion pipeline
// to the local ingestion_jobs table.
func (s *Service) HandleRecipeImportedEvent(ctx context.Context, event events.RecipeImportedEvent) error {
	status := strings.TrimSpace(event.Status)
	if status == "" {
		status = "staged"
	}

	switch status {
	case "failed":
		_, err := s.q.UpdateIngestionJobStatus(ctx, db.UpdateIngestionJobStatusParams{
			ID:     event.JobID,
			Status: "failed",
		})
		if err != nil {
			return fmt.Errorf("mark job failed: %w", err)
		}
		return nil
	case "staged":
		if len(event.StagedData) == 0 {
			return errors.New("recipe.imported event missing staged_data")
		}

		var staged StagedRecipe
		if err := json.Unmarshal(event.StagedData, &staged); err != nil {
			return fmt.Errorf("invalid staged_data payload: %w", err)
		}

		raw := event.StagedData
		_, err := s.q.UpdateIngestionJobStaged(ctx, db.UpdateIngestionJobStagedParams{
			ID:         event.JobID,
			StagedData: &raw,
		})
		if err != nil {
			return fmt.Errorf("stage imported recipe: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported recipe.imported status: %q", status)
	}
}
