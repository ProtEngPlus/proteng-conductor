package conductor

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type memoryEvaluationRuns struct {
	run       *models.EvaluationRun
	findErr   error
	finishErr error
	finishes  int
}

func (r *memoryEvaluationRuns) Create(run *models.EvaluationRun) error { r.run = run; return nil }
func (r *memoryEvaluationRuns) FindById(string) (*models.EvaluationRun, error) {
	return r.run, r.findErr
}
func (r *memoryEvaluationRuns) Finish(_ primitive.ObjectID, state models.EvaluationState, message string) error {
	r.finishes++
	if r.finishErr != nil {
		return r.finishErr
	}
	r.run.State, r.run.Error = state, message
	return nil
}

type memoryEvaluationResults struct {
	items  map[primitive.ObjectID]*models.EvaluationResult
	err    error
	writes int
}

func (r *memoryEvaluationResults) UpsertAll(results []*models.EvaluationResult) error {
	r.writes++
	for _, result := range results {
		r.items[result.MutationResultId] = result
		if r.err != nil {
			return r.err
		} // Simulate a partially successful bulk write.
	}
	return nil
}

type evaluationFixture struct {
	con        *conductor
	runs       *memoryEvaluationRuns
	results    *memoryEvaluationResults
	job        *models.Job
	candidates map[string]*models.MutationResult
	data       Data
}

func newEvaluationFixture(t *testing.T) *evaluationFixture {
	con, deps, finish := newTestConductor(t)
	t.Cleanup(finish)
	jobID, mutationID := primitive.NewObjectID(), primitive.NewObjectID()
	ids := []primitive.ObjectID{primitive.NewObjectID(), primitive.NewObjectID()}
	run := &models.EvaluationRun{
		Id: primitive.NewObjectID(), JobId: jobID, MutationId: mutationID,
		MutationResultIds: ids, Plugins: []string{"mock"}, State: models.EvaluationOngoing,
	}
	f := &evaluationFixture{
		con: con, runs: &memoryEvaluationRuns{run: run},
		results:    &memoryEvaluationResults{items: make(map[primitive.ObjectID]*models.EvaluationResult)},
		job:        &models.Job{Id: jobID, UserId: "owner", StageId: 3, State: enum.JobStateCompleted},
		candidates: make(map[string]*models.MutationResult),
		data:       Data{JobID: jobID.Hex(), StageID: EvaluationStageID, EvaluationRunID: run.Id.Hex(), Plugin: "mock", Status: "COMPLETED"},
	}
	for i, id := range ids {
		f.candidates[id.Hex()] = &models.MutationResult{Id: id, JobId: jobID, MutationId: mutationID, UserId: "owner"}
		f.data.EvaluationResults = append(f.data.EvaluationResults, EvaluationResultData{
			MutationResultID: id.Hex(), Values: map[string]interface{}{"mock_score": 0.50 + float64(i)*0.01},
		})
	}
	WithEvaluationRepositories(f.runs, f.results)(con)
	deps.jobRepository.EXPECT().FindById(gomock.Any()).DoAndReturn(func(string) (*models.Job, error) { return f.job, nil }).AnyTimes()
	deps.mutationResultRepository.EXPECT().FindById(gomock.Any()).DoAndReturn(func(id string) (*models.MutationResult, error) {
		result, ok := f.candidates[id]
		if !ok {
			return nil, mongo.ErrNoDocuments
		}
		return result, nil
	}).AnyTimes()
	// No Update or publish expectations: Evaluation must leave the parent pipeline alone.
	return f
}

func (f *evaluationFixture) message(t *testing.T) string {
	b, err := json.Marshal(Payload{ServiceName: "mock-evaluation", Data: f.data})
	require.NoError(t, err)
	return string(b)
}

func TestEvaluationCompletionAndRedelivery(t *testing.T) {
	f := newEvaluationFixture(t)
	require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
	require.Equal(t, models.EvaluationCompleted, f.runs.run.State)
	require.Len(t, f.results.items, 2)
	for i, id := range f.runs.run.MutationResultIds {
		result := f.results.items[id]
		require.Equal(t, f.runs.run.Id, result.EvaluationRunId)
		require.Equal(t, "mock", result.Plugin)
		require.Equal(t, 0.50+float64(i)*0.01, result.Values["mock_score"])
	}
	require.Equal(t, enum.JobStateCompleted, f.job.State)
	require.Equal(t, 3, f.job.StageId)
	require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
	f.data.Status, f.data.Error, f.data.EvaluationResults = "FAILED", "late failure", nil
	require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
	require.Equal(t, 1, f.results.writes)
	require.Equal(t, 1, f.runs.finishes)
	require.Equal(t, models.EvaluationCompleted, f.runs.run.State)
}

func TestEvaluationFailure(t *testing.T) {
	f := newEvaluationFixture(t)
	f.data.Status, f.data.Error, f.data.EvaluationResults = "FAILED", "mock failed", nil
	f.con.Orchestrate(f.message(t)) // The existing entry point also reaches Evaluation.
	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
	for _, result := range f.results.items {
		require.Equal(t, models.EvaluationFailed, result.State)
		require.Equal(t, "mock failed", result.Error)
		require.Empty(t, result.Values)
	}
	require.Equal(t, enum.JobStateCompleted, f.job.State)
	require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
	require.Equal(t, 1, f.results.writes)
}

