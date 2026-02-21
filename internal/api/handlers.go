package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/logging"
	"github.com/mwhite7112/woodpantry-recipes/internal/service"
)

// NewRouter wires up all routes.
func NewRouter(svc *service.Service) http.Handler {
	r := chi.NewRouter()
	r.Use(logging.Middleware)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handleHealth)

	r.Get("/recipes", handleListRecipes(svc))
	r.Post("/recipes", handleCreateRecipe(svc))
	r.Get("/recipes/{id}", handleGetRecipe(svc))
	r.Put("/recipes/{id}", handleUpdateRecipe(svc))
	r.Delete("/recipes/{id}", handleDeleteRecipe(svc))

	r.Post("/recipes/ingest", handleIngest(svc))
	r.Get("/recipes/ingest/{job_id}", handleGetIngestJob(svc))
	r.Post("/recipes/ingest/{job_id}/confirm", handleConfirmIngest(svc))

	return r
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok")) //nolint:errcheck
}

// --- list ---

func handleListRecipes(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		tag := q.Get("tag")
		titleSearch := q.Get("title")
		cookTimeStr := q.Get("cook_time_max")

		var recipes []db.Recipe
		var err error

		switch {
		case tag != "":
			recipes, err = svc.Queries().ListRecipesByTag(r.Context(), tag)
		case cookTimeStr != "":
			maxCook, parseErr := strconv.ParseInt(cookTimeStr, 10, 32)
			if parseErr != nil {
				jsonError(w, "invalid cook_time_max", http.StatusBadRequest)
				return
			}
			recipes, err = svc.Queries().ListRecipesByCookTime(r.Context(), int32(maxCook))
		case titleSearch != "":
			recipes, err = svc.Queries().ListRecipesByTitle(r.Context(), titleSearch)
		default:
			recipes, err = svc.Queries().ListRecipes(r.Context())
		}

		if err != nil {
			jsonError(w, "failed to list recipes", http.StatusInternalServerError, err)
			return
		}
		if recipes == nil {
			recipes = []db.Recipe{}
		}
		jsonOK(w, recipes)
	}
}

// --- create ---

type recipeInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	SourceURL   string   `json:"source_url"`
	Servings    int      `json:"servings"`
	PrepMinutes int      `json:"prep_minutes"`
	CookMinutes int      `json:"cook_minutes"`
	Tags        []string `json:"tags"`
	Steps       []struct {
		StepNumber  int    `json:"step_number"`
		Instruction string `json:"instruction"`
	} `json:"steps"`
	Ingredients []struct {
		IngredientID     string  `json:"ingredient_id"`
		Quantity         float64 `json:"quantity"`
		Unit             string  `json:"unit"`
		IsOptional       bool    `json:"is_optional"`
		PreparationNotes string  `json:"preparation_notes"`
	} `json:"ingredients"`
}

