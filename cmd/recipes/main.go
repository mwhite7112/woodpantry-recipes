package main

import (
	"database/sql"
	"errors"
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
	if err := run(); err != nil {
		slog.Error("recipes service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logging.Setup()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return errors.New("DB_URL is required")
	}

	dictionaryURL := os.Getenv("DICTIONARY_URL")
	if dictionaryURL == "" {
		return errors.New("DICTIONARY_URL is required")
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		return errors.New("OPENAI_API_KEY is required")
	}

	extractModel := os.Getenv("EXTRACT_MODEL")
	if extractModel == "" {
		extractModel = "gpt-5-mini"
	}

	sqlDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	if err := runMigrations(sqlDB); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	queries := db.New(sqlDB)
	extractor := service.NewOpenAIExtractor(openaiKey, extractModel)
	resolver := service.NewDictionaryResolver(dictionaryURL)
	svc := service.New(queries, sqlDB, extractor, resolver)
	handler := api.NewRouter(svc)

	addr := fmt.Sprintf(":%s", port)
	slog.Info("recipes service listening", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}

	return nil
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
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