func TestEvaluationRejectsInvalidCallbacksBeforeWriting(t *testing.T) {
	cases := map[string]func(*evaluationFixture){
		"invalid run ID":      func(f *evaluationFixture) { f.data.EvaluationRunID = "test-eval-run-002" },
		"invalid job ID":      func(f *evaluationFixture) { f.data.JobID = "test-job-002" },
		"unknown run":         func(f *evaluationFixture) { f.runs.findErr = mongo.ErrNoDocuments },
		"wrong job":           func(f *evaluationFixture) { f.data.JobID = primitive.NewObjectID().Hex() },
		"wrong plugin":        func(f *evaluationFixture) { f.data.Plugin = "stability" },
		"nonterminal status":  func(f *evaluationFixture) { f.data.Status = "RUNNING" },
		"partial results":     func(f *evaluationFixture) { f.data.EvaluationResults = f.data.EvaluationResults[:1] },
		"duplicate candidate": func(f *evaluationFixture) { f.data.EvaluationResults[1] = f.data.EvaluationResults[0] },
		"foreign candidate": func(f *evaluationFixture) {
			f.data.EvaluationResults[1].MutationResultID = primitive.NewObjectID().Hex()
		},
		"invalid candidate ID":  func(f *evaluationFixture) { f.data.EvaluationResults[0].MutationResultID = "bad" },
		"empty values":          func(f *evaluationFixture) { f.data.EvaluationResults[0].Values = nil },
		"completion with error": func(f *evaluationFixture) { f.data.Error = "bad" },
		"failure without error": func(f *evaluationFixture) { f.data.Status, f.data.EvaluationResults = "FAILED", nil },
		"failure with results":  func(f *evaluationFixture) { f.data.Status, f.data.Error = "FAILED", "bad" },
		"candidate deleted":     func(f *evaluationFixture) { delete(f.candidates, f.runs.run.MutationResultIds[0].Hex()) },
		"candidate wrong job": func(f *evaluationFixture) {
			f.candidates[f.runs.run.MutationResultIds[0].Hex()].JobId = primitive.NewObjectID()
		},
		"candidate wrong mutation": func(f *evaluationFixture) {
			f.candidates[f.runs.run.MutationResultIds[0].Hex()].MutationId = primitive.NewObjectID()
		},
		"candidate wrong user": func(f *evaluationFixture) { f.candidates[f.runs.run.MutationResultIds[0].Hex()].UserId = "other" },
		"invalid stored run":   func(f *evaluationFixture) { f.runs.run.Plugins = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEvaluationFixture(t)
			mutate(f)
			require.ErrorIs(t, f.con.receiveEvaluation(f.data), ErrInvalidEvaluation)
			require.NoError(t, f.con.OrchestrateWithError(f.message(t))) // Permanent invalid messages are acknowledged.
			require.Zero(t, f.results.writes)
			require.Zero(t, f.runs.finishes)
			require.Equal(t, enum.JobStateCompleted, f.job.State)
		})
	}
}

func TestEvaluationStorageFailuresCanRetry(t *testing.T) {
	for _, location := range []string{"find", "results", "finish"} {
		t.Run(location, func(t *testing.T) {
			f := newEvaluationFixture(t)
			failure := errors.New("database unavailable")
			switch location {
			case "find":
				f.runs.findErr = failure
			case "results":
				f.results.err = failure
			case "finish":
				f.runs.finishErr = failure
			}
			require.ErrorIs(t, f.con.OrchestrateWithError(f.message(t)), failure)
			require.Equal(t, models.EvaluationOngoing, f.runs.run.State)
			f.runs.findErr, f.results.err, f.runs.finishErr = nil, nil, nil
			require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
			require.Equal(t, models.EvaluationCompleted, f.runs.run.State)
			require.Len(t, f.results.items, 2)
		})
	}
}

func TestEvaluationPreservesGenericValues(t *testing.T) {
	f := newEvaluationFixture(t)
	f.data.EvaluationResults[0].Values = map[string]interface{}{
		"label": "mock", "confidence": 0.9, "details": map[string]interface{}{"valid": true},
	}
	require.NoError(t, f.con.OrchestrateWithError(f.message(t)))
	require.Equal(t, f.data.EvaluationResults[0].Values, f.results.items[f.runs.run.MutationResultIds[0]].Values)
}

func TestEvaluationMalformedJSONAndMissingRepositories(t *testing.T) {
	f := newEvaluationFixture(t)
	require.NoError(t, f.con.OrchestrateWithError("{"))
	f.con.evaluationRunRepository = nil
	require.Error(t, f.con.OrchestrateWithError(f.message(t)))
	require.Zero(t, f.results.writes)
}

// These fixtures were emitted by the uploaded 3.1 thread.py/mq.py, with only
// the RabbitMQ publisher replaced by a capture function and timestamp normalized.
func TestEvaluationUploadedWorkerCallbacks(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			f := newEvaluationFixture(t)
			body, err := os.ReadFile("testdata/mock_" + status + ".json")
			require.NoError(t, err)
			var payload Payload
			require.NoError(t, json.Unmarshal(body, &payload))
			f.job.Id, err = primitive.ObjectIDFromHex(payload.Data.JobID)
			require.NoError(t, err)
			f.runs.run.JobId = f.job.Id
			f.runs.run.Id, err = primitive.ObjectIDFromHex(payload.Data.EvaluationRunID)
			require.NoError(t, err)
			f.runs.run.MutationResultIds = nil
			f.candidates = make(map[string]*models.MutationResult)
			for _, hex := range []string{"650000000000000000000003", "650000000000000000000004"} {
				id, err := primitive.ObjectIDFromHex(hex)
				require.NoError(t, err)
				f.runs.run.MutationResultIds = append(f.runs.run.MutationResultIds, id)
				f.candidates[hex] = &models.MutationResult{Id: id, JobId: f.job.Id, MutationId: f.runs.run.MutationId, UserId: f.job.UserId}
			}
			require.NoError(t, f.con.OrchestrateWithError(string(body)))
			require.Equal(t, models.EvaluationState(payload.Data.Status), f.runs.run.State)
			require.Len(t, f.results.items, 2)
		})
	}
}
