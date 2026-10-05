package publisher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeConfirmation struct {
	acked bool
	block bool
}

func (f fakeConfirmation) WaitContext(ctx context.Context) (bool, error) {
	if f.block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return f.acked, nil
}

func TestWaitForConfirm(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when broker acks", func(tt *testing.T) {
		err := waitForConfirm(context.Background(), fakeConfirmation{acked: true}, time.Second)
		assert.NoError(tt, err)
	})

	t.Run("returns error when broker nacks", func(tt *testing.T) {
		err := waitForConfirm(context.Background(), fakeConfirmation{acked: false}, time.Second)
		assert.Error(tt, err)
	})

	t.Run("returns deadline error when broker does not answer", func(tt *testing.T) {
		start := time.Now()
		err := waitForConfirm(context.Background(), fakeConfirmation{block: true}, 50*time.Millisecond)
		assert.True(tt, errors.Is(err, context.DeadlineExceeded))
		assert.Less(tt, time.Since(start), time.Second)
	})
}
