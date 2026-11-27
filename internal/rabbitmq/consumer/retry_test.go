package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// retryTestOrchestrator fails the first `fail` calls, then succeeds.
type retryTestOrchestrator struct {
	events *[]string
	fail   int
	calls  int
}

func (f *retryTestOrchestrator) Orchestrate(string) {}

func (f *retryTestOrchestrator) OrchestrateWithError(message string) error {
	f.calls++
	*f.events = append(*f.events, "orchestrate:"+message)
	if f.calls <= f.fail {
		return errors.New("database unavailable")
	}
	return nil
}

func newRetryTestConsumer(orch orchestrator, delays *[]time.Duration) *Consumer {
	return &Consumer{conductor: orch, sleep: func(d time.Duration) { *delays = append(*delays, d) }}
}

func TestRetryDelaySchedule(t *testing.T) {
	want := map[int]time.Duration{
		0: 0, 1: 0, 2: time.Second, 3: 2 * time.Second, 4: 4 * time.Second,
		5: 8 * time.Second, 6: 16 * time.Second, 7: 30 * time.Second, 9: 30 * time.Second, 100: 30 * time.Second,
	}
	for failures, delay := range want {
		assert.Equal(t, delay, retryDelay(failures), "failures=%d", failures)
	}
}

func TestConsumerRetryBackoff(t *testing.T) {
	t.Run("first failure requeues at once", func(tt *testing.T) {
		var events []string
		var delays []time.Duration
		c := newRetryTestConsumer(&retryTestOrchestrator{events: &events, fail: 1}, &delays)

		c.handle(newDelivery(&events, "msg"))

		assert.Equal(tt, []string{"orchestrate:msg", "nack-requeue"}, events)
		assert.Empty(tt, delays)
	})

	t.Run("repeated failures back off and are dropped at the limit", func(tt *testing.T) {
		var events []string
		var delays []time.Duration
		c := newRetryTestConsumer(&retryTestOrchestrator{events: &events, fail: 100}, &delays)

		for i := 0; i < maxDeliveryAttempts; i++ {
			c.handle(newDelivery(&events, "msg"))
		}

		var acks []string
		for _, e := range events {
			if e != "orchestrate:msg" {
				acks = append(acks, e)
			}
		}
		assert.Len(tt, acks, maxDeliveryAttempts)
		assert.Equal(tt, "nack-drop", acks[len(acks)-1])
		for _, a := range acks[:len(acks)-1] {
			assert.Equal(tt, "nack-requeue", a)
		}
		assert.Equal(tt, []time.Duration{
			time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
			16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
		}, delays)
	})

	t.Run("success resets the count", func(tt *testing.T) {
		var events []string
		var delays []time.Duration
		orch := &retryTestOrchestrator{events: &events, fail: 2}
		c := newRetryTestConsumer(orch, &delays)

		c.handle(newDelivery(&events, "msg")) // fail 1, no pause
		c.handle(newDelivery(&events, "msg")) // fail 2, 1s pause
		c.handle(newDelivery(&events, "msg")) // success
		orch.calls, orch.fail = 0, 1
		c.handle(newDelivery(&events, "msg")) // a new first failure

		assert.Equal(tt, []time.Duration{time.Second}, delays)
		assert.Equal(tt, "nack-requeue", events[len(events)-1])
	})

	t.Run("different messages are counted separately", func(tt *testing.T) {
		var events []string
		var delays []time.Duration
		c := newRetryTestConsumer(&retryTestOrchestrator{events: &events, fail: 100}, &delays)

		c.handle(newDelivery(&events, "a"))
		c.handle(newDelivery(&events, "b"))

		assert.Empty(tt, delays)
	})

	t.Run("shutdown during the pause requeues at once", func(tt *testing.T) {
		var events []string
		c := &Consumer{conductor: &retryTestOrchestrator{events: &events, fail: 100}}
		c.failures = map[string]int{messageKey([]byte("msg")): 5} // the next pause would be 16s
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		c.handleContext(ctx, newDelivery(&events, "msg"))

		assert.Less(tt, time.Since(start), time.Second)
		assert.Equal(tt, []string{"orchestrate:msg", "nack-requeue"}, events)
	})
}
