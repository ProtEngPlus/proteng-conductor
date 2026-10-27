package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

type fakeEvaluationRuns struct {
	repositories.EvaluationRunRepository
	runs []*models.EvaluationRun
}

func (f *fakeEvaluationRuns) FindById(id string) (*models.EvaluationRun, error) {
	for _, run := range f.runs {
		if run.Id.Hex() == id {
			return run, nil
		}
	}
	return nil, mongo.ErrNoDocuments
}

func (f *fakeEvaluationRuns) FindByJob(jobId string) ([]*models.EvaluationRun, error) {
	runs := []*models.EvaluationRun{}
	for _, run := range f.runs {
		if run.JobId.Hex() == jobId {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

type fakeEvaluationResults struct {
	repositories.EvaluationResultRepository
	results []*models.EvaluationResult
}

func (f *fakeEvaluationResults) FindByRun(runId string) ([]*models.EvaluationResult, error) {
	results := []*models.EvaluationResult{}
	for _, result := range f.results {
		if result.EvaluationRunId.Hex() == runId {
			results = append(results, result)
		}
	}
	return results, nil
}

func TestEvaluationController(t *testing.T) {
	gin.SetMode(gin.TestMode)

	job := &models.Job{Id: primitive.NewObjectID(), UserId: "owner"}
	kept, deleted := primitive.NewObjectID(), primitive.NewObjectID()
	run := &models.EvaluationRun{
		Id: primitive.NewObjectID(), JobId: job.Id, MutationId: primitive.NewObjectID(),
		MutationResultIds: []primitive.ObjectID{kept, deleted}, Plugins: []string{"mock"}, State: models.EvaluationCompleted,
	}
	results := []*models.EvaluationResult{
		{EvaluationRunId: run.Id, MutationResultId: kept, Plugin: "mock", State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.5}},
		{EvaluationRunId: run.Id, MutationResultId: deleted, Plugin: "mock", State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.6}},
	}

	setup := func(tt *testing.T) (*gin.Engine, *mock_repository.MockJobRepository, *mock_repository.MockMutationResultRepository) {
		ctrl := gomock.NewController(tt)
		jr := mock_repository.NewMockJobRepository(ctrl)
		mrr := mock_repository.NewMockMutationResultRepository(ctrl)
		ec := NewEvaluationController(jr, mrr, &fakeEvaluationRuns{runs: []*models.EvaluationRun{run}}, &fakeEvaluationResults{results: results})
		router := gin.New()
		router.GET("/evaluation-plugins", ec.GetEvaluationPlugins)
		router.GET("/jobs/:id/evaluations", ec.GetJobEvaluations)
		router.GET("/evaluations/:id", ec.GetEvaluation)
		return router, jr, mrr
	}

	get := func(router *gin.Engine, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	t.Run("plugins lists the registered plugins in order", func(tt *testing.T) {
		router, _, _ := setup(tt)

		w := get(router, "/evaluation-plugins")
		assert.Equal(tt, http.StatusOK, w.Code)
		var body struct {
			Data []string `json:"data"`
		}
		assert.NoError(tt, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Contains(tt, body.Data, "mock")
		assert.True(tt, sort.StringsAreSorted(body.Data))
	})

	t.Run("job evaluations returns 404 when the job is missing", func(tt *testing.T) {
		router, jr, _ := setup(tt)
		jr.EXPECT().FindById(job.Id.Hex()).Return(nil, mongo.ErrNoDocuments)

		w := get(router, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=owner")
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("job evaluations returns 403 for another user", func(tt *testing.T) {
		router, jr, _ := setup(tt)
		jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := get(router, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=someone-else")
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("job evaluations lists the runs of the job", func(tt *testing.T) {
		router, jr, _ := setup(tt)
		jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := get(router, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=owner")
		assert.Equal(tt, http.StatusOK, w.Code)
		var body struct {
			Data []models.EvaluationRun `json:"data"`
		}
		assert.NoError(tt, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(tt, body.Data, 1)
		assert.Equal(tt, run.Id, body.Data[0].Id)
	})

	t.Run("evaluation returns 404 when the run is missing", func(tt *testing.T) {
		router, _, _ := setup(tt)

		w := get(router, "/evaluations/"+primitive.NewObjectID().Hex()+"?user_id=owner")
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("evaluation returns 403 for another user", func(tt *testing.T) {
		router, jr, _ := setup(tt)
		jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := get(router, "/evaluations/"+run.Id.Hex()+"?user_id=someone-else")
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("evaluation groups results by mutant and skips deleted mutants", func(tt *testing.T) {
		router, jr, mrr := setup(tt)
		jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		mrr.EXPECT().FindById(kept.Hex()).Return(&models.MutationResult{Id: kept, JobId: job.Id, ProteinSequence: "AAAC"}, nil)
		mrr.EXPECT().FindById(deleted.Hex()).Return(nil, mongo.ErrNoDocuments)

		w := get(router, "/evaluations/"+run.Id.Hex()+"?user_id=owner")
		assert.Equal(tt, http.StatusOK, w.Code)
		var body struct {
			Data struct {
				Run     models.EvaluationRun `json:"run"`
				Mutants []struct {
					MutationResult models.MutationResult             `json:"mutation_result"`
					Results        map[string]evaluationPluginResult `json:"results"`
				} `json:"mutants"`
			} `json:"data"`
		}
		assert.NoError(tt, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(tt, run.Id, body.Data.Run.Id)
		assert.Len(tt, body.Data.Mutants, 1)
		assert.Equal(tt, kept, body.Data.Mutants[0].MutationResult.Id)
		assert.Equal(tt, 0.5, body.Data.Mutants[0].Results["mock"].Values["mock_score"])
	})
}
