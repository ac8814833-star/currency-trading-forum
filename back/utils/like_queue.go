package utils

import (
	"back/config"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var ErrInvalidLikeMessage = errors.New("invalid like message")

func likeQueueName() string {
	if name := strings.TrimSpace(os.Getenv("LIKE_QUEUE_NAME")); name != "" {
		return name
	}
	if config.AppConfig != nil && strings.TrimSpace(config.AppConfig.RabbitMQ.QueueName) != "" {
		return config.AppConfig.RabbitMQ.QueueName
	}
	return "likes_sync_queue_v2"
}

// LikeMessage is deliberately versioned. RabbitMQ offers at-least-once
// delivery, so a consumer must be able to ignore duplicate or older actions.
type LikeMessage struct {
	EventID   string `json:"event_id"`
	UserID    uint   `json:"user_id"`
	ArticleID string `json:"article_id"`
	Type      int    `json:"type"`
	Version   int64  `json:"version"`
}

func openLikePublisher() (*amqp.Channel, <-chan amqp.Confirmation, error) {
	conn, err := config.RabbitMQConnection()
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}
	// Publisher confirms only acknowledge that RabbitMQ accepted a publish. If
	// the default-exchange queue has not been declared yet, a non-mandatory
	// publish can still be dropped while receiving an ACK. Declare it on the
	// relay channel as well as consumer/startup paths to make the relay safe
	// during startup and reconnect races.
	if _, err := ch.QueueDeclare(likeQueueName(), true, false, false, false, nil); err != nil {
		_ = ch.Close()
		return nil, nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, nil, err
	}
	return ch, ch.NotifyPublish(make(chan amqp.Confirmation, 1)), nil
}

func publishLikeMessageConfirmed(ctx context.Context, ch *amqp.Channel, confirms <-chan amqp.Confirmation, msg LikeMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := ch.PublishWithContext(ctx, "", likeQueueName(), false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}); err != nil {
		return err
	}
	select {
	case confirmation, ok := <-confirms:
		if !ok {
			return errors.New("rabbitmq publisher confirm channel closed")
		}
		if !confirmation.Ack {
			return errors.New("rabbitmq rejected like message")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishLikeMessage remains useful for one-off maintenance jobs. The request
// path uses the Redis outbox relay instead, so it never loses a state change
// in the gap between Redis mutation and broker publication.
func PublishLikeMessage(msg LikeMessage) error {
	ch, confirms, err := openLikePublisher()
	if err != nil {
		return err
	}
	defer ch.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return publishLikeMessageConfirmed(ctx, ch, confirms, msg)
}
