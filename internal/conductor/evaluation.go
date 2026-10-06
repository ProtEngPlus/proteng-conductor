package conductor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const EvaluationStageID = 4

var ErrInvalidEvaluation = errors.New("invalid evaluation callback")

type Option func(*conductor)

// WithEvaluationRepositories leaves existing constructor callers compatible.
func WithEvaluationRepositories(runs repositories.EvaluationRunRepository, results repositories.EvaluationResultRepository) Option {
	return func(con *conductor) {
		con.evaluationRunRepository = runs
		con.evaluationResultRepository = results
	}
}

// receiveEvaluation handles registered, manually dispatched single-plugin runs.
// It never changes the parent job or dispatches the next pipeline stage.
func (con *conductor) receiveEvaluation(data Data) error {
	if con.evaluationRunRepository == nil || con.evaluationResultRepository == nil {
		return fmt.Errorf("evaluation persistence is not configured")
	}
	runID, err := primitive.ObjectIDFromHex(data.EvaluationRunID)
	if err != nil || runID.IsZero() {
		return fmt.Errorf("%w: evaluation_run_id must be a MongoDB ObjectID", ErrInvalidEvaluation)
	}
	jobID, err := primitive.ObjectIDFromHex(data.JobID)
	if err != nil || jobID.IsZero() {
		return fmt.Errorf("%w: job_id must be a MongoDB ObjectID", ErrInvalidEvaluation)
	}
	if data.Status != string(models.EvaluationCompleted) && data.Status != string(models.EvaluationFailed) {
		return fmt.Errorf("%w: unsupported status", ErrInvalidEvaluation)
	}
	run, err := con.evaluationRunRepository.FindById(runID.Hex())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: unknown evaluation run", ErrInvalidEvaluation)
	}
	if err != nil {
		return fmt.Errorf("find evaluation run: %w", err)
	}
	if err := run.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvaluation, err)
	}
	if run.Id != runID || run.JobId != jobID || run.Plugins[0] != data.Plugin {
		return fmt.Errorf("%w: run, job, or plugin mismatch", ErrInvalidEvaluation)
	}
	if run.State == models.EvaluationCompleted || run.State == models.EvaluationFailed {
		return nil // Redelivery, including a late conflicting terminal event.
	}
	if run.State != models.EvaluationOngoing {
		return fmt.Errorf("%w: run is not ongoing", ErrInvalidEvaluation)
	}
	job, err := con.jobRepository.FindById(jobID.Hex())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: parent job no longer exists", ErrInvalidEvaluation)
	}
	if err != nil {
		return fmt.Errorf("find evaluation job: %w", err)
	}
	if job == nil || job.Id != jobID {
		return fmt.Errorf("%w: parent job mismatch", ErrInvalidEvaluation)
	}
	values := make(map[primitive.ObjectID]map[string]interface{}, len(data.EvaluationResults))
	if data.Status == string(models.EvaluationCompleted) {
		if data.Error != "" || len(data.EvaluationResults) != len(run.MutationResultIds) {
			return fmt.Errorf("%w: completion requires all candidates and no error", ErrInvalidEvaluation)
		}
		for _, result := range data.EvaluationResults {
			id, err := primitive.ObjectIDFromHex(result.MutationResultID)
			if err != nil || id.IsZero() || len(result.Values) == 0 {
				return fmt.Errorf("%w: invalid candidate or empty values", ErrInvalidEvaluation)
			}
			if _, exists := values[id]; exists {
				return fmt.Errorf("%w: duplicate candidate", ErrInvalidEvaluation)
			}
			values[id] = result.Values
		}
	} else if strings.TrimSpace(data.Error) == "" || len(data.EvaluationResults) != 0 {
		return fmt.Errorf("%w: failure requires an error and no results", ErrInvalidEvaluation)
	}
	results := make([]*models.EvaluationResult, 0, len(run.MutationResultIds))
	for _, id := range run.MutationResultIds {
		if data.Status == string(models.EvaluationCompleted) {
			if _, ok := values[id]; !ok {
				return fmt.Errorf("%w: candidate set does not match run", ErrInvalidEvaluation)
			}
		}
		candidate, err := con.mutationResultRepository.FindById(id.Hex())
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w: candidate no longer exists", ErrInvalidEvaluation)
		}
		if err != nil {
			return fmt.Errorf("find evaluation candidate: %w", err)
		}
		if candidate == nil || candidate.Id != id || candidate.JobId != run.JobId || candidate.MutationId != run.MutationId || candidate.UserId != job.UserId {
			return fmt.Errorf("%w: candidate belongs to another job, mutation, or user", ErrInvalidEvaluation)
		}
		resultValues := values[id]
		if resultValues == nil {
			resultValues = map[string]interface{}{}
		}
		results = append(results, &models.EvaluationResult{
			EvaluationRunId:  run.Id,
			MutationResultId: id,
			Plugin:           data.Plugin,
			State:            models.EvaluationState(data.Status),
			Values:           resultValues,
			Error:            data.Error,
		})
	}
	if err := con.evaluationResultRepository.UpsertAll(results); err != nil {
		return fmt.Errorf("save evaluation results: %w", err)
	}
	if err := con.evaluationRunRepository.Finish(run.Id, models.EvaluationState(data.Status), data.Error); err != nil {
		return fmt.Errorf("finish evaluation run: %w", err)
	}
	return nil
}
