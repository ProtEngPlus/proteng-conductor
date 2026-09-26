package consumer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/protengplus/proteng-conductor/internal/logger"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	logger.Zap = zap.NewNop()
	os.Exit(m.Run())
}

// fakeAcknowledger records ack calls into a shared event log.
type fakeAcknowledger struct {
	events *[]string
}

func (f *fakeAcknowledger) Ack(tag uint64, multiple bool) error {
	*f.events = append(*f.events, "ack")
	return nil
}

func (f *fakeAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	if requeue {
		*f.events = append(*f.events, "nack-requeue")
	} else {
		*f.events = append(*f.events, "nack-drop")
	}
	return nil
}

func (f *fakeAcknowledger) Reject(tag uint64, requeue bool) error {
	*f.events = append(*f.events, "reject")
	return nil
}

// fakeOrchestrator records the call and can be told to panic.
type fakeOrchestrator struct {
	events  *[]string
	doPanic bool
}

func (f *fakeOrchestrator) Orchestrate(message string) {
	*f.events = append(*f.events, "orchestrate:"+message)
	if f.doPanic {
		panic("boom")
	}
}

func newDelivery(events *[]string, body string) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: &fakeAcknowledger{events: events},
		DeliveryTag:  1,
		Body:         []byte(body),
	}
}

func TestConsumer_handle(t *testing.T) {
	t.Parallel()

	t.Run("acks once after orchestrate returns", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &fakeOrchestrator{events: &events}}

		c.handle(newDelivery(&events, "msg"))

		assert.Equal(tt, []string{"orchestrate:msg", "ack"}, events)
	})

	t.Run("recovers panic and drops message without ack", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &fakeOrchestrator{events: &events, doPanic: true}}

		assert.NotPanics(tt, func() {
			c.handle(newDelivery(&events, "bad"))
		})
		assert.Equal(tt, []string{"orchestrate:bad", "nack-drop"}, events)
	})
}

type blockingOrchestrator struct {
	events  *[]string
	started chan struct{}
	release chan struct{}
}

func (f *blockingOrchestrator) Orchestrate(message string) {
	*f.events = append(*f.events, "orchestrate:"+message)
	close(f.started)
	<-f.release
}

// runConsume runs consume in a goroutine and returns a channel closed on return
func runConsume(c *Consumer, ctx context.Context, msgs <-chan amqp.Delivery) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.consume(ctx, msgs)
	}()
	return done
}

func TestConsumer_consume(t *testing.T) {
	t.Parallel()

	const timeout = 2 * time.Second

	t.Run("returns when context is cancelled", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &fakeOrchestrator{events: &events}}
		ctx, cancel := context.WithCancel(context.Background())
		msgs := make(chan amqp.Delivery)

		done := runConsume(c, ctx, msgs)
		cancel()

		select {
		case <-done:
		case <-time.After(timeout):
			tt.Fatal("consume did not return after cancel")
		}
		assert.Empty(tt, events)
	})

	t.Run("does not take a new message after cancel", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &fakeOrchestrator{events: &events}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msgs := make(chan amqp.Delivery, 1)
		msgs <- newDelivery(&events, "late")

		select {
		case <-runConsume(c, ctx, msgs):
		case <-time.After(timeout):
			tt.Fatal("consume did not return")
		}
		assert.Empty(tt, events)
	})

	t.Run("finishes the in-flight message before returning", func(tt *testing.T) {
		var events []string
		orch := &blockingOrchestrator{
			events:  &events,
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		c := &Consumer{conductor: orch}
		ctx, cancel := context.WithCancel(context.Background())
		msgs := make(chan amqp.Delivery, 1)
		msgs <- newDelivery(&events, "inflight")

		done := runConsume(c, ctx, msgs)
		select {
		case <-orch.started:
		case <-time.After(timeout):
			tt.Fatal("message was not picked up")
		}
		cancel()

		select {
		case <-done:
			tt.Fatal("consume returned before the in-flight message finished")
		case <-time.After(50 * time.Millisecond):
		}

		close(orch.release)
		select {
		case <-done:
		case <-time.After(timeout):
			tt.Fatal("consume did not return after the message finished")
		}
		assert.Equal(tt, []string{"orchestrate:inflight", "ack"}, events)
	})

	t.Run("returns when the delivery channel is closed", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &fakeOrchestrator{events: &events}}
		msgs := make(chan amqp.Delivery)
		close(msgs)

		select {
		case <-runConsume(c, context.Background(), msgs):
		case <-time.After(timeout):
			tt.Fatal("consume did not return after msgs closed")
		}
	})
}
