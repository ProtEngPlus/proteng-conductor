package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type dispatchFixture struct {
	con       *conductor
	deps      *conductorDependencies
	runs      *memoryEvaluationRuns
	job       *models.Job
	mutation  *models.Mutation
	data      Data
	sent      []byte
	events    []string
	latest    []latestEvaluation
	latestErr error
}

type latestEvaluation struct {
	method string
	runID  primitive.ObjectID
	state  models.EvaluationState
}

func recordLatestEvaluations(deps *conductorDependencies, calls *[]latestEvaluation, err *error) {
	record := func(method string) func(string, primitive.ObjectID, models.EvaluationState) error {
		return func(_ string, runID primitive.ObjectID, state models.EvaluationState) error {
			*calls = append(*calls, latestEvaluation{method, runID, state})
			return *err
		}
	}
	deps.jobRepository.EXPECT().SetLatestEvaluation(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(record("set")).AnyTimes()
	deps.jobRepository.EXPECT().FinishLatestEvaluation(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(record("finish")).AnyTimes()
}

func newDispatchFixture(t *testing.T, runType string, runID int) *dispatchFixture {
	con, deps, finish := newTestConductor(t)
	t.Cleanup(finish)
	job := &models.Job{
		Id:                primitive.NewObjectID(),
		Name:              "lipase",
		UserId:            "user-1",
		State:             enum.JobStateOnGoing,
		StageId:           3,
		InputProtein:      "AAAA",
		RunType:           runType,
		EvaluationPlugins: []string{"mock"},
		Meta:              []string{"blast", "unirep", "ridgecv", "mutation"},
		RunTime:           map[string]models.StageRunTime{},
	}
	mutation := &models.Mutation{
		Id:           primitive.NewObjectID(),
		JobId:        job.Id,
		RunId:        runID,
		InputProtein: job.InputProtein,
		State:        enum.MutationStateOnGoing,
	}
	f := &dispatchFixture{
		con: con, deps: deps, runs: &memoryEvaluationRuns{}, job: job, mutation: mutation,
		data: Data{
			JobID: job.Id.Hex(), MutationID: mutation.Id.Hex(), StageID: 3, Status: "COMPLETED",
			MutationResult: map[string]float32{"AAAC": 0.5, "AACA": 0.7},
		},
	}
	WithEvaluationRepositories(f.runs, &memoryEvaluationResults{items: map[primitive.ObjectID]*models.EvaluationResult{}})(con)
	recordLatestEvaluations(deps, &f.latest, &f.latestErr)
	return f
}

// expectMutationStored covers updateMutationData up to the job update.
func (f *dispatchFixture) expectMutationStored(jobFinds int) {
	f.deps.jobRepository.EXPECT().FindById(f.job.Id.Hex()).Return(f.job, nil).Times(jobFinds)
	f.deps.mutationRepository.EXPECT().FindById(f.mutation.Id.Hex()).Return(f.mutation, nil)
	f.deps.mutationResultRepository.EXPECT().DeleteByMutationId(f.mutation.Id.Hex()).Return(nil)
	f.deps.mutationResultRepository.EXPECT().Create(gomock.Any()).
		DoAndReturn(func(r *models.MutationResult) error { r.Id = primitive.NewObjectID(); return nil }).
		Times(len(f.data.MutationResult))
	f.deps.mutationRepository.EXPECT().Update(f.mutation.Id.Hex(), gomock.Any()).Return(nil)
	f.expectJobSaved()
}

func (f *dispatchFixture) expectJobSaved() {
	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).
		DoAndReturn(func(_ string, j *models.Job) error {
			f.events = append(f.events, "save "+string(j.State))
			return nil
		})
}

func (f *dispatchFixture) expectDispatch(err error) {
	f.deps.publisher.EXPECT().PublishWithTopic(gomock.Any(), "evaluation.mock", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, body []byte) error {
			f.events = append(f.events, "publish")
			f.sent = body
			return err
		})
}

var (
	savedOnGoing   = "save " + string(enum.JobStateOnGoing)
	savedCompleted = "save " + string(enum.JobStateCompleted)
)

