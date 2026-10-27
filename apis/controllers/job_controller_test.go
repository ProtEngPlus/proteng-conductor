package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

func TestDeleteJobRemovesEvaluations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)

	jobID, runID := primitive.NewObjectID(), primitive.NewObjectID()
	id := jobID.Hex()
	jr := mock_repository.NewMockJobRepository(ctrl)
	mr := mock_repository.NewMockMutationRepository(ctrl)
	mrr := mock_repository.NewMockMutationResultRepository(ctrl)
	cr := mock_repository.NewMockConfigurationRepository(ctrl)
	qr := mock_repository.NewMockQueryResultRepository(ctrl)
	exr := mock_repository.NewMockExperimentalResultRepository(ctrl)
	jr.EXPECT().Delete(id).Return(nil)
	exr.EXPECT().DeleteByJobId(id).Return(nil)
	mrr.EXPECT().DeleteByJobId(id).Return(nil)
	mr.EXPECT().DeleteByJobId(id).Return(nil)
	cr.EXPECT().DeleteByJobId(id).Return(nil)
	qr.EXPECT().DeleteByJobId(id).Return(nil)
	runs := &fakeEvaluationRuns{runs: []*models.EvaluationRun{
		{Id: runID, JobId: jobID},
		{Id: primitive.NewObjectID(), JobId: primitive.NewObjectID()},
	}}
	results := &fakeEvaluationResults{}

	jc := NewJobController(jr, mr, mrr, cr, qr, exr, runs, results, nil)
	router := gin.New()
	router.DELETE("/jobs/:id", jc.DeleteJob)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/jobs/"+id, nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []primitive.ObjectID{runID}, results.deletedRuns)
	assert.Equal(t, []string{id}, runs.deletedJobs)
}
