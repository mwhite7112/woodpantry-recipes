package main

import (
	"context"
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
	"github.com/mwhite7112/woodpantry-recipes/internal/events"
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

	rabbitMQURL := os.Getenv("RABBITMQ_URL")

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
	resolver := service.NewDictionaryResolver(dictionaryURL)

	importPublisher, err := setupImportRequestedPublisher(rabbitMQURL)
	if err != nil {
		return err
	}
	defer importPublisher.Close()

	svc := service.New(queries, sqlDB, nil, resolver, importPublisher)
	handler := api.NewRouter(svc)

	importedSubscriber, err := setupRecipeImportedSubscriber(rabbitMQURL, svc)
	if err != nil {
		return err
	}
	defer importedSubscriber.Close()

	if rabbitMQURL != "" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			if err := importedSubscriber.Run(ctx); err != nil {
				slog.Error("recipe.imported subscriber stopped", "error", err)
			}
		}()
	}

	addr := fmt.Sprintf(":%s", port)
	slog.Info("recipes service listening", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}

	return nil
}

type importRequestedPublisher interface {
	service.ImportRequestPublisher
	Close() error
}

func setupImportRequestedPublisher(rabbitMQURL string) (importRequestedPublisher, error) {
	if rabbitMQURL == "" {
		slog.Info("RABBITMQ_URL not set; recipe.import.requested publishing disabled")
		return nopImportRequestedPublisher{}, nil
	}

	pub, err := events.NewRecipeImportRequestedPublisher(rabbitMQURL)
	if err != nil {
		slog.Warn("failed to initialize RabbitMQ publisher; recipe.import.requested publishing disabled", "error", err)
		return nopImportRequestedPublisher{}, nil
	}

	slog.Info("RabbitMQ recipe.import.requested publisher enabled")
	return pub, nil
}

type nopImportRequestedPublisher struct{}

func (nopImportRequestedPublisher) PublishRecipeImportRequested(
	_ context.Context,
	_ events.RecipeImportRequestedEvent,
) error {
	return nil
}

func (nopImportRequestedPublisher) Close() error {
	return nil
}

type recipeImportedSubscriber interface {
	Run(ctx context.Context) error
	Close() error
}

func setupRecipeImportedSubscriber(
	rabbitMQURL string,
	svc events.RecipeImportedEventHandler,
) (recipeImportedSubscriber, error) {
	if rabbitMQURL == "" {
		slog.Info("RABBITMQ_URL not set; recipe.imported subscriber disabled")
		return nopRecipeImportedSubscriber{}, nil
	}

	sub, err := events.NewRecipeImportedSubscriber(rabbitMQURL, svc)
	if err != nil {
		slog.Warn("failed to initialize recipe.imported subscriber; subscriber disabled", "error", err)
		return nopRecipeImportedSubscriber{}, nil
	}

	slog.Info("RabbitMQ recipe.imported subscriber enabled")
	return sub, nil
}

type nopRecipeImportedSubscriber struct{}

func (nopRecipeImportedSubscriber) Run(_ context.Context) error {
	return nil
}

func (nopRecipeImportedSubscriber) Close() error {
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
