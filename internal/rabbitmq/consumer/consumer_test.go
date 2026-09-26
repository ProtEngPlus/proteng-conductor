package consumer

import (
	"os"
	"testing"

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
