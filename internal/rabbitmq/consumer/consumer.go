package consumer

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conductor conductor.Conductor
}

func NewConsumer(conductor conductor.Conductor) *Consumer {
	return &Consumer{conductor: conductor}
}

func (c *Consumer) RunConsumer(amqpURL string, queueName string) error {
	attempts := 0
	for {
		attempts++
		conn, err := amqp.Dial(amqpURL)
		if err != nil {
			logger.Errorf("Failed to connect to RabbitMQ: %v", err)
			if attempts >= 5 {
				return errors.New("failed to connect to RabbitMQ after 5 attempts")
			}
			time.Sleep(10 * time.Second)
			continue
		}
		attempts = 0

		ch, err := conn.Channel()
		if err != nil {
			logger.Errorf("Failed to open a channel: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		q, err := ch.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			nil,       // arguments
		)
		if err != nil {
			logger.Errorf("Failed to declare a queue: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		msgs, err := ch.Consume(
			q.Name, // queue
			"",     // consumer
			true,   // auto-ack
			false,  // exclusive
			false,  // no-local
			false,  // no-wait
			nil,    // arguments
		)
		if err != nil {
			logger.Errorf("Failed to register a consumer: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		logger.Infof(" [*] Waiting for messages from %s", q.Name)

		go func() {
			for d := range msgs {
				logger.Infof("JobConsumer: Received a message: %v from %v", string(d.Body), q.Name)
				c.conductor.Orchestrate(string(d.Body))
			}
		}()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

		<-sig
		logger.Zap.Info("Shutting down consumer...")
		os.Exit(0)
		return nil
	}
}
