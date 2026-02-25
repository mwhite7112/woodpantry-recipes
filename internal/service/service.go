package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/events"
)

// LLMExtractor abstracts LLM-based recipe extraction for testing.
type LLMExtractor interface {
	ExtractRecipe(ctx context.Context, rawText string) (*StagedRecipe, error)
}

// IngredientResolver abstracts ingredient resolution via the Dictionary for testing.
type IngredientResolver interface {
	ResolveIngredient(ctx context.Context, name string) (uuid.UUID, error)
}

// ImportRequestPublisher publishes recipe.import.requested events.
type ImportRequestPublisher interface {
	PublishRecipeImportRequested(ctx context.Context, event events.RecipeImportRequestedEvent) error
}

// Service holds dependencies for recipe business logic.
type Service struct {
	q               db.Querier
	sqlDB           *sql.DB
	extractor       LLMExtractor
	resolver        IngredientResolver
	importPublisher ImportRequestPublisher
}

func New(
	q db.Querier,
	sqlDB *sql.DB,
	extractor LLMExtractor,
	resolver IngredientResolver,
	publishers ...ImportRequestPublisher,
) *Service {
	publisher := ImportRequestPublisher(noopImportRequestPublisher{})
	if len(publishers) > 0 && publishers[0] != nil {
		publisher = publishers[0]
	}

	return &Service{
		q:               q,
		sqlDB:           sqlDB,
		extractor:       extractor,
		resolver:        resolver,
		importPublisher: publisher,
	}
}

func (s *Service) Queries() db.Querier     { return s.q }
func (s *Service) DB() *sql.DB             { return s.sqlDB }
func (s *Service) Extractor() LLMExtractor { return s.extractor }

// ExtractRecipe delegates to the configured LLM extractor.
func (s *Service) ExtractRecipe(ctx context.Context, rawText string) (*StagedRecipe, error) {
	if s.extractor == nil {
		return nil, errors.New("llm extractor is not configured")
	}

	return s.extractor.ExtractRecipe(ctx, rawText)
}

func (s *Service) PublishRecipeImportRequested(ctx context.Context, job db.IngestionJob) error {
	event := events.NewRecipeImportRequestedEvent(job.ID, job.Type, job.RawInput)
	if err := s.importPublisher.PublishRecipeImportRequested(ctx, event); err != nil {
		return fmt.Errorf("publish recipe.import.requested event: %w", err)
	}

	return nil
}

type noopImportRequestPublisher struct{}

func (noopImportRequestPublisher) PublishRecipeImportRequested(
	_ context.Context,
	_ events.RecipeImportRequestedEvent,
) error {
	return nil
}
