package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/internal/rabbitmq/publisher/mock_publisher"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
	"go.uber.org/zap"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMain(m *testing.M) {
	logger.Zap = zap.NewNop()
	os.Exit(m.Run())
}

func TestConductor_getFirstMutation(t *testing.T) {
	t.Parallel()

	var testJobId = primitive.NewObjectID()
	testJob := models.Job{
		Id:           testJobId,
		InputProtein: "testprotein",
		Meta: []string{
			"blast",
			"unirep",
			"ridgecv",
			"ridgecv",
		},
		Options: map[string]interface{}{
			"ridgecv": make(map[string]interface{}),
		},
	}

	t.Run("get first mutation create new mutation", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		deps.mutationRepository.EXPECT().GetAll(gomock.Any()).Return([]*models.Mutation{}, nil)
		deps.mutationRepository.EXPECT().Create(gomock.Any()).Return(nil)

		mutation, err := conductor.getCurrentMutation(&testJob)
		assert.Nil(tt, err)
		assert.Equal(tt, mutation.JobId, testJobId)
		assert.Equal(tt, mutation.InputProtein, testJob.InputProtein)
	})

	t.Run("get first mutation return existing mutation", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		existingMutation := &models.Mutation{
			JobId:        testJobId,
			InputProtein: testJob.InputProtein,
		}
		deps.mutationRepository.EXPECT().GetAll(gomock.Any()).Return([]*models.Mutation{existingMutation}, nil)

		mutation, err := conductor.getCurrentMutation(&testJob)
		assert.Nil(tt, err)
		assert.Equal(tt, mutation, existingMutation)
	})
}

func TestCheckStage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		jobStage int
		msgStage int
		want     stageCheck
	}{
		{"same stage", 1, 1, stageMatch},
		{"message for a passed stage", 2, 0, stageStale},
		{"message one stage behind", 3, 2, stageStale},
		{"message for a future stage", 1, 3, stageAhead},
		{"message one stage ahead", 0, 1, stageAhead},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(tt *testing.T) {
			assert.Equal(tt, tc.want, checkStage(tc.jobStage, tc.msgStage))
		})
	}
}

func TestConductor_updateJobData_stageMismatch(t *testing.T) {
	t.Parallel()

	newJob := func(stageID int, notify bool) *models.Job {
		return &models.Job{
			Id:               primitive.NewObjectID(),
			Name:             "test job",
			UserId:           "user-1",
			State:            enum.JobStateOnGoing,
			StageId:          stageID,
			Meta:             []string{"blast", "unirep", "ridgecv", "mutation"},
			IsNotificationOn: notify,
		}
	}

	t.Run("stale message is ignored and job keeps running", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob(2, true)
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().AddErrorLog(job.Id.Hex(), gomock.Any()).Return(nil)
		// no Update and no publish: gomock fails the test on unexpected calls

		got := conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 0, Status: "COMPLETED"})

		assert.Nil(tt, got)
		assert.Equal(tt, enum.JobStateOnGoing, job.State)
		assert.Equal(tt, 2, job.StageId)
	})

	t.Run("message ahead of job fails the job and notifies", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob(1, true)
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).
			DoAndReturn(func(_ string, j *models.Job) error {
				assert.Equal(tt, enum.JobStateFailed, j.State)
				return nil
			})
		deps.jobRepository.EXPECT().AddErrorLog(job.Id.Hex(), gomock.Any()).Return(nil)
		deps.publisher.EXPECT().
			PublishDefaultExchange(gomock.Any(), "job_status_email_notification", gomock.Any()).
			Return(nil)

		got := conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 3, Status: "COMPLETED"})

		assert.Nil(tt, got)
		assert.Equal(tt, enum.JobStateFailed, job.State)
	})

	t.Run("message ahead of job fails the job without email when notification is off", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob(1, false)
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().AddErrorLog(job.Id.Hex(), gomock.Any()).Return(nil)

		got := conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 3, Status: "COMPLETED"})

		assert.Nil(tt, got)
		assert.Equal(tt, enum.JobStateFailed, job.State)
	})
}

type conductorDependencies struct {
	jobRepository            *mock_repository.MockJobRepository
	mutationRepository       *mock_repository.MockMutationRepository
	queryResultRepository    *mock_repository.MockQueryResultRepository
	mutationResultRepository *mock_repository.MockMutationResultRepository
	publisher                *mock_publisher.MockPublisher
}

func newTestConductor(t *testing.T) (*conductor, *conductorDependencies, func()) {
	mockCtrl := gomock.NewController(t)

	deps := &conductorDependencies{
		jobRepository:            mock_repository.NewMockJobRepository(mockCtrl),
		mutationRepository:       mock_repository.NewMockMutationRepository(mockCtrl),
		queryResultRepository:    mock_repository.NewMockQueryResultRepository(mockCtrl),
		mutationResultRepository: mock_repository.NewMockMutationResultRepository(mockCtrl),
		publisher:                mock_publisher.NewMockPublisher(mockCtrl),
	}

	finish := func() {
		mockCtrl.Finish()
	}

	return NewConductor(deps.jobRepository, deps.mutationRepository, deps.queryResultRepository, deps.mutationResultRepository, deps.publisher), deps, finish
}