func TestAfterMutation_autoJobStartsEvaluation(t *testing.T) {
	f := newDispatchFixture(t, "auto", 1)
	f.expectMutationStored(1)
	f.expectDispatch(nil)

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateOnGoing, state)
	require.Equal(t, enum.JobStateOnGoing, f.job.State)
	require.Equal(t, []string{savedOnGoing, "publish"}, f.events)
	require.Equal(t, 3, f.job.StageId)
	require.True(t, f.job.CompleteAt.IsZero())
	require.NotNil(t, f.runs.run)
	require.Equal(t, f.job.Id, f.runs.run.JobId)
	require.Equal(t, f.mutation.Id, f.runs.run.MutationId)
	require.Equal(t, []string{"mock"}, f.runs.run.Plugins)
	require.Len(t, f.runs.run.MutationResultIds, 2)
	require.Equal(t, []latestEvaluation{{"set", f.runs.run.Id, models.EvaluationOngoing}}, f.latest)

	var request map[string]interface{}
	require.NoError(t, json.Unmarshal(f.sent, &request))
	require.Equal(t, f.runs.run.Id.Hex(), request["evaluation_run_id"])
	require.Equal(t, "mock", request["plugin"])
	require.Equal(t, map[string]interface{}{"sequence": "AAAA"}, request["wild_type"])
	require.Len(t, request["mutants"], 2)
}

func TestAfterMutation_oneStepJobWaits(t *testing.T) {
	f := newDispatchFixture(t, "manual", 1)
	f.expectMutationStored(1)
	// no publish expected: gomock fails the test on an unexpected dispatch

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStatePending, state)
	require.Equal(t, enum.JobStatePending, f.job.State)
	require.Nil(t, f.runs.run)
	require.Equal(t, []latestEvaluation{{"set", primitive.NilObjectID, models.EvaluationPending}}, f.latest)
}

func TestAfterMutation_failedStateWriteDoesNotStopEvaluation(t *testing.T) {
	f := newDispatchFixture(t, "auto", 1)
	f.latestErr = errors.New("database unavailable")
	f.expectMutationStored(1)
	f.expectDispatch(nil)

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateOnGoing, state)
	require.Equal(t, []string{savedOnGoing, "publish"}, f.events)
	require.Equal(t, models.EvaluationOngoing, f.runs.run.State)
	require.Len(t, f.latest, 1)
}

func TestAfterMutation_extraMutationRunEvaluatesAtOnce(t *testing.T) {
	f := newDispatchFixture(t, "manual", 2)
	f.expectMutationStored(1)
	f.expectDispatch(nil)

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateOnGoing, state)
	require.NotNil(t, f.runs.run)
}

func TestAfterMutation_dispatchFailureCompletesJob(t *testing.T) {
	f := newDispatchFixture(t, "auto", 1)
	f.expectMutationStored(1)
	f.expectDispatch(errors.New("broker down"))
	f.deps.jobRepository.EXPECT().AddErrorLog(f.job.Id.Hex(), gomock.Any()).Return(nil)
	f.expectJobSaved()

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateCompleted, state)
	require.Equal(t, []string{savedOnGoing, "publish", savedCompleted}, f.events)
	require.False(t, f.job.CompleteAt.IsZero())
	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
	require.Equal(t, []latestEvaluation{
		{"set", f.runs.run.Id, models.EvaluationOngoing},
		{"finish", f.runs.run.Id, models.EvaluationFailed},
	}, f.latest)
}

func TestOrchestrateJob_resumesWaitingEvaluation(t *testing.T) {
	f := newDispatchFixture(t, "manual", 1)
	f.mutation.State = enum.MutationStateCompleted
	results := []*models.MutationResult{
		{Id: primitive.NewObjectID(), JobId: f.job.Id, MutationId: f.mutation.Id, ProteinSequence: "AAAC"},
		{Id: primitive.NewObjectID(), JobId: f.job.Id, MutationId: f.mutation.Id, ProteinSequence: "AACA"},
	}
	f.deps.mutationRepository.EXPECT().GetAll(gomock.Any()).Return([]*models.Mutation{f.mutation}, nil)
	f.deps.mutationResultRepository.EXPECT().GetAll(gomock.Any()).Return(results, nil)
	f.expectJobSaved()
	f.expectDispatch(nil)

	require.NoError(t, f.con.OrchestrateJob(f.job))
	require.Equal(t, []string{savedOnGoing, "publish"}, f.events)
	require.NotNil(t, f.runs.run)
	require.Equal(t, []primitive.ObjectID{results[0].Id, results[1].Id}, f.runs.run.MutationResultIds)
	require.Equal(t, []latestEvaluation{{"set", f.runs.run.Id, models.EvaluationOngoing}}, f.latest)
}

