package utils

import (
	"back/global"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// StartLikeOutboxRelay bridges the Redis 3-compatible List outbox to
// RabbitMQ. An item stays in the processing list until RabbitMQ confirms its
// persistent publish, closing the Redis-mutate / MQ-publish crash window.
func StartLikeOutboxRelay(ctx context.Context) {
	go func() {
		logrus.Info("like outbox relay started")
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if global.RedisDB == nil {
				if !waitLikeRelay(ctx, 3*time.Second) {
					return
				}
				continue
			}
			if err := relayLikeOutbox(ctx); err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				logrus.Errorf("like outbox relay stopped: %v", err)
				if !waitLikeRelay(ctx, 2*time.Second) {
					return
				}
			}
		}
	}()
}

func waitLikeRelay(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

func relayLikeOutbox(ctx context.Context) error {
	if err := recoverLikeOutbox(); err != nil {
		return err
	}
	ch, confirms, err := openLikePublisher()
	if err != nil {
		return err
	}
	defer ch.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		payload, err := global.RedisDB.BRPopLPush(likeOutboxList, likeOutboxProcessingList, time.Second).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return err
		}
		if err := relayLikePayload(ctx, ch, confirms, payload); err != nil {
			return err
		}
	}
}

// Any item left in processing was either never published or published just
// before a relay crash. Requeueing is safe because MySQL applies the greatest
// version for each user/article pair.
func recoverLikeOutbox() error {
	for {
		_, err := global.RedisDB.RPopLPush(likeOutboxProcessingList, likeOutboxList).Result()
		if err == redis.Nil {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func relayLikePayload(ctx context.Context, ch *amqp.Channel, confirms <-chan amqp.Confirmation, payload string) error {
	msg, err := likeMessageFromOutbox(payload)
	if err != nil {
		if errors.Is(err, ErrInvalidLikeMessage) {
			_ = global.RedisDB.LRem(likeOutboxProcessingList, 1, payload).Err()
			return nil
		}
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = publishLikeMessageConfirmed(publishCtx, ch, confirms, msg)
	cancel()
	if err != nil {
		return err
	}
	if err := global.RedisDB.LRem(likeOutboxProcessingList, 1, payload).Err(); err != nil {
		return err
	}
	return nil
}

func likeMessageFromOutbox(payload string) (LikeMessage, error) {
	parts := strings.Split(payload, "|")
	if len(parts) != 4 {
		return LikeMessage{}, fmt.Errorf("%w: malformed outbox payload", ErrInvalidLikeMessage)
	}
	articleID, articleErr := strconv.ParseUint(parts[0], 10, 64)
	userID, userErr := strconv.ParseUint(parts[1], 10, 64)
	action, actionErr := strconv.Atoi(parts[2])
	version, versionErr := strconv.ParseInt(parts[3], 10, 64)
	if articleErr != nil || articleID == 0 || userErr != nil || userID == 0 || actionErr != nil || versionErr != nil || version <= 0 || (action != 1 && action != -1) {
		return LikeMessage{}, fmt.Errorf("%w: malformed outbox payload", ErrInvalidLikeMessage)
	}
	return LikeMessage{
		EventID:   "redis-" + parts[3],
		ArticleID: parts[0],
		UserID:    uint(userID),
		Type:      action,
		Version:   version,
	}, nil
}
