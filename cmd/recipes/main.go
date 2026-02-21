package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/mwhite7112/woodpantry-recipes/internal/api"
	"github.com/mwhite7112/woodpantry-recipes/internal/db"
	"github.com/mwhite7112/woodpantry-recipes/internal/logging"
	"github.com/mwhite7112/woodpantry-recipes/internal/service"
)

func main() {
	logging.Setup()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		slog.Error("DB_URL is required")
		os.Exit(1)
	}

	dictionaryURL := os.Getenv("DICTIONARY_URL")
	if dictionaryURL == "" {
		slog.Error("DICTIONARY_URL is required")
		os.Exit(1)
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		slog.Error("OPENAI_API_KEY is required")
		os.Exit(1)
	}

	extractModel := os.Getenv("EXTRACT_MODEL")
	if extractModel == "" {
		extractModel = "gpt-5-mini"
	}

	sqlDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(sqlDB); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	queries := db.New(sqlDB)
	svc := service.New(queries, sqlDB, dictionaryURL, openaiKey, extractModel)
	handler := api.NewRouter(svc)

	addr := fmt.Sprintf(":%s", port)
	slog.Info("recipes service listening", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func runMigrations(sqlDB *sql.DB) error {
	srcDriver, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}
	dbDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", srcDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