func handleCreateRecipe(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recipeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			jsonError(w, "title is required", http.StatusBadRequest)
			return
		}
		tags := req.Tags
		if tags == nil {
			tags = []string{}
		}

		tx, err := svc.DB().BeginTx(r.Context(), nil)
		if err != nil {
			jsonError(w, "failed to start transaction", http.StatusInternalServerError, err)
			return
		}
		defer tx.Rollback() //nolint:errcheck

		qtx := svc.Queries().WithTx(tx)

		recipe, err := qtx.CreateRecipe(r.Context(), db.CreateRecipeParams{
			Title:       req.Title,
			Description: nullString(req.Description),
			SourceURL:   nullString(req.SourceURL),
			Servings:    nullInt32(req.Servings),
			PrepMinutes: nullInt32(req.PrepMinutes),
			CookMinutes: nullInt32(req.CookMinutes),
			Tags:        tags,
		})
		if err != nil {
			jsonError(w, "failed to create recipe", http.StatusInternalServerError, err)
			return
		}

		for _, step := range req.Steps {
			if _, err := qtx.CreateStep(r.Context(), db.CreateStepParams{
				RecipeID:    recipe.ID,
				StepNumber:  int32(step.StepNumber),
				Instruction: step.Instruction,
			}); err != nil {
				jsonError(w, "failed to create step", http.StatusInternalServerError, err)
				return
			}
		}

		for _, ing := range req.Ingredients {
			ingID, err := uuid.Parse(ing.IngredientID)
			if err != nil {
				jsonError(w, "invalid ingredient_id: "+ing.IngredientID, http.StatusBadRequest)
				return
			}
			if _, err := qtx.CreateRecipeIngredient(r.Context(), db.CreateRecipeIngredientParams{
				RecipeID:         recipe.ID,
				IngredientID:     ingID,
				Quantity:         nullFloat64(ing.Quantity),
				Unit:             nullString(ing.Unit),
				IsOptional:       ing.IsOptional,
				PreparationNotes: nullString(ing.PreparationNotes),
			}); err != nil {
				jsonError(w, "failed to create recipe ingredient", http.StatusInternalServerError, err)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			jsonError(w, "failed to commit", http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(recipe) //nolint:errcheck
	}
}

// --- get ---

type recipeDetail struct {
	db.Recipe
	Steps       []db.RecipeStep       `json:"steps"`
	Ingredients []db.RecipeIngredient `json:"ingredients"`
}

func handleGetRecipe(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		recipe, err := svc.Queries().GetRecipe(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "recipe not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get recipe", http.StatusInternalServerError, err)
			return
		}
		steps, err := svc.Queries().ListStepsByRecipe(r.Context(), id)
		if err != nil {
			jsonError(w, "failed to get steps", http.StatusInternalServerError, err)
			return
		}
		ingredients, err := svc.Queries().ListIngredientsByRecipe(r.Context(), id)
		if err != nil {
			jsonError(w, "failed to get ingredients", http.StatusInternalServerError, err)
			return
		}
		if steps == nil {
			steps = []db.RecipeStep{}
		}
		if ingredients == nil {
			ingredients = []db.RecipeIngredient{}
		}
		jsonOK(w, recipeDetail{Recipe: recipe, Steps: steps, Ingredients: ingredients})
	}
}

// --- update ---

func handleUpdateRecipe(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}

		// Verify recipe exists first.
		if _, err := svc.Queries().GetRecipe(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "recipe not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get recipe", http.StatusInternalServerError, err)
			return
		}

		var req recipeInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			jsonError(w, "title is required", http.StatusBadRequest)
			return
		}
		tags := req.Tags
		if tags == nil {
			tags = []string{}
		}

		tx, err := svc.DB().BeginTx(r.Context(), nil)
		if err != nil {
			jsonError(w, "failed to start transaction", http.StatusInternalServerError, err)
			return
		}
		defer tx.Rollback() //nolint:errcheck

		qtx := svc.Queries().WithTx(tx)

		recipe, err := qtx.UpdateRecipe(r.Context(), db.UpdateRecipeParams{
			ID:          id,
			Title:       req.Title,
			Description: nullString(req.Description),
			SourceURL:   nullString(req.SourceURL),
			Servings:    nullInt32(req.Servings),
			PrepMinutes: nullInt32(req.PrepMinutes),
			CookMinutes: nullInt32(req.CookMinutes),
			Tags:        tags,
		})
		if err != nil {
			jsonError(w, "failed to update recipe", http.StatusInternalServerError, err)
			return
		}

		if err := qtx.DeleteStepsByRecipe(r.Context(), id); err != nil {
			jsonError(w, "failed to clear steps", http.StatusInternalServerError, err)
			return
		}
		for _, step := range req.Steps {
			if _, err := qtx.CreateStep(r.Context(), db.CreateStepParams{
				RecipeID:    id,
				StepNumber:  int32(step.StepNumber),
				Instruction: step.Instruction,
			}); err != nil {
				jsonError(w, "failed to update step", http.StatusInternalServerError, err)
				return
			}
		}

		if err := qtx.DeleteIngredientsByRecipe(r.Context(), id); err != nil {
			jsonError(w, "failed to clear ingredients", http.StatusInternalServerError, err)
			return
		}
		for _, ing := range req.Ingredients {
			ingID, err := uuid.Parse(ing.IngredientID)
			if err != nil {
				jsonError(w, "invalid ingredient_id: "+ing.IngredientID, http.StatusBadRequest)
				return
			}
			if _, err := qtx.CreateRecipeIngredient(r.Context(), db.CreateRecipeIngredientParams{
				RecipeID:         id,
				IngredientID:     ingID,
				Quantity:         nullFloat64(ing.Quantity),
				Unit:             nullString(ing.Unit),
				IsOptional:       ing.IsOptional,
				PreparationNotes: nullString(ing.PreparationNotes),
			}); err != nil {
				jsonError(w, "failed to update ingredient", http.StatusInternalServerError, err)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			jsonError(w, "failed to commit", http.StatusInternalServerError, err)
			return
		}

		jsonOK(w, recipe)
	}
}

