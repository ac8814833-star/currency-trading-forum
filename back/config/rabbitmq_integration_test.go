package config

import (
	"back/global"
	"fmt"
	"os"
	"testing"
	"time"
)

// This test uses a temporary queue and proves that a closed broker connection
// is replaced on the next accessor call. It is opt-in because RabbitMQ is an
// external integration dependency.
func TestRabbitMQConnectionRecoversAfterClose(t *testing.T) {
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("set TEST_RABBITMQ_URL to run RabbitMQ recovery integration test")
	}

	previousConfig := AppConfig
	previousConn, previousChannel := global.RabbitMQConn, global.RabbitMQChannel
	queueSuffix := time.Now().UnixNano()
	AppConfig = &config{}
	AppConfig.RabbitMQ.URL = url
	AppConfig.RabbitMQ.QueueName = fmt.Sprintf("like_reconnect_test_%d", queueSuffix)
	AppConfig.RabbitMQ.RAGIndexQueueName = fmt.Sprintf("rag_reconnect_test_%d", queueSuffix)
	global.RabbitMQConn, global.RabbitMQChannel = nil, nil
	t.Cleanup(func() {
		if conn := global.RabbitMQConn; conn != nil && !conn.IsClosed() {
			if ch, err := conn.Channel(); err == nil {
				_, _ = ch.QueueDelete(AppConfig.RabbitMQ.QueueName, false, false, false)
				_, _ = ch.QueueDelete(AppConfig.RabbitMQ.RAGIndexQueueName, false, false, false)
				_ = ch.Close()
			}
		}
		_ = CloseRabbitMQ()
		AppConfig = previousConfig
		global.RabbitMQConn, global.RabbitMQChannel = previousConn, previousChannel
	})

	first, err := RabbitMQConnection()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := RabbitMQConnection()
	if err != nil {
		t.Fatal(err)
	}
	if second == first || second.IsClosed() {
		t.Fatal("closed RabbitMQ connection was not replaced")
	}
	channel, err := RabbitMQChannel()
	if err != nil || channel.IsClosed() {
		t.Fatalf("replacement connection did not restore shared channel: %v", err)
	}
}
