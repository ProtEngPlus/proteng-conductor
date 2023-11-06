package rabbitmq

import (
	"errors"
	"log"
	"os"
	"os/signal"
	"proteng-conductor/internal/conductor"
	"syscall"
	"time"

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
			log.Printf("Failed to connect to RabbitMQ: %v", err)
			if attempts >= 5 {
				return errors.New("failed to connect to RabbitMQ after 5 attempts")
			}
			time.Sleep(10 * time.Second)
			continue
		}
		attempts = 0

		ch, err := conn.Channel()
		if err != nil {
			log.Printf("Failed to open a channel: %v", err)
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
			log.Printf("Failed to declare a queue: %v", err)
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
			log.Printf("Failed to register a consumer: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		log.Printf(" [*] Waiting for messages from %s", q.Name)

		go func() {
			for d := range msgs {
				log.Printf("Received a message: %v from %v", string(d.Body), q.Name)
				c.conductor.Orchestrate(string(d.Body))
			}
		}()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

		<-sig
		log.Println("Shutting down consumer...")
		return nil
	}
}
