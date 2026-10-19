package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// StartLikeWorker persists Redis/MQ actions without serializing every request
// on an article row. Multiple consumers are safe because StateVersion makes an
// older or duplicated message a no-op for its (user, article) pair.
func StartLikeWorker(ctx context.Context) {
	workers := 4
	if config.AppConfig != nil && config.AppConfig.RabbitMQ.LikeWorkerCount > 0 {
		workers = config.AppConfig.RabbitMQ.LikeWorkerCount
	}
	for i := 0; i < workers; i++ {
		workerID := i + 1
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if global.Db == nil {
					if !waitLikeWorker(ctx, 3*time.Second) {
						return
					}
					continue
				}
				if err := consumeLikeMessages(ctx, workerID); err != nil {
					logrus.Errorf("like queue consumer %d stopped: %v", workerID, err)
					if !waitLikeWorker(ctx, 2*time.Second) {
						return
					}
				}
			}
		}()
	}
}

func waitLikeWorker(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

// ApplyLikeMessage is kept exported for integration tests and recovery jobs.
func ApplyLikeMessage(msg LikeMessage) error {
	if err := processLikeMessage(msg); err != nil {
		return err
	}
	return markLikeEventPersisted(msg)
}

func consumeLikeMessages(ctx context.Context, workerID int) error {
	conn, err := config.RabbitMQConnection()
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if _, err := ch.QueueDeclare(likeQueueName(), true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.Qos(64, 0, false); err != nil {
		return err
	}
	msgs, err := ch.Consume(likeQueueName(), fmt.Sprintf("like-persist-%d", workerID), false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return errors.New("like rabbitmq consumer channel closed")
			}
			var msg LikeMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				logrus.Errorf("discard invalid like message: %v", err)
				_ = d.Ack(false)
				continue
			}
			if err := ApplyLikeMessage(msg); err != nil {
				if errors.Is(err, ErrInvalidLikeMessage) {
					logrus.Errorf("discard invalid like message: %v", err)
					_ = d.Ack(false)
					continue
				}
				logrus.Errorf("persist like message failed: %v", err)
				// Keep the message for a transient MySQL failure. The upsert below is
				// idempotent, so a redelivery cannot duplicate the relation.
				_ = d.Nack(false, true)
				if !waitLikeWorker(ctx, 200*time.Millisecond) {
					return nil
				}
				continue
			}
			if err := d.Ack(false); err != nil {
				return err
			}
		}
	}
}

func processLikeMessage(msg LikeMessage) error {
	articleID, err := strconv.ParseUint(msg.ArticleID, 10, 64)
	if err != nil || articleID == 0 || msg.EventID == "" || msg.UserID == 0 || msg.Version <= 0 || (msg.Type != 1 && msg.Type != -1) {
		return fmt.Errorf("%w: %+v", ErrInvalidLikeMessage, msg)
	}
	if global.Db == nil {
		return errors.New("database is not initialized")
	}

	now := time.Now()
	var deletedAt interface{}
	if msg.Type == -1 {
		deletedAt = now
	}
	return global.Db.Transaction(func(tx *gorm.DB) error {
		var article models.Article
		if err := tx.Select("id").First(&article, uint(articleID)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// The article was deleted after the event was accepted; no relation
				// should be recreated, and ACKing this message is correct.
				return nil
			}
			return err
		}
		// MySQL performs a row-level conflict check on the user/article unique
		// index, not a lock on the hot article row. A newer version wins, which
		// makes duplicate delivery and cross-worker reordering harmless.
		return tx.Exec(`INSERT INTO likes (user_id, article_id, state_version, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				deleted_at = CASE WHEN state_version < VALUES(state_version) THEN VALUES(deleted_at) ELSE deleted_at END,
				state_version = CASE WHEN state_version < VALUES(state_version) THEN VALUES(state_version) ELSE state_version END,
				updated_at = CASE WHEN state_version < VALUES(state_version) THEN VALUES(updated_at) ELSE updated_at END`,
			msg.UserID, uint(articleID), msg.Version, now, now, deletedAt).Error
	})
}