func TestOrchestrateJob_resumeDispatchFailureCompletesJob(t *testing.T) {
	f := newDispatchFixture(t, "manual", 1)
	f.mutation.State = enum.MutationStateCompleted
	results := []*models.MutationResult{
		{Id: primitive.NewObjectID(), JobId: f.job.Id, MutationId: f.mutation.Id, ProteinSequence: "AAAC"},
	}
	f.deps.mutationRepository.EXPECT().GetAll(gomock.Any()).Return([]*models.Mutation{f.mutation}, nil)
	f.deps.mutationResultRepository.EXPECT().GetAll(gomock.Any()).Return(results, nil)
	f.expectJobSaved()
	f.expectDispatch(errors.New("broker down"))
	f.deps.jobRepository.EXPECT().AddErrorLog(f.job.Id.Hex(), gomock.Any()).Return(nil)
	f.expectJobSaved()

	require.NoError(t, f.con.OrchestrateJob(f.job))

	require.Equal(t, []string{savedOnGoing, "publish", savedCompleted}, f.events)
	require.Equal(t, enum.JobStateCompleted, f.job.State)
	require.False(t, f.job.CompleteAt.IsZero())
	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
}

func TestUpdateJobData_mutationEmails(t *testing.T) {
	t.Run("no email while the evaluation runs", func(tt *testing.T) {
		f := newDispatchFixture(tt, "auto", 1)
		f.job.IsNotificationOn = true
		f.expectMutationStored(2)
		f.expectDispatch(nil)
		// no PublishDefaultExchange expected

		require.Nil(tt, f.con.updateJobData(f.data))
	})

	t.Run("one-step job is told that it waits", func(tt *testing.T) {
		f := newDispatchFixture(tt, "manual", 1)
		f.job.IsNotificationOn = true
		f.expectMutationStored(2)
		f.deps.publisher.EXPECT().
			PublishDefaultExchange(gomock.Any(), "job_status_email_notification", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, body []byte) error {
				var message map[string]string
				require.NoError(tt, json.Unmarshal(body, &message))
				require.Equal(tt, string(enum.JobStatePending), message["job_state"])
				return nil
			})

		require.Nil(tt, f.con.updateJobData(f.data))
	})
}

func TestEvaluationCompletesWaitingJob(t *testing.T) {
	for _, status := range []string{"COMPLETED", "FAILED"} {
		t.Run("plugin "+status+" completes the job", func(tt *testing.T) {
			f := newEvaluationFixture(tt)
			f.job.State = enum.JobStateOnGoing
			if status == "FAILED" {
				f.data.Status, f.data.Error, f.data.EvaluationResults = "FAILED", "mock failed", nil
			}
			f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)

			require.NoError(tt, f.con.OrchestrateWithError(f.message(tt)))

			require.Equal(tt, models.EvaluationState(status), f.runs.run.State)
			require.Equal(tt, enum.JobStateCompleted, f.job.State)
			require.Equal(tt, 3, f.job.StageId)
			require.False(tt, f.job.CompleteAt.IsZero())
		})
	}

	t.Run("completed job is emailed once", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.job.IsNotificationOn = true
		f.job.Meta = []string{"blast", "unirep", "ridgecv", "mutation"}
		f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)
		f.deps.publisher.EXPECT().
			PublishDefaultExchange(gomock.Any(), "job_status_email_notification", gomock.Any()).
			Return(nil)

		require.NoError(tt, f.con.OrchestrateWithError(f.message(tt)))
		require.NoError(tt, f.con.OrchestrateWithError(f.message(tt))) // redelivery: no second email
		require.Equal(tt, enum.JobStateCompleted, f.job.State)
	})

	t.Run("failed job update is retried by the redelivered message", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		failure := errors.New("database unavailable")
		gomock.InOrder(
			f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).
				DoAndReturn(func(_ string, j *models.Job) error { j.State = enum.JobStateOnGoing; return failure }),
			f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil),
		)

		require.ErrorIs(tt, f.con.OrchestrateWithError(f.message(tt)), failure)
		require.Equal(tt, models.EvaluationCompleted, f.runs.run.State)
		require.NoError(tt, f.con.OrchestrateWithError(f.message(tt)))
		require.Equal(tt, enum.JobStateCompleted, f.job.State)
		require.Equal(tt, 1, f.results.writes)
	})
}

