package conductor

import (
	"context"
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
)

const (
	// evaluationTimeout is how long a run may stay ongoing without a result.
	// It leaves room for slow plugins such as DDGemb on many mutants.
	evaluationTimeout = 30 * time.Minute
	// evaluationSweepInterval is how often ongoing runs are checked.
	evaluationSweepInterval = time.Minute
)

// RunEvaluationSweeper fails runs that got no result in time until ctx ends.
// A request to a missing worker is dropped by RabbitMQ without an error, so
// without this check such a run, and its job, would stay ongoing forever.
func (con *conductor) RunEvaluationSweeper(ctx context.Context) {
	if con.evaluationRunRepository == nil {
		return
	}
	ticker := time.NewTicker(evaluationSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := con.sweepStaleEvaluations(now.Add(-evaluationTimeout)); err != nil {
				logger.Errorf("Conductor: evaluation sweeper: %v", err)
			}
		}
	}
}

// sweepStaleEvaluations fails every ongoing run created before the cutoff and
// completes its job. A run that a worker finished meanwhile is skipped, because
// Finish only changes ongoing runs; a later callback for a failed run is ignored.
func (con *conductor) sweepStaleEvaluations(createdBefore time.Time) error {
	runs, err := con.evaluationRunRepository.FindStale(createdBefore)
	if err != nil {
		return fmt.Errorf("find stale evaluation runs: %w", err)
	}
	for _, run := range runs {
		plugin := ""
		if run.CurrentPluginIndex < len(run.Plugins) {
			plugin = run.Plugins[run.CurrentPluginIndex]
		}
		message := fmt.Sprintf("timed out: no result from evaluation.%s within %s", plugin, evaluationTimeout)
		if err := con.evaluationRunRepository.Finish(run.Id, models.EvaluationFailed, message); err != nil {
			logger.Warnf("Conductor: evaluation sweeper: run %s not failed: %v", run.Id.Hex(), err)
			continue
		}
		if err := con.completeEvaluatedJob(run); err != nil {
			logger.Errorf("Conductor: evaluation sweeper: run %s: %v", run.Id.Hex(), err)
		}
	}
	return nil
}
