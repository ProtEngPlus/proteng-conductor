package conductor

import (
	"testing"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type conductorDependencies struct {
	jobRepository      *mock_repository.MockJobRepository
	mutationRepository *mock_repository.MockMutationRepository
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

func newTestConductor(t *testing.T) (Conductor, *conductorDependencies, func()) {
	mockCtrl := gomock.NewController(t)

	deps := &conductorDependencies{
		jobRepository:      mock_repository.NewMockJobRepository(mockCtrl),
		mutationRepository: mock_repository.NewMockMutationRepository(mockCtrl),
	}

	finish := func() {
		mockCtrl.Finish()
	}

	return NewConductor(deps.jobRepository, deps.mutationRepository), deps, finish
}
