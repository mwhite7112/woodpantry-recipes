package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RecipeImportRequestedPublisher publishes recipe.import.requested events.
type RecipeImportRequestedPublisher struct {
	conn *amqp.Connection
}

func NewRecipeImportRequestedPublisher(rabbitmqURL string) (*RecipeImportRequestedPublisher, error) {
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
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
		_ = conn.Close()
		return nil, fmt.Errorf("declare exchange %q: %w", exchangeName, err)
	}

	return &RecipeImportRequestedPublisher{conn: conn}, nil
}

func (p *RecipeImportRequestedPublisher) PublishRecipeImportRequested(
	ctx context.Context,
	event RecipeImportRequestedEvent,
) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal recipe.import.requested event: %w", err)
	}

	if err := ch.PublishWithContext(ctx, exchangeName, recipeImportRequestedRoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish recipe.import.requested: %w", err)
	}

	return nil
}

func (p *RecipeImportRequestedPublisher) Close() error {
	return p.conn.Close()
}
