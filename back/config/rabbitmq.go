package config

import (
	"back/global"
	"fmt"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// RabbitMQ connections can be interrupted independently of the HTTP server.
// Keep replacement of the global handles serialized so publishers and workers
// can request a healthy connection after broker restart without requiring an
// application restart.
var rabbitMQMu sync.Mutex

func InitRabbitMQ() {
	if err := EnsureRabbitMQ(); err != nil {
		logrus.Warnf("RabbitMQ is unavailable at startup: %v (workers will retry)", err)
	}
}

// EnsureRabbitMQ makes sure the shared startup channel and its durable queues
// exist. It is safe to call repeatedly after RabbitMQ restarts.
func EnsureRabbitMQ() error {
	_, err := RabbitMQChannel()
	return err
}

// RabbitMQConnection returns a healthy connection. Callers that use their own
// short-lived channels (such as the like relay/consumers) should use this
// accessor instead of retaining global.RabbitMQConn directly.
func RabbitMQConnection() (*amqp.Connection, error) {
	rabbitMQMu.Lock()
	defer rabbitMQMu.Unlock()
	return ensureRabbitMQConnectionLocked()
}

// RabbitMQChannel returns the shared channel used by the RAG publisher. If a
// connection or channel was closed by a broker restart, it is recreated and
// both durable queues are declared again.
func RabbitMQChannel() (*amqp.Channel, error) {
	rabbitMQMu.Lock()
	defer rabbitMQMu.Unlock()

	conn, err := ensureRabbitMQConnectionLocked()
	if err != nil {
		return nil, err
	}
	if global.RabbitMQChannel != nil && !global.RabbitMQChannel.IsClosed() {
		return global.RabbitMQChannel, nil
	}
	if global.RabbitMQChannel != nil {
		_ = global.RabbitMQChannel.Close()
		global.RabbitMQChannel = nil
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := declareRabbitMQQueuesLocked(ch); err != nil {
		_ = ch.Close()
		return nil, err
	}
	global.RabbitMQChannel = ch
	return ch, nil
}

func ensureRabbitMQConnectionLocked() (*amqp.Connection, error) {
	if global.RabbitMQConn != nil && !global.RabbitMQConn.IsClosed() {
		return global.RabbitMQConn, nil
	}
	if global.RabbitMQChannel != nil {
		_ = global.RabbitMQChannel.Close()
		global.RabbitMQChannel = nil
	}
	if global.RabbitMQConn != nil {
		_ = global.RabbitMQConn.Close()
		global.RabbitMQConn = nil
	}
	if AppConfig == nil || strings.TrimSpace(AppConfig.RabbitMQ.URL) == "" {
		return nil, fmt.Errorf("rabbitmq url is empty")
	}
	conn, err := amqp.Dial(AppConfig.RabbitMQ.URL)
	if err != nil {
		return nil, err
	}
	global.RabbitMQConn = conn
	return conn, nil
}

func declareRabbitMQQueuesLocked(ch *amqp.Channel) error {
	if AppConfig == nil || strings.TrimSpace(AppConfig.RabbitMQ.QueueName) == "" {
		return fmt.Errorf("rabbitmq queue name is empty")
	}
	if AppConfig.RabbitMQ.RAGIndexQueueName == "" {
		AppConfig.RabbitMQ.RAGIndexQueueName = "rag_index_queue"
	}
	if _, err := ch.QueueDeclare(AppConfig.RabbitMQ.QueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare like queue: %w", err)
	}
	if _, err := ch.QueueDeclare(AppConfig.RabbitMQ.RAGIndexQueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare rag index queue: %w", err)
	}
	return nil
}

func CloseRabbitMQ() error {
	rabbitMQMu.Lock()
	defer rabbitMQMu.Unlock()

	ch, conn := global.RabbitMQChannel, global.RabbitMQConn
	global.RabbitMQChannel, global.RabbitMQConn = nil, nil
	if ch != nil {
		_ = ch.Close()
	}
	if conn != nil {
		return conn.Close()
	}
	return nil
}
