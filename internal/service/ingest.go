package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
)

// StagedIngredient is an ingredient as extracted by LLM before resolve.
type StagedIngredient struct {
	IngredientID     string  `json:"ingredient_id,omitempty"`
	Name             string  `json:"name"`
	Quantity         float64 `json:"quantity,omitempty"`
	Unit             string  `json:"unit,omitempty"`
	IsOptional       bool    `json:"is_optional,omitempty"`
	PreparationNotes string  `json:"preparation_notes,omitempty"`
}

// StagedRecipe is the LLM-extracted recipe before commit.
type StagedRecipe struct {
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	SourceURL   string             `json:"source_url,omitempty"`
	Servings    int                `json:"servings,omitempty"`
	PrepMinutes int                `json:"prep_minutes,omitempty"`
	CookMinutes int                `json:"cook_minutes,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Steps       []string           `json:"steps,omitempty"`
	Ingredients []StagedIngredient `json:"ingredients"`
}

// resolveResponse is the response body from POST /ingredients/resolve.
type resolveResponse struct {
	Ingredient struct {
		ID string `json:"id"`
	} `json:"ingredient"`
}

// DictionaryResolver implements IngredientResolver using the Dictionary HTTP API.
type DictionaryResolver struct {
	baseURL string
}

func NewDictionaryResolver(baseURL string) *DictionaryResolver {
	return &DictionaryResolver{baseURL: baseURL}
}

func (d *DictionaryResolver) ResolveIngredient(ctx context.Context, name string) (uuid.UUID, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.baseURL+"/ingredients/resolve",
		bytes.NewReader(body),
	)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("create resolve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("resolve request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return uuid.UUID{}, fmt.Errorf("resolve returned status %d", resp.StatusCode)
	}

	var rr resolveResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return uuid.UUID{}, fmt.Errorf("decode resolve response: %w", err)
	}

	id, err := uuid.Parse(rr.Ingredient.ID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parse ingredient id: %w", err)
	}
	return id, nil
}

// CommitStagedRecipe resolves all ingredients and persists the recipe.
func (s *Service) CommitStagedRecipe(ctx context.Context, job db.IngestionJob) (*db.Recipe, error) {
	var staged StagedRecipe
	if job.StagedData == nil {
		return nil, errors.New("staged data is nil")
	}
	if err := json.Unmarshal(*job.StagedData, &staged); err != nil {
		return nil, fmt.Errorf("unmarshal staged data: %w", err)
	}

	logger := slog.Default()
	logger.InfoContext(
		ctx,
		"committing staged recipe",
		"job_id",
		job.ID,
		"title",
		staged.Title,
		"ingredients",
		len(staged.Ingredients),
	)

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := db.New(tx)

	tags := staged.Tags
	if tags == nil {
		tags = []string{}
	}

	recipe, err := qtx.CreateRecipe(ctx, db.CreateRecipeParams{
		Title:       staged.Title,
		Description: nullString(staged.Description),
		SourceUrl:   nullString(staged.SourceURL),
		Servings:    nullInt32(staged.Servings),
		PrepMinutes: nullInt32(staged.PrepMinutes),
		CookMinutes: nullInt32(staged.CookMinutes),
		Tags:        tags,
	})
	if err != nil {
		return nil, fmt.Errorf("create recipe: %w", err)
	}

	for i, step := range staged.Steps {
		if _, err := qtx.CreateStep(ctx, db.CreateStepParams{
			RecipeID:    recipe.ID,
			StepNumber:  int32(i + 1),
			Instruction: step,
		}); err != nil {
			return nil, fmt.Errorf("create step %d: %w", i+1, err)
		}
	}

	for _, ing := range staged.Ingredients {
		var ingredientID uuid.UUID
		if ing.IngredientID != "" {
			ingredientID, err = uuid.Parse(ing.IngredientID)
			if err != nil {
				return nil, fmt.Errorf("parse ingredient_id %q: %w", ing.IngredientID, err)
			}
		} else {
			if s.resolver == nil {
				return nil, errors.New("ingredient resolver is not configured")
			}

			ingredientID, err = s.resolver.ResolveIngredient(ctx, ing.Name)
			if err != nil {
				return nil, fmt.Errorf("resolve ingredient %q: %w", ing.Name, err)
			}
		}

		logger.InfoContext(ctx, "resolved ingredient", "name", ing.Name, "ingredient_id", ingredientID)
		if _, err := qtx.CreateRecipeIngredient(ctx, db.CreateRecipeIngredientParams{
			RecipeID:         recipe.ID,
			IngredientID:     ingredientID,
			Quantity:         nullFloat64(ing.Quantity),
			Unit:             nullString(ing.Unit),
			IsOptional:       ing.IsOptional,
			PreparationNotes: nullString(ing.PreparationNotes),
		}); err != nil {
			return nil, fmt.Errorf("create recipe ingredient: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	logger.InfoContext(ctx, "recipe committed", "recipe_id", recipe.ID, "title", recipe.Title)
	return &recipe, nil
}
