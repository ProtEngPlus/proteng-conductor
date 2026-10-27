package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

type fakeEvaluationRuns struct {
	repositories.EvaluationRunRepository
	runs        []*models.EvaluationRun
	deletedJobs []string
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

func (f *fakeEvaluationRuns) DeleteByJobId(jobId string) error {
	f.deletedJobs = append(f.deletedJobs, jobId)
	return nil
}

type fakeEvaluationResults struct {
	repositories.EvaluationResultRepository
	results     []*models.EvaluationResult
	deletedRuns []primitive.ObjectID
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

func (f *fakeEvaluationResults) DeleteByRunIds(runIds []primitive.ObjectID) error {
	f.deletedRuns = append(f.deletedRuns, runIds...)
	return nil
}

type rerunConductor struct {
	conductor.Conductor
	run      *models.EvaluationRun
	err      error
	mutation *models.Mutation
	plugins  []string
}

func (f *rerunConductor) RerunEvaluation(_ *models.Job, mutation *models.Mutation, plugins []string) (*models.EvaluationRun, error) {
	f.mutation, f.plugins = mutation, plugins
	return f.run, f.err
}

type evaluationControllerTest struct {
	router *gin.Engine
	jr     *mock_repository.MockJobRepository
	mr     *mock_repository.MockMutationRepository
	mrr    *mock_repository.MockMutationResultRepository
	con    *rerunConductor
}

func TestEvaluationController(t *testing.T) {
	gin.SetMode(gin.TestMode)

	job := &models.Job{Id: primitive.NewObjectID(), UserId: "owner", State: enum.JobStateCompleted}
	kept, deleted := primitive.NewObjectID(), primitive.NewObjectID()
	run := &models.EvaluationRun{
		Id: primitive.NewObjectID(), JobId: job.Id, MutationId: primitive.NewObjectID(),
		MutationResultIds: []primitive.ObjectID{kept, deleted}, Plugins: []string{"mock"}, State: models.EvaluationCompleted,
	}
	results := []*models.EvaluationResult{
		{EvaluationRunId: run.Id, MutationResultId: kept, Plugin: "mock", State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.5}},
		{EvaluationRunId: run.Id, MutationResultId: deleted, Plugin: "mock", State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.6}},
	}
	rerunPath := "/jobs/" + job.Id.Hex() + "/evaluations"

	setup := func(tt *testing.T) *evaluationControllerTest {
		ctrl := gomock.NewController(tt)
		d := &evaluationControllerTest{
			jr:  mock_repository.NewMockJobRepository(ctrl),
			mr:  mock_repository.NewMockMutationRepository(ctrl),
			mrr: mock_repository.NewMockMutationResultRepository(ctrl),
			con: &rerunConductor{run: &models.EvaluationRun{Id: primitive.NewObjectID(), Rerun: true}},
		}
		ec := NewEvaluationController(d.jr, d.mr, d.mrr, &fakeEvaluationRuns{runs: []*models.EvaluationRun{run}}, &fakeEvaluationResults{results: results}, d.con)
		d.router = gin.New()
		d.router.GET("/evaluation-plugins", ec.GetEvaluationPlugins)
		d.router.GET("/jobs/:id/evaluations", ec.GetJobEvaluations)
		d.router.GET("/evaluations/:id", ec.GetEvaluation)
		d.router.POST("/jobs/:id/evaluations", ec.RerunEvaluation)
		return d
	}

	send := func(router *gin.Engine, method string, path string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	t.Run("plugins lists the registered plugins in order", func(tt *testing.T) {
		d := setup(tt)

		w := send(d.router, http.MethodGet, "/evaluation-plugins", "")
		assert.Equal(tt, http.StatusOK, w.Code)
		var body struct {
			Data []string `json:"data"`
		}
		assert.NoError(tt, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Contains(tt, body.Data, "mock")
		assert.True(tt, sort.StringsAreSorted(body.Data))
	})

	t.Run("job evaluations returns 404 when the job is missing", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(nil, mongo.ErrNoDocuments)

		w := send(d.router, http.MethodGet, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=owner", "")
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("job evaluations returns 403 for another user", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := send(d.router, http.MethodGet, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=someone-else", "")
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("job evaluations lists the runs of the job", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := send(d.router, http.MethodGet, "/jobs/"+job.Id.Hex()+"/evaluations?user_id=owner", "")
		assert.Equal(tt, http.StatusOK, w.Code)
		var body struct {
			Data []models.EvaluationRun `json:"data"`
		}
		assert.NoError(tt, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(tt, body.Data, 1)
		assert.Equal(tt, run.Id, body.Data[0].Id)
	})

	t.Run("evaluation returns 404 when the run is missing", func(tt *testing.T) {
		d := setup(tt)

		w := send(d.router, http.MethodGet, "/evaluations/"+primitive.NewObjectID().Hex()+"?user_id=owner", "")
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("evaluation returns 403 for another user", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := send(d.router, http.MethodGet, "/evaluations/"+run.Id.Hex()+"?user_id=someone-else", "")
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("evaluation groups results by mutant and skips deleted mutants", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		d.mrr.EXPECT().FindById(kept.Hex()).Return(&models.MutationResult{Id: kept, JobId: job.Id, ProteinSequence: "AAAC"}, nil)
		d.mrr.EXPECT().FindById(deleted.Hex()).Return(nil, mongo.ErrNoDocuments)

		w := send(d.router, http.MethodGet, "/evaluations/"+run.Id.Hex()+"?user_id=owner", "")
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

	t.Run("rerun returns 400 without plugins", func(tt *testing.T) {
		d := setup(tt)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":[]}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
	})

	t.Run("rerun returns 400 for an unknown plugin", func(tt *testing.T) {
		d := setup(tt)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["unknown"]}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
	})

	t.Run("rerun returns 404 when the job is missing", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(nil, mongo.ErrNoDocuments)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["mock"]}`)
		assert.Equal(tt, http.StatusNotFound, w.Code)
	})

	t.Run("rerun returns 403 for another user", func(tt *testing.T) {
		d := setup(tt)
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"someone-else","plugins":["mock"]}`)
		assert.Equal(tt, http.StatusForbidden, w.Code)
	})

	t.Run("rerun returns 400 while the job is not completed", func(tt *testing.T) {
		d := setup(tt)
		ongoing := *job
		ongoing.State = enum.JobStateOnGoing
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(&ongoing, nil)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["mock"]}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
		assert.Nil(tt, d.con.mutation)
	})

	t.Run("rerun evaluates the latest completed mutation run by default", func(tt *testing.T) {
		d := setup(tt)
		failed := &models.Mutation{Id: primitive.NewObjectID(), JobId: job.Id, RunId: 3, State: enum.MutationStateFailed}
		latest := &models.Mutation{Id: primitive.NewObjectID(), JobId: job.Id, RunId: 2, State: enum.MutationStateCompleted}
		first := &models.Mutation{Id: primitive.NewObjectID(), JobId: job.Id, RunId: 1, State: enum.MutationStateCompleted}
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		d.mr.EXPECT().GetAll(map[string]interface{}{"job_id": job.Id.Hex()}).Return([]*models.Mutation{failed, latest, first}, nil)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["mock"]}`)
		assert.Equal(tt, http.StatusOK, w.Code)
		assert.Equal(tt, latest, d.con.mutation)
		assert.Equal(tt, []string{"mock"}, d.con.plugins)
	})

	t.Run("rerun rejects a mutation of another job", func(tt *testing.T) {
		d := setup(tt)
		other := &models.Mutation{Id: primitive.NewObjectID(), JobId: primitive.NewObjectID(), State: enum.MutationStateCompleted}
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		d.mr.EXPECT().FindById(other.Id.Hex()).Return(other, nil)

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["mock"],"mutation_id":"`+other.Id.Hex()+`"}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
		assert.Nil(tt, d.con.mutation)
	})

	t.Run("rerun reports a running evaluation as a bad request", func(tt *testing.T) {
		d := setup(tt)
		completed := &models.Mutation{Id: primitive.NewObjectID(), JobId: job.Id, RunId: 1, State: enum.MutationStateCompleted}
		d.jr.EXPECT().FindById(job.Id.Hex()).Return(job, nil)
		d.mr.EXPECT().GetAll(gomock.Any()).Return([]*models.Mutation{completed}, nil)
		d.con.err = conductor.ErrEvaluationRunning

		w := send(d.router, http.MethodPost, rerunPath, `{"user_id":"owner","plugins":["mock"]}`)
		assert.Equal(tt, http.StatusBadRequest, w.Code)
	})
}
