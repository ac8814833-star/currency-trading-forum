package utils

import (
	"back/config"
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RAGIndexMessage struct {
	ArticleID  uint `json:"article_id"`
	RagVersion int  `json:"rag_version"`
}

func AsyncIndexArticleByID(articleID uint, ragVersion int) error {
	if config.AppConfig == nil || !config.AppConfig.RAG.AutoIndex {
		return nil
	}
	return PublishRAGIndexMessage(RAGIndexMessage{
		ArticleID:  articleID,
		RagVersion: ragVersion,
	})
}

func PublishRAGIndexMessage(msg RAGIndexMessage) error {
	channel, err := config.RabbitMQChannel()
	if err != nil {
		return err
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return channel.PublishWithContext(
		ctx,
		"",
		config.AppConfig.RabbitMQ.RAGIndexQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
}
