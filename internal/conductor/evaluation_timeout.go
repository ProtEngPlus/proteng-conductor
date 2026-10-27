package conductor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"go.mongodb.org/mongo-driver/mongo"
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

// sweepStaleEvaluations treats the current plugin of every run that waited past
// the cutoff as failed and moves the run on: the next plugin is dispatched, or
// the run is finished and its job completed. A late callback from a timed-out
// plugin is ignored, because the run has moved past it.
func (con *conductor) sweepStaleEvaluations(pluginStartedBefore time.Time) error {
	runs, err := con.evaluationRunRepository.FindStale(pluginStartedBefore)
	if err != nil {
		return fmt.Errorf("find stale evaluation runs: %w", err)
	}
	for _, run := range runs {
		pluginError := fmt.Sprintf("%s: timed out: no result from evaluation.%s within %s",
			run.CurrentPlugin(), run.CurrentPlugin(), evaluationTimeout)
		job, candidates, err := con.evaluationContext(run)
		if errors.Is(err, mongo.ErrNoDocuments) {
			// The job or a candidate was deleted, so no plugin can score this
			// run and every later sweep would fail the same way: stop it here.
			con.stopOrphanedRun(run, fmt.Sprintf("%s; run stopped: %v", pluginError, err))
			continue
		}
		if err != nil {
			// Probably temporary (e.g. the database is unreachable): keep the
			// run ongoing and try again on the next sweep.
			logger.Errorf("Conductor: evaluation sweeper: run %s: %v", run.Id.Hex(), err)
			continue
		}
		if err := con.moveToNextPlugin(job, run, candidates, pluginError); err != nil {
			logger.Errorf("Conductor: evaluation sweeper: run %s: %v", run.Id.Hex(), err)
		}
	}
	return nil
}

// stopOrphanedRun finishes a run whose job or candidates no longer exist as
// failed, without trying its remaining plugins, and completes the job if it
// still exists and waits at stage 3.
func (con *conductor) stopOrphanedRun(run *models.EvaluationRun, reason string) {
	if err := con.finishRun(run, reason); err != nil && !errors.Is(err, repositories.ErrEvaluationRunChanged) {
		logger.Errorf("Conductor: evaluation sweeper: run %s: finish: %v", run.Id.Hex(), err)
		return
	}
	if err := con.completeEvaluatedJob(run); err != nil {
		logger.Errorf("Conductor: evaluation sweeper: run %s: %v", run.Id.Hex(), err)
	}
}

// evaluationContext loads what dispatching the run's next plugin needs.
func (con *conductor) evaluationContext(run *models.EvaluationRun) (*models.Job, []*models.MutationResult, error) {
	job, err := con.jobRepository.FindById(run.JobId.Hex())
	if err != nil {
		return nil, nil, fmt.Errorf("find job: %w", err)
	}
	candidates := make([]*models.MutationResult, 0, len(run.MutationResultIds))
	for _, id := range run.MutationResultIds {
		candidate, err := con.mutationResultRepository.FindById(id.Hex())
		if err != nil {
			return nil, nil, fmt.Errorf("find candidate %s: %w", id.Hex(), err)
		}
		candidates = append(candidates, candidate)
	}
	return job, candidates, nil
}
