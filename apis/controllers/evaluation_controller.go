package controllers

import (
	"errors"
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type EvaluationController struct {
	jobRepository              repositories.JobRepository
	mutationResultRepository   repositories.MutationResultRepository
	evaluationRunRepository    repositories.EvaluationRunRepository
	evaluationResultRepository repositories.EvaluationResultRepository
}

type evaluationPluginResult struct {
	State  models.EvaluationState `json:"state"`
	Values map[string]interface{} `json:"values"`
	Error  string                 `json:"error"`
}

type evaluationMutant struct {
	MutationResult *models.MutationResult            `json:"mutation_result"`
	Results        map[string]evaluationPluginResult `json:"results"`
}

type evaluationRunDetail struct {
	Run     *models.EvaluationRun `json:"run"`
	Mutants []evaluationMutant    `json:"mutants"`
}

func NewEvaluationController(jobRepository repositories.JobRepository, mutationResultRepository repositories.MutationResultRepository, evaluationRunRepository repositories.EvaluationRunRepository, evaluationResultRepository repositories.EvaluationResultRepository) *EvaluationController {
	return &EvaluationController{jobRepository: jobRepository, mutationResultRepository: mutationResultRepository, evaluationRunRepository: evaluationRunRepository, evaluationResultRepository: evaluationResultRepository}
}

func (ec *EvaluationController) GetEvaluationPlugins(c *gin.Context) {
	plugins := make([]string, 0, len(config.EvaluationPluginSchemas))
	for plugin := range config.EvaluationPluginSchemas {
		plugins = append(plugins, plugin)
	}
	sort.Strings(plugins)
	apiutil.ApiResponseOk(c, plugins)
}

func (ec *EvaluationController) GetJobEvaluations(c *gin.Context) {
	job, err := ec.jobRepository.FindById(c.Param("id"))
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	if job.UserId != c.Query("user_id") {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: job does not belong to user"))
		return
	}

	runs, err := ec.evaluationRunRepository.FindByJob(job.Id.Hex())
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, runs)
}

func (ec *EvaluationController) GetEvaluation(c *gin.Context) {
	run, err := ec.evaluationRunRepository.FindById(c.Param("id"))
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	job, err := ec.jobRepository.FindById(run.JobId.Hex())
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	if job.UserId != c.Query("user_id") {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: evaluation run does not belong to user"))
		return
	}

	results, err := ec.evaluationResultRepository.FindByRun(run.Id.Hex())
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	byMutant := make(map[primitive.ObjectID]map[string]evaluationPluginResult, len(run.MutationResultIds))
	for _, result := range results {
		if byMutant[result.MutationResultId] == nil {
			byMutant[result.MutationResultId] = map[string]evaluationPluginResult{}
		}
		byMutant[result.MutationResultId][result.Plugin] = evaluationPluginResult{State: result.State, Values: result.Values, Error: result.Error}
	}

	detail := evaluationRunDetail{Run: run, Mutants: make([]evaluationMutant, 0, len(run.MutationResultIds))}
	for _, id := range run.MutationResultIds {
		mutationResult, err := ec.mutationResultRepository.FindById(id.Hex())
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
		pluginResults := byMutant[id]
		if pluginResults == nil {
			pluginResults = map[string]evaluationPluginResult{}
		}
		detail.Mutants = append(detail.Mutants, evaluationMutant{MutationResult: mutationResult, Results: pluginResults})
	}

	apiutil.ApiResponseOk(c, detail)
}