func TestRerunEvaluation(t *testing.T) {
	results := func(f *dispatchFixture) []*models.MutationResult {
		return []*models.MutationResult{
			{Id: primitive.NewObjectID(), JobId: f.job.Id, MutationId: f.mutation.Id, ProteinSequence: "AAAC"},
		}
	}

	t.Run("starts a rerun with the requested plugins and leaves the job alone", func(tt *testing.T) {
		f := newDispatchFixture(tt, "auto", 1)
		f.job.State = enum.JobStateCompleted
		f.deps.mutationResultRepository.EXPECT().GetAll(gomock.Any()).Return(results(f), nil)
		f.expectDispatch(nil)

		run, err := f.con.RerunEvaluation(f.job, f.mutation, []string{"mock"})

		require.NoError(tt, err)
		require.True(tt, run.Rerun)
		require.Equal(tt, []string{"mock"}, run.Plugins)
		require.Equal(tt, []string{"publish"}, f.events)
		require.Equal(tt, enum.JobStateCompleted, f.job.State)
		require.Equal(tt, []latestEvaluation{{"set", run.Id, models.EvaluationOngoing}}, f.latest)
	})

	t.Run("rejects a rerun while another run of the job is ongoing", func(tt *testing.T) {
		f := newDispatchFixture(tt, "auto", 1)
		f.runs.run = &models.EvaluationRun{Id: primitive.NewObjectID(), JobId: f.job.Id, State: models.EvaluationOngoing}

		_, err := f.con.RerunEvaluation(f.job, f.mutation, []string{"mock"})

		require.ErrorIs(tt, err, ErrEvaluationRunning)
		require.Empty(tt, f.events)
		require.Empty(tt, f.latest)
	})

	t.Run("rejects a mutation run without results", func(tt *testing.T) {
		f := newDispatchFixture(tt, "auto", 1)
		f.deps.mutationResultRepository.EXPECT().GetAll(gomock.Any()).Return([]*models.MutationResult{}, nil)

		_, err := f.con.RerunEvaluation(f.job, f.mutation, []string{"mock"})

		require.ErrorIs(tt, err, ErrNoEvaluationCandidates)
	})
}

func TestRunMutation_clearsTheLatestEvaluation(t *testing.T) {
	for _, clearErr := range []error{nil, errors.New("database unavailable")} {
		f := newDispatchFixture(t, "auto", 2)
		previousRun := primitive.NewObjectID()
		f.job.State = enum.JobStateCompleted
		f.job.EvaluationState = models.EvaluationCompleted
		f.job.EvaluationRunId = &previousRun
		f.mutation.State = enum.MutationStatePending
		f.deps.jobRepository.EXPECT().FindById(f.job.Id.Hex()).Return(f.job, nil)
		f.deps.mutationRepository.EXPECT().Update(f.mutation.Id.Hex(), gomock.Any()).Return(nil)
		f.expectJobSaved()
		f.deps.jobRepository.EXPECT().ClearLatestEvaluation(f.job.Id.Hex()).
			DoAndReturn(func(string) error {
				f.events = append(f.events, "clear")
				return clearErr
			})
		f.deps.publisher.EXPECT().PublishWithTopic(gomock.Any(), "mutation.", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []byte) error {
				f.events = append(f.events, "publish")
				return nil
			})

		require.NoError(t, f.con.RunMutation(f.mutation))
		require.Equal(t, []string{savedOnGoing, "clear", "publish"}, f.events)
	}
}
