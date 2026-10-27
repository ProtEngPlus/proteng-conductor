package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// evaluationCandidates chooses which mutants an evaluation run scores.
// For now every result of the mutation run is evaluated; BLF4 (Top-X%
// selection) replaces this body later without touching the rest of the flow.
func evaluationCandidates(results []*models.MutationResult) []*models.MutationResult {
	return results
}

// evaluationRequest is the message an evaluation.<plugin> worker receives.
func evaluationRequest(job *models.Job, run *models.EvaluationRun, candidates []*models.MutationResult) map[string]interface{} {
	mutants := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		mutants = append(mutants, map[string]interface{}{
			"mutation_result_id": candidate.Id.Hex(),
			"sequence":           candidate.ProteinSequence,
			"assay_score":        candidate.AssayScore,
		})
	}
	return map[string]interface{}{
		"job_id":            job.Id.Hex(),
		"evaluation_run_id": run.Id.Hex(),
		"plugin":            run.Plugins[run.CurrentPluginIndex],
		"wild_type":         map[string]string{"sequence": job.InputProtein},
		"mutants":           mutants,
		"config":            map[string]interface{}{},
	}
}

// afterMutation decides the job state once a mutation run has stored its
// results, and whether an evaluation should start. The job stays at stage 3
// while an evaluation runs; the EvaluationRun tracks it. The caller saves the
// job first and only then calls beginEvaluation.
func afterMutation(job *models.Job, mutation *models.Mutation) (enum.JobState, bool) {
	if len(job.EvaluationPlugins) == 0 {
		return enum.JobStateCompleted, false // inherited behaviour: mutation is the last stage
	}
	// One-step jobs wait for the user like every other stage. Extra mutation
	// runs (RunId > 1) are started by the user explicitly, so they evaluate at once.
	if job.RunType != "auto" && mutation.RunId <= 1 {
		return enum.JobStatePending, false
	}
	return enum.JobStateOnGoing, true
}

// resumeEvaluation starts the evaluation a one-step job waits for after mutation.
func (con *conductor) resumeEvaluation(job *models.Job, mutation *models.Mutation) error {
	results, err := con.mutationResultRepository.GetAll(map[string]interface{}{"mutation_id": mutation.Id.Hex()})
	if err != nil {
		return err
	}
	job.State = enum.JobStateOnGoing
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		return err
	}
	return con.beginEvaluation(job, mutation, results)
}

// beginEvaluation starts the evaluation of a job that is already saved as
// ongoing. The save must come first: a plugin can reply, and the consumer can
// complete the job, before this returns, so saving the caller's copy of the
// job afterwards would set it back to ongoing for good. The job is saved again
// only when no plugin received the run, because then no reply can race with it.
func (con *conductor) beginEvaluation(job *models.Job, mutation *models.Mutation, results []*models.MutationResult) error {
	err := con.startEvaluation(job, mutation, results)
	if err == nil {
		return nil
	}
	logger.Errorf("Conductor: JobID %s: evaluation could not start: %v", job.Id.Hex(), err)
	con.jobRepository.AddErrorLog(job.Id.Hex(), "Evaluation could not start: "+err.Error())
	job.State = enum.JobStateCompleted // mutation results stay usable; a plugin never fails the job
	if job.CompleteAt.IsZero() {
		job.CompleteAt = time.Now()
	}
	return con.jobRepository.Update(job.Id.Hex(), job)
}

// startEvaluation registers an evaluation run for the selected mutants and sends
// it to the run's first plugin. It returns an error when no plugin could be
// dispatched; the run is then finished as failed, so it never stays ongoing.
func (con *conductor) startEvaluation(job *models.Job, mutation *models.Mutation, results []*models.MutationResult) error {
	if con.evaluationRunRepository == nil {
		return fmt.Errorf("evaluation persistence is not configured")
	}
	candidates := evaluationCandidates(results)
	ids := make([]primitive.ObjectID, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.Id)
	}
	run := &models.EvaluationRun{
		Id:                primitive.NewObjectID(),
		JobId:             job.Id,
		MutationId:        mutation.Id,
		MutationResultIds: ids,
		Plugins:           job.EvaluationPlugins,
	}
	if err := con.evaluationRunRepository.Create(run); err != nil {
		return fmt.Errorf("create evaluation run: %w", err)
	}
	dispatched, err := con.dispatchCurrentPlugin(job, run, candidates)
	if err != nil {
		return err
	}
	if !dispatched {
		return fmt.Errorf("no plugin of evaluation run %s could be dispatched", run.Id.Hex())
	}
	return nil
}

// dispatchCurrentPlugin sends the run to its current plugin. A plugin that cannot
// be reached counts as failed, and the next one is tried; after the last one the
// run is finished. It reports whether some plugin received the run.
func (con *conductor) dispatchCurrentPlugin(job *models.Job, run *models.EvaluationRun, candidates []*models.MutationResult) (bool, error) {
	for {
		plugin := run.CurrentPlugin()
		body, err := json.Marshal(evaluationRequest(job, run, candidates))
		if err == nil {
			err = con.publisher.PublishWithTopic(context.Background(), "evaluation."+plugin, body)
		}
		if err == nil {
			return true, nil
		}
		pluginError := fmt.Sprintf("%s: dispatch failed: %v", plugin, err)
		logger.Errorf("Conductor: evaluation run %s: %s", run.Id.Hex(), pluginError)
		if run.IsLastPlugin() {
			if finishErr := con.finishRun(run, pluginError); finishErr != nil && !errors.Is(finishErr, repositories.ErrEvaluationRunChanged) {
				logger.Errorf("Conductor: Failed to finish evaluation run %s: %v", run.Id.Hex(), finishErr)
			}
			return false, nil
		}
		if err := con.advanceRun(run, pluginError); err != nil {
			if errors.Is(err, repositories.ErrEvaluationRunChanged) {
				return true, nil // another delivery or the sweeper already moved it on
			}
			return false, fmt.Errorf("advance evaluation run: %w", err)
		}
	}
}
