package publisher

import (
	"context"
	"sync"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

//go:generate mockgen -source=publisher.go -destination=mock_publisher/mock_publisher.go -package=mock_publisher

type publisher struct {
	mu   sync.Mutex
	conn *amqp.Connection
}

type Publisher interface {
	PublishDefaultExchange(ctx context.Context, queueName string, body []byte) error
	PublishWithTopic(ctx context.Context, routingKey string, body []byte) error
}

func NewPublisher() Publisher {
	return &publisher{}
}

func (p *publisher) newConnection() error {
	amqpURL := config.Config.RabbitMqUrl

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return err
	}
	p.conn = conn
	return nil
}

func (p *publisher) ensureConnection() (*amqp.Connection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil && !p.conn.IsClosed() {
		return p.conn, nil
	}
	if p.conn != nil {
		logger.Errorf("Publisher: connection closed, reconnecting")
	}
	if err := p.newConnection(); err != nil {
		return nil, err
	}
	return p.conn, nil
}

func (p *publisher) PublishDefaultExchange(ctx context.Context, queueName string, body []byte) error {
	conn, err := p.ensureConnection()
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return err
	}

	err = ch.PublishWithContext(
		ctx,
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		})
	if err != nil {
		return err
	}

	return nil
}

func (p *publisher) PublishWithTopic(ctx context.Context, routingKey string, body []byte) error {
	conn, err := p.ensureConnection()
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(
		"logs_topic", // name
		"topic",      // type
		false,        // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return err
	}

	err = ch.PublishWithContext(ctx,
		"logs_topic", // exchange
		routingKey,   // routing key **change here to tool**
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		})
	if err != nil {
		return err
	}

	return nil
}
