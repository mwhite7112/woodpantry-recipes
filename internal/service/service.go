package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/mwhite7112/woodpantry-recipes/internal/db"
)

// LLMExtractor abstracts LLM-based recipe extraction for testing.
type LLMExtractor interface {
	ExtractRecipe(ctx context.Context, rawText string) (*StagedRecipe, error)
}

// IngredientResolver abstracts ingredient resolution via the Dictionary for testing.
type IngredientResolver interface {
	ResolveIngredient(ctx context.Context, name string) (uuid.UUID, error)
}

// Service holds dependencies for recipe business logic.
type Service struct {
	q         db.Querier
	sqlDB     *sql.DB
	extractor LLMExtractor
	resolver  IngredientResolver
}

func New(q db.Querier, sqlDB *sql.DB, extractor LLMExtractor, resolver IngredientResolver) *Service {
	return &Service{
		q:         q,
		sqlDB:     sqlDB,
		extractor: extractor,
		resolver:  resolver,
	}
}

func (s *Service) Queries() db.Querier    { return s.q }
func (s *Service) DB() *sql.DB            { return s.sqlDB }
func (s *Service) Extractor() LLMExtractor { return s.extractor }

// ExtractRecipe delegates to the configured LLM extractor.
func (s *Service) ExtractRecipe(ctx context.Context, rawText string) (*StagedRecipe, error) {
	return s.extractor.ExtractRecipe(ctx, rawText)
}
