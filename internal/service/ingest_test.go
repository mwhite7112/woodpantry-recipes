package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDictionaryResolver_Success(t *testing.T) {
	t.Parallel()

	ingredientID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/ingredients/resolve", r.URL.Path)

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "flour", body["name"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resolveResponse{
			Ingredient: struct {
				ID string `json:"id"`
			}{ID: ingredientID.String()},
		})
	}))
	defer server.Close()

	resolver := NewDictionaryResolver(server.URL)
	id, err := resolver.ResolveIngredient(context.Background(), "flour")
	require.NoError(t, err)
	assert.Equal(t, ingredientID, id)
}

func TestDictionaryResolver_Created(t *testing.T) {
	t.Parallel()

	ingredientID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resolveResponse{
			Ingredient: struct {
				ID string `json:"id"`
			}{ID: ingredientID.String()},
		})
	}))
	defer server.Close()

	resolver := NewDictionaryResolver(server.URL)
	id, err := resolver.ResolveIngredient(context.Background(), "new ingredient")
	require.NoError(t, err)
	assert.Equal(t, ingredientID, id)
}

func TestDictionaryResolver_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := NewDictionaryResolver(server.URL)
	_, err := resolver.ResolveIngredient(context.Background(), "flour")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestDictionaryResolver_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	resolver := NewDictionaryResolver(server.URL)
	_, err := resolver.ResolveIngredient(context.Background(), "flour")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode resolve response")
}

func TestDictionaryResolver_InvalidUUID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resolveResponse{
			Ingredient: struct {
				ID string `json:"id"`
			}{ID: "not-a-uuid"},
		})
	}))
	defer server.Close()

	resolver := NewDictionaryResolver(server.URL)
	_, err := resolver.ResolveIngredient(context.Background(), "flour")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ingredient id")
}
