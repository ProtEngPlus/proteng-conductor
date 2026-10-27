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

func TestJobControllerAssay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	assayType := &models.AssayType{Id: primitive.NewObjectID(), UserId: "owner", Name: "Yield"}
	typeId := assayType.Id.Hex()

	type mocks struct {
		jr  *mock_repository.MockJobRepository
		mrr *mock_repository.MockMutationResultRepository
		exr *mock_repository.MockExperimentalResultRepository
		atr *mock_repository.MockAssayTypeRepository
	}

	setup := func(tt *testing.T) (*JobController, *gin.Engine, mocks) {
		ctrl := gomock.NewController(tt)
		m := mocks{
			jr:  mock_repository.NewMockJobRepository(ctrl),
			mrr: mock_repository.NewMockMutationResultRepository(ctrl),
			exr: mock_repository.NewMockExperimentalResultRepository(ctrl),
			atr: mock_repository.NewMockAssayTypeRepository(ctrl),
		}
		jc := NewJobController(m.jr, nil, m.mrr, nil, nil, m.exr, m.atr, nil)
		router := gin.New()
		router.PUT("/jobs/:id", jc.UpdateJob)
		return jc, router, m
	}

	libraryResult := &models.ExperimentalResult{Id: primitive.NewObjectID(), UserId: "owner", AssayTypeId: assayType.Id, ProteinSequence: "MKV", ActualAssayScore: 2}

	t.Run("assay type must exist and belong to the caller", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(nil, fmt.Errorf("missing"))
		status, err := jc.checkAssay(&models.Job{AssayTypeId: assayType.Id}, "owner", false)
		assert.NotNil(tt, err)
		assert.Equal(tt, http.StatusNotFound, status)

		m.atr.EXPECT().FindById(typeId).Return(assayType, nil)
		status, err = jc.checkAssay(&models.Job{AssayTypeId: assayType.Id}, "other", false)
		assert.NotNil(tt, err)
		assert.Equal(tt, http.StatusForbidden, status)
	})

	t.Run("duplicate sequences are rejected", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil)
		job := &models.Job{AssayTypeId: assayType.Id, LabResult: models.LabResult{Total: 2, Sequences: []string{"mkv", "MKV"}, Scores: []float32{1, 2}}}

		status, err := jc.checkAssay(job, "owner", true)
		assert.Equal(tt, http.StatusBadRequest, status)
		assert.ErrorContains(tt, err, "duplicate")
	})

	t.Run("library results must have the job's type", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil)
		wrong := *libraryResult
		wrong.AssayTypeId = primitive.NewObjectID()
		m.exr.EXPECT().GetByIds([]string{wrong.Id.Hex()}).Return([]*models.ExperimentalResult{&wrong}, nil)
		job := &models.Job{AssayTypeId: assayType.Id, LibraryResultIds: []primitive.ObjectID{wrong.Id}}

		status, _ := jc.checkAssay(job, "owner", false)
		assert.Equal(tt, http.StatusBadRequest, status)
	})

	t.Run("library results of another user are rejected", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil)
		foreign := *libraryResult
		foreign.UserId = "other"
		m.exr.EXPECT().GetByIds([]string{foreign.Id.Hex()}).Return([]*models.ExperimentalResult{&foreign}, nil)
		job := &models.Job{AssayTypeId: assayType.Id, LibraryResultIds: []primitive.ObjectID{foreign.Id}}

		status, _ := jc.checkAssay(job, "owner", false)
		assert.Equal(tt, http.StatusForbidden, status)
	})

	t.Run("library rows must match the stored score", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil).Times(2)
		m.exr.EXPECT().GetByIds([]string{libraryResult.Id.Hex()}).Return([]*models.ExperimentalResult{libraryResult}, nil).Times(2)
		job := &models.Job{
			AssayTypeId:      assayType.Id,
			LibraryResultIds: []primitive.ObjectID{libraryResult.Id},
			LabResult:        models.LabResult{Total: 1, Sequences: []string{"mkv"}, Scores: []float32{2}, Sources: []string{models.SourceLibrary}},
		}

		status, err := jc.checkAssay(job, "owner", true)
		assert.Nil(tt, err)
		assert.Equal(tt, http.StatusOK, status)

		job.LabResult.Scores = []float32{9}
		status, _ = jc.checkAssay(job, "owner", true)
		assert.Equal(tt, http.StatusBadRequest, status)
	})

	t.Run("library rows without library_result_ids are rejected", func(tt *testing.T) {
		jc, _, m := setup(tt)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil)
		job := &models.Job{
			AssayTypeId: assayType.Id,
			LabResult:   models.LabResult{Total: 1, Sequences: []string{"mkv"}, Scores: []float32{2}, Sources: []string{models.SourceLibrary}},
		}

		status, _ := jc.checkAssay(job, "owner", true)
		assert.Equal(tt, http.StatusBadRequest, status)
	})

	t.Run("changing the assay type moves the job's results", func(tt *testing.T) {
		_, router, m := setup(tt)
		oldJob := &models.Job{Id: primitive.NewObjectID(), UserId: "owner", Name: "J"}
		jobId := oldJob.Id.Hex()
		m.jr.EXPECT().FindById(jobId).Return(oldJob, nil)
		m.atr.EXPECT().FindById(typeId).Return(assayType, nil).Times(2)
		m.jr.EXPECT().Update(jobId, gomock.Any()).Return(nil)
		m.exr.EXPECT().UpdateAssayTypeByJobId(jobId, assayType.Id).Return(nil)
		missing := &models.ExperimentalResult{MutationResultId: primitive.NewObjectID()}
		m.exr.EXPECT().GetAll(map[string]interface{}{"job_id": jobId}).Return([]*models.ExperimentalResult{missing}, nil)
		m.mrr.EXPECT().FindById(missing.MutationResultId.Hex()).Return(&models.MutationResult{ProteinSequence: "MKV"}, nil)
		m.exr.EXPECT().Upsert(missing.MutationResultId.Hex(), gomock.Any()).DoAndReturn(func(_ string, result *models.ExperimentalResult) error {
			assert.Equal(tt, "MKV", result.ProteinSequence)
			return nil
		})

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/jobs/"+jobId, strings.NewReader(`{"assay_type_id":"`+typeId+`"}`)))
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Contains(tt, w.Body.String(), `"assay_type_name":"Yield"`)
	})
}
