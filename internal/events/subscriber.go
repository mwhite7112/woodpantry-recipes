package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

const recipeImportedQueue = "recipes.recipe-imported"

// RecipeImportedEventHandler handles recipe.imported events.
type RecipeImportedEventHandler interface {
	HandleRecipeImportedEvent(ctx context.Context, event RecipeImportedEvent) error
}

// RecipeImportedSubscriber consumes recipe.imported events.
type RecipeImportedSubscriber struct {
	conn    *amqp.Connection
	handler RecipeImportedEventHandler
}

func NewRecipeImportedSubscriber(
	rabbitmqURL string,
	handler RecipeImportedEventHandler,
) (*RecipeImportedSubscriber, error) {
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	return &RecipeImportedSubscriber{
		conn:    conn,
		handler: handler,
	}, nil
}

func (s *RecipeImportedSubscriber) Run(ctx context.Context) error {
	ch, err := s.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %q: %w", exchangeName, err)
	}

	if _, err := ch.QueueDeclare(
		recipeImportedQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare queue %q: %w", recipeImportedQueue, err)
	}

	if err := ch.QueueBind(
		recipeImportedQueue,
		recipeImportedRoutingKey,
		exchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind queue %q: %w", recipeImportedQueue, err)
	}

	msgs, err := ch.Consume(
		recipeImportedQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("consume %q: %w", recipeImportedQueue, err)
	}

	slog.Info("recipe.imported subscriber started", "queue", recipeImportedQueue)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("recipe.imported delivery channel closed")
			}

			var event RecipeImportedEvent
			if err := json.Unmarshal(msg.Body, &event); err != nil {
				slog.Error("invalid recipe.imported payload", "error", err)
				_ = msg.Ack(false)
				continue
			}

			if err := s.handler.HandleRecipeImportedEvent(ctx, event); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.Warn("dropping recipe.imported event for unknown job", "job_id", event.JobID)
					_ = msg.Ack(false)
					continue
				}

				slog.Error("failed to handle recipe.imported event", "job_id", event.JobID, "error", err)
				_ = msg.Nack(false, true)
				continue
			}

			if err := msg.Ack(false); err != nil {
				slog.Error("failed to ack recipe.imported event", "job_id", event.JobID, "error", err)
			}
		}
	}
}

func (s *RecipeImportedSubscriber) Close() error {
	return s.conn.Close()
}
