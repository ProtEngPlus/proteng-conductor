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
	con      *conductor
	deps     *conductorDependencies
	runs     *memoryEvaluationRuns
	job      *models.Job
	mutation *models.Mutation
	data     Data
	sent     []byte
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
	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)
}

func (f *dispatchFixture) expectDispatch(err error) {
	f.deps.publisher.EXPECT().PublishWithTopic(gomock.Any(), "evaluation.mock", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, body []byte) error { f.sent = body; return err })
}

func TestAfterMutation_autoJobStartsEvaluation(t *testing.T) {
	f := newDispatchFixture(t, "auto", 1)
	f.expectMutationStored(1)
	f.expectDispatch(nil)

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateOnGoing, state)
	require.Equal(t, enum.JobStateOnGoing, f.job.State)
	require.Equal(t, 3, f.job.StageId)
	require.True(t, f.job.CompleteAt.IsZero())
	require.NotNil(t, f.runs.run)
	require.Equal(t, f.job.Id, f.runs.run.JobId)
	require.Equal(t, f.mutation.Id, f.runs.run.MutationId)
	require.Equal(t, []string{"mock"}, f.runs.run.Plugins)
	require.Len(t, f.runs.run.MutationResultIds, 2)

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

	state := f.con.updateMutationData(f.data)

	require.Equal(t, enum.JobStateCompleted, state)
	require.False(t, f.job.CompleteAt.IsZero())
	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
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
	f.expectDispatch(nil)
	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).
		DoAndReturn(func(_ string, j *models.Job) error {
			require.Equal(t, enum.JobStateOnGoing, j.State)
			return nil
		})

	require.NoError(t, f.con.OrchestrateJob(f.job))
	require.NotNil(t, f.runs.run)
	require.Equal(t, []primitive.ObjectID{results[0].Id, results[1].Id}, f.runs.run.MutationResultIds)
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