// --- delete ---

func handleDeleteRecipe(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := svc.Queries().DeleteRecipe(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "recipe not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to delete recipe", http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- ingest ---

type ingestRequest struct {
	Text string `json:"text"`
}

func handleIngest(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ingestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Text == "" {
			jsonError(w, "text is required", http.StatusBadRequest)
			return
		}

		job, err := svc.Queries().CreateIngestionJob(r.Context(), db.CreateIngestionJobParams{
			Type:     "text_blob",
			RawInput: req.Text,
		})
		if err != nil {
			jsonError(w, "failed to create ingestion job", http.StatusInternalServerError, err)
			return
		}

		// Mark processing and run LLM extraction synchronously (Phase 1).
		if _, err := svc.Queries().UpdateIngestionJobStatus(r.Context(), db.UpdateIngestionJobStatusParams{
			ID:     job.ID,
			Status: "processing",
		}); err != nil {
			jsonError(w, "failed to update job status", http.StatusInternalServerError, err)
			return
		}

		staged, err := svc.ExtractRecipe(r.Context(), req.Text)
		if err != nil {
			svc.Queries().UpdateIngestionJobStatus(r.Context(), db.UpdateIngestionJobStatusParams{ //nolint:errcheck
				ID:     job.ID,
				Status: "failed",
			})
			jsonError(w, "extraction failed: "+err.Error(), http.StatusInternalServerError, err)
			return
		}

		stagedJSON, err := json.Marshal(staged)
		if err != nil {
			jsonError(w, "failed to serialize staged recipe", http.StatusInternalServerError, err)
			return
		}

		updatedJob, err := svc.Queries().UpdateIngestionJobStaged(r.Context(), db.UpdateIngestionJobStagedParams{
			ID:         job.ID,
			StagedData: stagedJSON,
		})
		if err != nil {
			jsonError(w, "failed to stage recipe", http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(updatedJob) //nolint:errcheck
	}
}

func handleGetIngestJob(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "job_id"))
		if err != nil {
			jsonError(w, "invalid job_id", http.StatusBadRequest)
			return
		}
		job, err := svc.Queries().GetIngestionJob(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "job not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get job", http.StatusInternalServerError, err)
			return
		}
		jsonOK(w, job)
	}
}

func handleConfirmIngest(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "job_id"))
		if err != nil {
			jsonError(w, "invalid job_id", http.StatusBadRequest)
			return
		}
		job, err := svc.Queries().GetIngestionJob(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "job not found", http.StatusNotFound)
				return
			}
			jsonError(w, "failed to get job", http.StatusInternalServerError, err)
			return
		}
		if job.Status != "staged" {
			jsonError(w, "job is not in staged status", http.StatusConflict)
			return
		}

		recipe, err := svc.CommitStagedRecipe(r.Context(), job)
		if err != nil {
			jsonError(w, "failed to commit recipe: "+err.Error(), http.StatusInternalServerError, err)
			return
		}

		if _, err := svc.Queries().UpdateIngestionJobStatus(r.Context(), db.UpdateIngestionJobStatusParams{
			ID:     job.ID,
			Status: "confirmed",
		}); err != nil {
			slog.Error("failed to mark job confirmed (recipe already committed)", "job_id", job.ID, "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(recipe) //nolint:errcheck
	}
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func jsonError(w http.ResponseWriter, msg string, status int, errs ...error) {
	if status >= 500 && len(errs) > 0 {
		slog.Error(msg, "status", status, "error", errs[0])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullInt32(n int) sql.NullInt32 {
	return sql.NullInt32{Int32: int32(n), Valid: n != 0}
}

func nullFloat64(f float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: f, Valid: f != 0}
}
