package consumer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type evaluationOrchestrator struct {
	events *[]string
	err    error
}

func (f *evaluationOrchestrator) Orchestrate(string) { panic("legacy entry point should not be used") }
func (f *evaluationOrchestrator) OrchestrateWithError(message string) error {
	*f.events = append(*f.events, "evaluation:"+message)
	return f.err
}

func TestConsumerEvaluationAcknowledgement(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "storage failed"}[failed], func(t *testing.T) {
			var events []string
			receiver := &evaluationOrchestrator{events: &events}
			if failed {
				receiver.err = errors.New("database unavailable")
			}
			c := &Consumer{conductor: receiver}
			c.handle(newDelivery(&events, "callback"))
			ack := "ack"
			if failed {
				ack = "nack-requeue"
			}
			require.Equal(t, []string{"evaluation:callback", ack}, events)
		})
	}
}
