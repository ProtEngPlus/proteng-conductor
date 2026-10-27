package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

func TestAssayTypeController(t *testing.T) {
	gin.SetMode(gin.TestMode)

	owned := &models.AssayType{Id: primitive.NewObjectID(), UserId: "owner", Name: "Yield"}
	id := owned.Id.Hex()

	type mocks struct {
		atr *mock_repository.MockAssayTypeRepository
		exr *mock_repository.MockExperimentalResultRepository
		mrr *mock_repository.MockMutationResultRepository
		jr  *mock_repository.MockJobRepository
	}

	setup := func(tt *testing.T) (*gin.Engine, mocks) {
		ctrl := gomock.NewController(tt)
		m := mocks{
			atr: mock_repository.NewMockAssayTypeRepository(ctrl),
			exr: mock_repository.NewMockExperimentalResultRepository(ctrl),
			mrr: mock_repository.NewMockMutationResultRepository(ctrl),
			jr:  mock_repository.NewMockJobRepository(ctrl),
		}
		ac := NewAssayTypeController(m.atr, m.exr, m.mrr, m.jr)
		router := gin.New()
		router.GET("/assay-types", ac.GetAllAssayTypes)
		router.POST("/assay-types", ac.CreateAssayType)
		router.GET("/assay-types/:id", ac.GetAssayType)
		router.PUT("/assay-types/:id", ac.UpdateAssayType)
		router.DELETE("/assay-types/:id", ac.DeleteAssayType)
		router.GET("/assay-types/:id/library/jobs", ac.GetLibraryJobs)
		router.GET("/assay-types/:id/library", ac.GetLibraryResults)
		router.GET("/assay-types/:id/library/export", ac.ExportLibrary)
		return router, m
	}

	send := func(router *gin.Engine, method string, path string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	t.Run("create rejects a duplicate name ignoring case", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().GetAllByUser("owner").Return([]*models.AssayType{owned}, nil)

		w := send(router, http.MethodPost, "/assay-types", `{"user_id":"owner","name":"  yield "}`)
		assert.Equal(tt, http.StatusConflict, w.Code)
	})

	t.Run("create rejects an empty name and a long name", func(tt *testing.T) {
		router, _ := setup(tt)

		w := send(router, http.MethodPost, "/assay-types", `{"user_id":"owner","name":"  "}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)

		w = send(router, http.MethodPost, "/assay-types", fmt.Sprintf(`{"user_id":"owner","name":"%s"}`, strings.Repeat("a", 51)))
		assert.Equal(tt, http.StatusBadRequest, w.Code)
	})

	t.Run("create stores a new type", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().GetAllByUser("owner").Return([]*models.AssayType{owned}, nil)
		m.atr.EXPECT().Create(gomock.Any()).DoAndReturn(func(assayType *models.AssayType) error {
			assert.Equal(tt, "Fluorescence", assayType.Name)
			assert.Equal(tt, "owner", assayType.UserId)
			return nil
		})

		w := send(router, http.MethodPost, "/assay-types", `{"user_id":"owner","name":"Fluorescence"}`)
		assert.Equal(tt, http.StatusOK, w.Code)
	})

	t.Run("another user gets 403", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil).Times(2)

		assert.Equal(tt, http.StatusForbidden, send(router, http.MethodGet, "/assay-types/"+id+"?user_id=other", "").Code)
		assert.Equal(tt, http.StatusForbidden, send(router, http.MethodDelete, "/assay-types/"+id+"?user_id=other", "").Code)
	})

	t.Run("update keeps its own name", func(tt *testing.T) {
		router, m := setup(tt)
		current := *owned
		m.atr.EXPECT().FindById(id).Return(&current, nil)
		m.atr.EXPECT().GetAllByUser("owner").Return([]*models.AssayType{owned}, nil)
		m.atr.EXPECT().Update(id, gomock.Any()).Return(nil)

		w := send(router, http.MethodPut, "/assay-types/"+id, `{"user_id":"owner","name":"Yield","description":"new"}`)
		assert.Equal(tt, http.StatusOK, w.Code)
	})

	t.Run("delete returns 409 while a job uses the type", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.jr.EXPECT().CountByAssayType(id).Return(int64(1), nil)
		m.exr.EXPECT().CountByAssayType(id).Return(int64(0), nil)

		assert.Equal(tt, http.StatusConflict, send(router, http.MethodDelete, "/assay-types/"+id+"?user_id=owner", "").Code)
	})

	t.Run("delete removes an unused type", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.jr.EXPECT().CountByAssayType(id).Return(int64(0), nil)
		m.exr.EXPECT().CountByAssayType(id).Return(int64(0), nil)
		m.atr.EXPECT().Delete(id).Return(nil)

		assert.Equal(tt, http.StatusOK, send(router, http.MethodDelete, "/assay-types/"+id+"?user_id=owner", "").Code)
	})

	jobOne, jobTwo := primitive.NewObjectID(), primitive.NewObjectID()
	now := time.Now()
	results := []*models.ExperimentalResult{
		{Id: primitive.NewObjectID(), JobId: jobOne, AssayTypeId: owned.Id, ProteinSequence: "MKV", ActualAssayScore: 1, MeasuredAt: now.Add(-time.Hour)},
		{Id: primitive.NewObjectID(), JobId: jobTwo, AssayTypeId: owned.Id, ProteinSequence: "mkv", ActualAssayScore: 2, MeasuredAt: now},
		{Id: primitive.NewObjectID(), JobId: jobOne, AssayTypeId: owned.Id, ProteinSequence: "AAA", ActualAssayScore: 3, MeasuredAt: now},
	}
	typeQuery := map[string]interface{}{"user_id": "owner", "assay_type_id": id}

	t.Run("list counts deduped results and jobs", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().GetAllByUser("owner").Return([]*models.AssayType{owned}, nil)
		m.exr.EXPECT().GetAll(map[string]interface{}{"user_id": "owner"}).Return(results, nil)

		w := send(router, http.MethodGet, "/assay-types?user_id=owner", "")
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Contains(tt, w.Body.String(), `"job_count":2`)
		assert.Contains(tt, w.Body.String(), `"result_count":2`)
	})

	t.Run("library jobs drop the older duplicate", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.exr.EXPECT().GetAll(typeQuery).Return(results, nil)
		m.jr.EXPECT().FindById(jobOne.Hex()).Return(&models.Job{Name: "Job one", CompleteAt: now.Add(-time.Hour)}, nil)
		m.jr.EXPECT().FindById(jobTwo.Hex()).Return(&models.Job{Name: "Job two", CompleteAt: now}, nil)

		w := send(router, http.MethodGet, "/assay-types/"+id+"/library/jobs?user_id=owner", "")
		assert.Equal(tt, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(tt, body, `"duplicates_collapsed":1`)
		assert.Less(tt, strings.Index(body, "Job two"), strings.Index(body, "Job one"))
		assert.Contains(tt, body, `"job_name":"Job one","completed_at":`)
		assert.Contains(tt, body, `"result_count":1`)
	})

	t.Run("library results of a job", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.exr.EXPECT().GetAll(typeQuery).Return(results, nil)
		m.mrr.EXPECT().GetAll(map[string]interface{}{"job_id": jobOne.Hex()}).Return([]*models.MutationResult{}, nil)

		w := send(router, http.MethodGet, "/assay-types/"+id+"/library?user_id=owner&job_id="+jobOne.Hex(), "")
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Contains(tt, w.Body.String(), "AAA")
		assert.NotContains(tt, w.Body.String(), "MKV")
	})

	t.Run("export gives selected rows as csv", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.exr.EXPECT().GetAll(typeQuery).Return(results, nil)

		w := send(router, http.MethodGet, "/assay-types/"+id+"/library/export?user_id=owner&ids="+results[1].Id.Hex()+","+results[2].Id.Hex(), "")
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Equal(tt, "mkv,2\nAAA,3\n", w.Body.String())
		assert.Contains(tt, w.Header().Get("Content-Disposition"), "Library_Yield.csv")
	})

	t.Run("export of nothing is 404", func(tt *testing.T) {
		router, m := setup(tt)
		m.atr.EXPECT().FindById(id).Return(owned, nil)
		m.exr.EXPECT().GetAll(typeQuery).Return(results, nil)

		w := send(router, http.MethodGet, "/assay-types/"+id+"/library/export?user_id=owner&ids="+primitive.NewObjectID().Hex(), "")
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})
}
