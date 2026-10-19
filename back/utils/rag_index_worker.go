package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func StartRAGIndexWorker(ctx context.Context) {
	go func() {
		logrus.Info("RAG index queue worker started")
		for {
			select {
			case <-ctx.Done():
				logrus.Info("RAG index queue worker stopped gracefully")
				return
			default:
			}

			if global.Db == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}

			if err := consumeRAGIndexMessages(ctx); err != nil {
				logrus.Errorf("RAG index queue consumer stopped: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			}
		}
	}()
}

func consumeRAGIndexMessages(ctx context.Context) error {
	conn, err := config.RabbitMQConnection()
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(
		config.AppConfig.RabbitMQ.RAGIndexQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		config.AppConfig.RabbitMQ.RAGIndexQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return errors.New("rag rabbitmq consumer channel closed")
			}

			var msg RAGIndexMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				logrus.Errorf("Error unmarshaling rag index message: %v", err)
				_ = d.Ack(false)
				continue
			}

			if err := processRAGIndexMessage(msg); err != nil {
				logrus.Errorf("Failed to process rag index message: %v", err)
				// Ack 后由周期性同步任务重试，避免故障期间反复 requeue 形成热循环。
				_ = d.Ack(false)
				continue
			}

			if err := d.Ack(false); err != nil {
				logrus.Errorf("Failed to ack rag index message: %v", err)
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(1 * time.Second):
				}
			}
		}
	}
}

func processRAGIndexMessage(msg RAGIndexMessage) error {
	var article models.Article
	err := global.Db.Unscoped().First(&article, msg.ArticleID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	if article.DeletedAt.Valid {
		return DeleteArticleChunks(article.ID)
	}

	if msg.RagVersion > 0 && article.RagVersion != msg.RagVersion {
		return nil
	}

	currentHash := BuildArticleIndexHash(article)
	if article.RagStatus == RAGStatusIndexed &&
		article.ContentHash != "" &&
		article.ContentHash == currentHash &&
		article.RagIndexSchemaVersion == CurrentRAGIndexSchemaVersion() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return IndexArticleByID(ctx, article.ID)
}
