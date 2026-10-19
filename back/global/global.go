package global

import (
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/go-redis/redis"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

var (
	Db              *gorm.DB
	RedisDB         *redis.Client
	RabbitMQConn    *amqp.Connection
	RabbitMQChannel *amqp.Channel
	ESClient        *elasticsearch.Client
)
