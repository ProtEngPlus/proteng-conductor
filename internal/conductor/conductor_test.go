package conductor

import (
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

func TestConductor_updateMutationData_redelivery(t *testing.T) {
	t.Parallel()

	newJob := func() *models.Job {
		return &models.Job{
			Id:      primitive.NewObjectID(),
			UserId:  "user-1",
			State:   enum.JobStateOnGoing,
			StageId: 3,
			RunTime: map[string]models.StageRunTime{},
		}
	}
	newMutation := func(jobID primitive.ObjectID, state enum.MutationState) *models.Mutation {
		return &models.Mutation{
			Id:           primitive.NewObjectID(),
			JobId:        jobID,
			RunId:        1,
			InputProtein: "AAAA",
			State:        state,
		}
	}

	for _, state := range []enum.MutationState{enum.MutationStateCompleted, enum.MutationStateFailed} {
		t.Run("message for a "+string(state)+" mutation is ignored", func(tt *testing.T) {
			conductor, deps, finish := newTestConductor(tt)
			defer finish()

			job := newJob()
			mutation := newMutation(job.Id, state)
			deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
			deps.mutationRepository.EXPECT().FindById(mutation.Id.Hex()).Return(mutation, nil)
			// no Create, Delete or Update: gomock fails the test on unexpected calls

			conductor.updateMutationData(Data{
				JobID:          job.Id.Hex(),
				MutationID:     mutation.Id.Hex(),
				StageID:        3,
				Status:         "COMPLETED",
				MutationResult: map[string]float32{"AAAC": 0.5},
			})

			assert.Equal(tt, enum.JobStateOnGoing, job.State)
		})
	}

	t.Run("results of an ongoing mutation replace earlier partial results", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob()
		mutation := newMutation(job.Id, enum.MutationStateOnGoing)
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.mutationRepository.EXPECT().FindById(mutation.Id.Hex()).Return(mutation, nil)
		gomock.InOrder(
			deps.mutationResultRepository.EXPECT().DeleteByMutationId(mutation.Id.Hex()).Return(nil),
			deps.mutationResultRepository.EXPECT().Create(gomock.Any()).Return(nil).Times(2),
		)
		deps.mutationRepository.EXPECT().Update(mutation.Id.Hex(), gomock.Any()).Return(nil)
		deps.jobRepository.EXPECT().Update(job.Id.Hex(), gomock.Any()).Return(nil)

		conductor.updateMutationData(Data{
			JobID:          job.Id.Hex(),
			MutationID:     mutation.Id.Hex(),
			StageID:        3,
			Status:         "COMPLETED",
			MutationResult: map[string]float32{"AAAC": 0.5, "AACA": 0.7},
		})

		assert.Equal(tt, enum.MutationStateCompleted, mutation.State)
		assert.Equal(tt, enum.JobStateCompleted, job.State)
	})

	t.Run("failing to clear earlier results stores nothing", func(tt *testing.T) {
		conductor, deps, finish := newTestConductor(tt)
		defer finish()

		job := newJob()
		mutation := newMutation(job.Id, enum.MutationStateOnGoing)
		deps.jobRepository.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		deps.mutationRepository.EXPECT().FindById(mutation.Id.Hex()).Return(mutation, nil)
		deps.mutationResultRepository.EXPECT().DeleteByMutationId(mutation.Id.Hex()).Return(assert.AnError)

		conductor.updateMutationData(Data{
			JobID:          job.Id.Hex(),
			MutationID:     mutation.Id.Hex(),
			StageID:        3,
			Status:         "COMPLETED",
			MutationResult: map[string]float32{"AAAC": 0.5},
		})

		assert.Equal(tt, enum.JobStateOnGoing, job.State)
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