func expectStatusEmail(t *testing.T, deps *conductorDependencies, want enum.JobState) {
	deps.publisher.EXPECT().
		PublishDefaultExchange(gomock.Any(), "job_status_email_notification", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, body []byte) error {
			var msg map[string]string
			assert.NoError(t, json.Unmarshal(body, &msg))
			assert.Equal(t, string(want), msg["job_state"])
			return nil
		})
}

func TestConductor_updateJobData_notification(t *testing.T) {
	t.Parallel()

	newJob := func(runType string) *models.Job {
		return &models.Job{
			Id:               primitive.NewObjectID(),
			Name:             "test job",
			UserId:           "user-1",
			State:            enum.JobStateOnGoing,
			StageId:          1,
			Meta:             []string{"blast", "unirep", "ridgecv", "mutation"},
			RunType:          runType,
			RunTime:          map[string]models.StageRunTime{},
			LabResult:        models.LabResult{Total: 1},
			IsNotificationOn: true,
		}
	}

	t.Run("one-step job emails when a stage finishes", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob("one-step")
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		expectStatusEmail(tt, deps, enum.JobStatePending)

		got := conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 1, Status: "COMPLETED"})

		assert.Equal(tt, enum.JobStatePending, got.State)
	})

	t.Run("auto job does not email when a stage finishes", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob("auto")
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		// no publish expected: gomock fails the test on an unexpected email

		got := conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 1, Status: "COMPLETED"})

		assert.Equal(tt, enum.JobStateOnGoing, got.State)
	})

	t.Run("auto job emails when a stage fails", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob("auto")
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().AddErrorLog(job.Id.Hex(), gomock.Any()).Return(nil)
		expectStatusEmail(tt, deps, enum.JobStateFailed)

		conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 1, Status: "FAILED", Error: "boom"})

		assert.Equal(tt, enum.JobStateFailed, job.State)
	})

	t.Run("no email when notification is off", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob("one-step")
		job.IsNotificationOn = false
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)

		conductor.updateJobData(Data{JobID: job.Id.Hex(), StageID: 1, Status: "COMPLETED"})
	})
}

func TestConductor_updateJobData_mutationNotification(t *testing.T) {
	t.Parallel()

	newJob := func() *models.Job {
		return &models.Job{
			Id:               primitive.NewObjectID(),
			Name:             "test job",
			UserId:           "user-1",
			State:            enum.JobStateOnGoing,
			StageId:          3,
			Meta:             []string{"blast", "unirep", "ridgecv", "mutation"},
			RunType:          "auto",
			RunTime:          map[string]models.StageRunTime{},
			IsNotificationOn: true,
		}
	}
	mutationID := primitive.NewObjectID()

	t.Run("completed mutation run emails completed", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob()
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil).Times(2)
		deps.mutationRepository.EXPECT().FindById(mutationID.Hex()).Return(&models.Mutation{Id: mutationID}, nil)
		deps.mutationRepository.EXPECT().Update(mutationID.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		expectStatusEmail(tt, deps, enum.JobStateCompleted)

		conductor.updateJobData(Data{JobID: job.Id.Hex(), MutationID: mutationID.Hex(), StageID: 3, Status: "COMPLETED"})
	})

	t.Run("failed mutation run emails failed", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob()
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil).Times(2)
		deps.mutationRepository.EXPECT().FindById(mutationID.Hex()).Return(&models.Mutation{Id: mutationID}, nil)
		deps.mutationRepository.EXPECT().Update(mutationID.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().AddErrorLog(job.Id.Hex(), gomock.Any()).Return(nil)
		expectStatusEmail(tt, deps, enum.JobStateFailed)

		conductor.updateJobData(Data{JobID: job.Id.Hex(), MutationID: mutationID.Hex(), StageID: 3, Status: "FAILED", Error: "boom"})
	})

	t.Run("no email when the mutation result could not be saved", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob()
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil).Times(2)
		deps.mutationRepository.EXPECT().FindById(mutationID.Hex()).Return(nil, errors.New("mongo down"))

		conductor.updateJobData(Data{JobID: job.Id.Hex(), MutationID: mutationID.Hex(), StageID: 3, Status: "COMPLETED"})
	})
}

func TestShouldNotify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		on      bool
		runType string
		state   enum.JobState
		want    bool
	}{
		{"off never emails", false, "one-step", enum.JobStateFailed, false},
		{"one-step emails when waiting for the next step", true, "one-step", enum.JobStatePending, true},
		{"one-step emails on completion", true, "one-step", enum.JobStateCompleted, true},
		{"one-step emails on failure", true, "one-step", enum.JobStateFailed, true},
		{"auto skips stages still running", true, "auto", enum.JobStateOnGoing, false},
		{"auto emails on completion", true, "auto", enum.JobStateCompleted, true},
		{"auto emails on failure", true, "auto", enum.JobStateFailed, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(tt *testing.T) {
			job := &models.Job{IsNotificationOn: tc.on, RunType: tc.runType}
			assert.Equal(tt, tc.want, shouldNotify(job, tc.state))
		})
	}
}
