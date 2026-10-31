package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

const consumerTag = "conductor"

type orchestrator interface {
	Orchestrate(message string)
}

type Consumer struct {
	conductor orchestrator

	// failures counts how often each message body failed in a row, so a message
	// that keeps failing is retried with growing pauses and finally dropped.
	mu       sync.Mutex
	failures map[string]int
	// sleep replaces the retry pause in tests; nil waits for real.
	sleep func(time.Duration)
}

const (
	// maxDeliveryAttempts is how often a failing message is tried before it is
	// dropped (about two minutes in total). A dropped Evaluation result is not
	// lost silently: the evaluation sweeper fails its run and completes the job.
	maxDeliveryAttempts = 10
	maxRetryDelay       = 30 * time.Second
	// maxTrackedFailures bounds the in-memory counters.
	maxTrackedFailures = 1000
)

// retryDelay is the pause before requeuing a message that failed `failures`
// times in a row. The first failure requeues at once, so a short blip costs
// nothing; later ones wait 1s, 2s, 4s, ... up to maxRetryDelay.
func retryDelay(failures int) time.Duration {
	if failures <= 1 {
		return 0
	}
	if failures-2 >= 5 {
		return maxRetryDelay
	}
	if d := time.Second << (failures - 2); d < maxRetryDelay {
		return d
	}
	return maxRetryDelay
}

func NewConsumer(conductor conductor.Conductor) *Consumer {
	return &Consumer{conductor: conductor}
}

func (c *Consumer) RunConsumer(ctx context.Context, amqpURL string, queueName string) error {
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

		if err := ch.Qos(1, 0, false); err != nil {
			logger.Errorf("Failed to set QoS: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		msgs, err := ch.Consume(
			q.Name,      // queue
			consumerTag, // consumer
			false,       // auto-ack
			false,       // exclusive
			false,       // no-local
			false,       // no-wait
			nil,         // arguments
		)
		if err != nil {
			logger.Errorf("Failed to register a consumer: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		logger.Infof(" [*] Waiting for messages from %s", q.Name)

		c.consume(ctx, msgs)

		if ctx.Err() == nil {
			logger.Errorf("RabbitMQ delivery channel closed, reconnecting...")
			conn.Close()
			time.Sleep(10 * time.Second)
			continue
		}

		logger.Zap.Info("Shutting down consumer...")
		if err := ch.Cancel(consumerTag, false); err != nil {
			logger.Errorf("Failed to cancel consumer: %v", err)
		}
		ch.Close()
		conn.Close()
		return nil
	}
}

func (c *Consumer) handle(d amqp.Delivery) {
	c.handleContext(context.Background(), d)
}

func (c *Consumer) handleContext(ctx context.Context, d amqp.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("JobConsumer: panic while processing message: %v", r)
			if err := d.Nack(false, false); err != nil {
				logger.Errorf("JobConsumer: failed to nack message: %v", err)
			}
		}
	}()

	if receiver, ok := c.conductor.(interface{ OrchestrateWithError(string) error }); ok {
		if err := receiver.OrchestrateWithError(string(d.Body)); err != nil {
			c.retryLater(ctx, d, err)
			return
		}
		c.forget(d.Body)
	} else {
		c.conductor.Orchestrate(string(d.Body))
	}

	if err := d.Ack(false); err != nil {
		logger.Errorf("JobConsumer: failed to ack message: %v", err)
	}
}

func (c *Consumer) consume(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case d, ok := <-msgs:
			if !ok {
				return
			}
			logger.Infof("JobConsumer: Received a message (%d bytes)", len(d.Body))
			c.handleContext(ctx, d)
		}
	}
}

// retryLater requeues a message whose processing failed, after a pause that
// grows with each failure, and drops it after maxDeliveryAttempts failures.
// A shutdown during the pause requeues at once, so nothing is lost.
func (c *Consumer) retryLater(ctx context.Context, d amqp.Delivery, cause error) {
	failures := c.countFailure(d.Body)
	if failures >= maxDeliveryAttempts {
		logger.Errorf("JobConsumer: giving up after %d failed attempts, dropping message: %v", failures, cause)
		c.forget(d.Body)
		if err := d.Nack(false, false); err != nil {
			logger.Errorf("JobConsumer: failed to nack message: %v", err)
		}
		return
	}
	delay := retryDelay(failures)
	logger.Errorf("JobConsumer: processing failed (attempt %d of %d), requeuing in %s: %v", failures, maxDeliveryAttempts, delay, cause)
	c.wait(ctx, delay)
	if err := d.Nack(false, true); err != nil {
		logger.Errorf("JobConsumer: failed to nack message: %v", err)
	}
}

func (c *Consumer) wait(ctx context.Context, delay time.Duration) {
	if delay <= 0 {
		return
	}
	if c.sleep != nil {
		c.sleep(delay)
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func messageKey(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (c *Consumer) countFailure(body []byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures == nil || len(c.failures) >= maxTrackedFailures {
		c.failures = make(map[string]int)
	}
	key := messageKey(body)
	c.failures[key]++
	return c.failures[key]
}

func (c *Consumer) forget(body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.failures, messageKey(body))
}
