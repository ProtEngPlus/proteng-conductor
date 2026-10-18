package conductor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
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

// afterMutation decides the job state once a mutation run has stored its results.
// The job stays at stage 3 while an evaluation runs; the EvaluationRun tracks it.
func (con *conductor) afterMutation(job *models.Job, mutation *models.Mutation, results []*models.MutationResult) enum.JobState {
	if len(job.EvaluationPlugins) == 0 {
		return enum.JobStateCompleted // inherited behaviour: mutation is the last stage
	}
	// One-step jobs wait for the user like every other stage. Extra mutation
	// runs (RunId > 1) are started by the user explicitly, so they evaluate at once.
	if job.RunType != "auto" && mutation.RunId <= 1 {
		return enum.JobStatePending
	}
	if err := con.startEvaluation(job, mutation, results); err != nil {
		logger.Errorf("Conductor: afterMutation: JobID %s: %v", job.Id.Hex(), err)
		con.jobRepository.AddErrorLog(job.Id.Hex(), "Evaluation could not start: "+err.Error())
		return enum.JobStateCompleted // mutation results stay usable; a plugin never fails the job
	}
	return enum.JobStateOnGoing
}

// resumeEvaluation starts the evaluation a one-step job waits for after mutation.
func (con *conductor) resumeEvaluation(job *models.Job, mutation *models.Mutation) error {
	results, err := con.mutationResultRepository.GetAll(map[string]interface{}{"mutation_id": mutation.Id.Hex()})
	if err != nil {
		return err
	}
	job.State = enum.JobStateOnGoing
	if err := con.startEvaluation(job, mutation, results); err != nil {
		logger.Errorf("Conductor: resumeEvaluation: JobID %s: %v", job.Id.Hex(), err)
		con.jobRepository.AddErrorLog(job.Id.Hex(), "Evaluation could not start: "+err.Error())
		job.State = enum.JobStateCompleted
	}
	return con.jobRepository.Update(job.Id.Hex(), job)
}

// startEvaluation registers an evaluation run for the selected mutants and sends
// it to the run's current plugin. A run that cannot be dispatched is finished as
// failed, so it never stays ongoing without a worker.
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
	body, err := json.Marshal(evaluationRequest(job, run, candidates))
	if err == nil {
		err = con.publisher.PublishWithTopic(context.Background(), "evaluation."+run.Plugins[run.CurrentPluginIndex], body)
	}
	if err != nil {
		if finishErr := con.evaluationRunRepository.Finish(run.Id, models.EvaluationFailed, "dispatch failed: "+err.Error()); finishErr != nil {
			logger.Errorf("Conductor: startEvaluation: Failed to finish evaluation run %s: %v", run.Id.Hex(), finishErr)
		}
		return fmt.Errorf("dispatch evaluation run %s: %w", run.Id.Hex(), err)
	}
	return nil
}
