package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/mwhite7112/woodpantry-recipes/internal/db"
)

// StagedIngredient is an ingredient as extracted by LLM before resolve.
type StagedIngredient struct {
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

// openAIMessage is a chat message for the OpenAI API.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIRequest is a chat completion request body.
type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
}

// openAIResponse is the chat completion response.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const extractionPrompt = `Extract the recipe from the following text into a JSON object with this exact schema:
{
  "title": "string",
  "description": "string (optional)",
  "source_url": "string (optional)",
  "servings": number (optional),
  "prep_minutes": number (optional),
  "cook_minutes": number (optional),
  "tags": ["string"] (e.g. ["dinner","vegetarian"]),
  "steps": ["string (each step as one instruction)"],
  "ingredients": [
    {
      "name": "string (ingredient name only, no quantity)",
      "quantity": number (optional),
      "unit": "string (e.g. cup, tbsp, g — optional)",
      "is_optional": boolean (default false),
      "preparation_notes": "string (e.g. finely chopped — optional)"
    }
  ]
}

Respond ONLY with the JSON object. No markdown, no explanation.

Recipe text:
`

// ExtractRecipe calls the OpenAI API to parse free text into a StagedRecipe.
func (s *Service) ExtractRecipe(ctx context.Context, rawText string) (*StagedRecipe, error) {
	slog.Info("LLM extraction starting", "model", s.extractModel, "input_len", len(rawText))

	reqBody := openAIRequest{
		Model: s.extractModel,
		Messages: []openAIMessage{
			{Role: "user", Content: extractionPrompt + rawText},
		},
	}
	reqBody.ResponseFormat.Type = "json_object"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.openaiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var aiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(aiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	var staged StagedRecipe
	if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &staged); err != nil {
		return nil, fmt.Errorf("parse extracted recipe json: %w", err)
	}

	slog.Info("LLM extraction complete", "title", staged.Title, "ingredients", len(staged.Ingredients), "steps", len(staged.Steps))
	return &staged, nil
}

// resolveResponse is the response body from POST /ingredients/resolve.
type resolveResponse struct {
	Ingredient struct {
		ID string `json:"id"`
	} `json:"ingredient"`
}

// resolveIngredient calls the Dictionary service and returns the canonical ingredient_id.
func (s *Service) resolveIngredient(ctx context.Context, name string) (uuid.UUID, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.dictionaryURL+"/ingredients/resolve",
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
	if err := json.Unmarshal(job.StagedData, &staged); err != nil {
		return nil, fmt.Errorf("unmarshal staged data: %w", err)
	}

	slog.Info("committing staged recipe", "job_id", job.ID, "title", staged.Title, "ingredients", len(staged.Ingredients))

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.q.WithTx(tx)

	tags := staged.Tags
	if tags == nil {
		tags = []string{}
	}

	recipe, err := qtx.CreateRecipe(ctx, db.CreateRecipeParams{
		Title:       staged.Title,
		Description: nullString(staged.Description),
		SourceURL:   nullString(staged.SourceURL),
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
		ingredientID, err := s.resolveIngredient(ctx, ing.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve ingredient %q: %w", ing.Name, err)
		}
		slog.Info("resolved ingredient", "name", ing.Name, "ingredient_id", ingredientID)
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

	slog.Info("recipe committed", "recipe_id", recipe.ID, "title", recipe.Title)
	return &recipe, nil
}
