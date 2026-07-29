package publisher

import (
	"context"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

//go:generate mockgen -source=publisher.go -destination=mock_publisher/mock_publisher.go -package=mock_publisher

type publisher struct {
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

func (p *publisher) ensureConnection() error {
	if p.conn == nil {
		return p.newConnection()
	}

	ch, err := p.conn.Channel()
	if err != nil {
		logger.Errorf("Publisher: Failed to open a channel: %v, renewing connection", err)
		return p.newConnection()
	}

	ch.Close()
	return nil
}

func (p *publisher) PublishDefaultExchange(ctx context.Context, queueName string, body []byte) error {
	err := p.ensureConnection()
	if err != nil {
		return err
	}

	ch, err := p.conn.Channel()
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
			ContentType: "text/plain",
			Body:        body,
		})
	if err != nil {
		return err
	}

	return nil
}

func (p *publisher) PublishWithTopic(ctx context.Context, routingKey string, body []byte)  error{
	err := p.ensureConnection()
	if err != nil {
		return err
	}

	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(
		"logs_topic", // name
		"topic",      // type
		false,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return err
	}
	
	err = ch.PublishWithContext(ctx,
		"logs_topic",          // exchange
		routingKey, // routing key **change here to tool**
		false, // mandatory
		false, // immediate
		amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
		})
	if err != nil {
		return err
	}

	return nil
}
