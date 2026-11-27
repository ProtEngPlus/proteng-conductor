package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

func TestExperimentalResultController(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mutationResult := &models.MutationResult{
		Id:         primitive.NewObjectID(),
		MutationId: primitive.NewObjectID(),
		JobId:      primitive.NewObjectID(),
		UserId:     "owner",

		ProteinSequence: "MKV",
	}
	jobAssayTypeId := primitive.NewObjectID()
	url := "/mutations/experimental-results/by-mutation-result/" + mutationResult.Id.Hex()

	setup := func(tt *testing.T) (*gin.Engine, *mock_repository.MockMutationResultRepository, *mock_repository.MockExperimentalResultRepository) {
		ctrl := gomock.NewController(tt)
		mrr := mock_repository.NewMockMutationResultRepository(ctrl)
		exr := mock_repository.NewMockExperimentalResultRepository(ctrl)
		jr := mock_repository.NewMockJobRepository(ctrl)
		jr.EXPECT().FindById(mutationResult.JobId.Hex()).Return(&models.Job{AssayTypeId: jobAssayTypeId}, nil).AnyTimes()
		atr := mock_repository.NewMockAssayTypeRepository(ctrl)
		erc := NewExperimentalResultController(mrr, exr, jr, atr)
		router := gin.New()
		router.GET("/mutations/experimental-results", erc.GetAllExperimentalResults)
		router.PUT("/mutations/experimental-results/by-mutation-result/:mutationResultId", erc.UpsertExperimentalResult)
		router.DELETE("/mutations/experimental-results/by-mutation-result/:mutationResultId", erc.DeleteExperimentalResult)
		return router, mrr, exr
	}

	send := func(router *gin.Engine, method string, path string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	t.Run("upsert returns 404 when mutation result is missing", func(tt *testing.T) {
		router, mrr, _ := setup(tt)
		mrr.EXPECT().FindById(mutationResult.Id.Hex()).Return(nil, fmt.Errorf("not found"))

		w := send(router, http.MethodPut, url, `{"user_id":"owner","actual_assay_score":1.5}`)
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("upsert returns 403 for another user", func(tt *testing.T) {
		router, mrr, _ := setup(tt)
		mrr.EXPECT().FindById(mutationResult.Id.Hex()).Return(mutationResult, nil)

		w := send(router, http.MethodPut, url, `{"user_id":"someone-else","actual_assay_score":1.5}`)
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("upsert returns 400 when actual_assay_score is missing", func(tt *testing.T) {
		router, _, _ := setup(tt)

		w := send(router, http.MethodPut, url, `{"user_id":"owner","note":"no score"}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
	})

	t.Run("upsert keeps a score of zero", func(tt *testing.T) {
		router, mrr, exr := setup(tt)
		mrr.EXPECT().FindById(mutationResult.Id.Hex()).Return(mutationResult, nil)
		exr.EXPECT().Upsert(mutationResult.Id.Hex(), gomock.Any()).DoAndReturn(func(_ string, result *models.ExperimentalResult) error {
			assert.Equal(tt, float32(0), result.ActualAssayScore)
			return nil
		})

		w := send(router, http.MethodPut, url, `{"user_id":"owner","actual_assay_score":0}`)
		assert.Equal(tt, http.StatusOK, w.Code)
	})

	t.Run("upsert takes ids from mutation result and defaults measured_at", func(tt *testing.T) {
		router, mrr, exr := setup(tt)
		mrr.EXPECT().FindById(mutationResult.Id.Hex()).Return(mutationResult, nil)
		exr.EXPECT().Upsert(mutationResult.Id.Hex(), gomock.Any()).DoAndReturn(func(_ string, result *models.ExperimentalResult) error {
			assert.Equal(tt, mutationResult.Id, result.MutationResultId)
			assert.Equal(tt, mutationResult.MutationId, result.MutationId)
			assert.Equal(tt, mutationResult.JobId, result.JobId)
			assert.Equal(tt, jobAssayTypeId, result.AssayTypeId)
			assert.Equal(tt, "MKV", result.ProteinSequence)
			assert.Equal(tt, float32(1.5), result.ActualAssayScore)
			assert.False(tt, result.MeasuredAt.IsZero())
			return nil
		})

		body := fmt.Sprintf(`{"user_id":"owner","actual_assay_score":1.5,"job_id":"%s"}`, primitive.NewObjectID().Hex())
		w := send(router, http.MethodPut, url, body)
		assert.Equal(tt, http.StatusOK, w.Code)
	})

	t.Run("delete returns 403 for another user", func(tt *testing.T) {
		router, _, exr := setup(tt)
		exr.EXPECT().FindByMutationResultId(mutationResult.Id.Hex()).Return(&models.ExperimentalResult{UserId: "owner"}, nil)

		w := send(router, http.MethodDelete, url+"?user_id=someone-else", "")
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("delete removes the owner's result", func(tt *testing.T) {
		router, _, exr := setup(tt)
		exr.EXPECT().FindByMutationResultId(mutationResult.Id.Hex()).Return(&models.ExperimentalResult{UserId: "owner"}, nil)
		exr.EXPECT().DeleteByMutationResultId(mutationResult.Id.Hex()).Return(nil)

		w := send(router, http.MethodDelete, url+"?user_id=owner", "")
		assert.Equal(tt, http.StatusOK, w.Code)
	})

	t.Run("get passes filters", func(tt *testing.T) {
		router, _, exr := setup(tt)
		exr.EXPECT().GetAll(map[string]interface{}{"job_id": "j", "user_id": "owner"}).Return([]*models.ExperimentalResult{}, nil)

		w := send(router, http.MethodGet, "/mutations/experimental-results?job_id=j&user_id=owner", "")
		assert.Equal(tt, http.StatusOK, w.Code)
	})
}

func TestExportExperimentalResults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	job := &models.Job{Id: primitive.NewObjectID(), Name: "My job", UserId: "owner", AssayTypeId: primitive.NewObjectID()}
	jobId := job.Id.Hex()

	setup := func(tt *testing.T) (*gin.Engine, *mock_repository.MockJobRepository, *mock_repository.MockExperimentalResultRepository, *mock_repository.MockAssayTypeRepository) {
		ctrl := gomock.NewController(tt)
		jr := mock_repository.NewMockJobRepository(ctrl)
		exr := mock_repository.NewMockExperimentalResultRepository(ctrl)
		atr := mock_repository.NewMockAssayTypeRepository(ctrl)
		erc := NewExperimentalResultController(mock_repository.NewMockMutationResultRepository(ctrl), exr, jr, atr)
		router := gin.New()
		router.GET("/mutations/experimental-results/export", erc.ExportExperimentalResults)
		return router, jr, exr, atr
	}

	get := func(router *gin.Engine, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/mutations/experimental-results/export?"+query, nil))
		return w
	}

	t.Run("403 for another user", func(tt *testing.T) {
		router, jr, _, _ := setup(tt)
		jr.EXPECT().FindById(jobId).Return(job, nil)

		assert.Equal(tt, http.StatusForbidden, get(router, "job_id="+jobId+"&user_id=other").Code)
	})

	t.Run("404 when the job has no results", func(tt *testing.T) {
		router, jr, exr, _ := setup(tt)
		jr.EXPECT().FindById(jobId).Return(job, nil)
		exr.EXPECT().GetAll(map[string]interface{}{"job_id": jobId, "user_id": "owner"}).Return([]*models.ExperimentalResult{}, nil)

		assert.Equal(tt, http.StatusNotFound, get(router, "job_id="+jobId+"&user_id=owner").Code)
	})

	t.Run("csv is named after the job and the type", func(tt *testing.T) {
		router, jr, exr, atr := setup(tt)
		jr.EXPECT().FindById(jobId).Return(job, nil)
		exr.EXPECT().GetAll(map[string]interface{}{"job_id": jobId, "user_id": "owner"}).Return([]*models.ExperimentalResult{
			{ProteinSequence: "MKV", ActualAssayScore: 1.25},
		}, nil)
		atr.EXPECT().FindById(job.AssayTypeId.Hex()).Return(&models.AssayType{Name: "Yield"}, nil)

		w := get(router, "job_id="+jobId+"&user_id=owner")
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Equal(tt, "MKV,1.25\n", w.Body.String())
		assert.Contains(tt, w.Header().Get("Content-Disposition"), "My_job_Yield.csv")
	})
}
