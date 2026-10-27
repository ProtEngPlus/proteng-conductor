package controllers

import (
	"errors"
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type EvaluationController struct {
	jobRepository              repositories.JobRepository
	mutationRepository         repositories.MutationRepository
	mutationResultRepository   repositories.MutationResultRepository
	evaluationRunRepository    repositories.EvaluationRunRepository
	evaluationResultRepository repositories.EvaluationResultRepository
	conductor                  conductor.Conductor
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

func NewEvaluationController(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository, mutationResultRepository repositories.MutationResultRepository, evaluationRunRepository repositories.EvaluationRunRepository, evaluationResultRepository repositories.EvaluationResultRepository, conductor conductor.Conductor) *EvaluationController {
	return &EvaluationController{jobRepository: jobRepository, mutationRepository: mutationRepository, mutationResultRepository: mutationResultRepository, evaluationRunRepository: evaluationRunRepository, evaluationResultRepository: evaluationResultRepository, conductor: conductor}
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

func (ec *EvaluationController) RerunEvaluation(c *gin.Context) {
	var input struct {
		UserId     string   `json:"user_id"`
		Plugins    []string `json:"plugins"`
		MutationId string   `json:"mutation_id"`
	}
	if err := c.BindJSON(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}
	if len(input.Plugins) == 0 {
		apiutil.ApiResponseErrorBadRequest(c, nil, "error: plugins are required")
		return
	}
	if err := validateEvaluationPlugins(input.Plugins); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid plugins")
		return
	}

	job, err := ec.jobRepository.FindById(c.Param("id"))
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	if job.UserId != input.UserId {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: job does not belong to user"))
		return
	}
	if job.State != enum.JobStateCompleted {
		apiutil.ApiResponseErrorBadRequest(c, fmt.Errorf("error: job is not completed"), "error: job is not completed")
		return
	}

	var mutation *models.Mutation
	if input.MutationId != "" {
		mutation, err = ec.mutationRepository.FindById(input.MutationId)
		if err != nil {
			apiutil.ApiResponseNotFound(c, err, "error: mutation is not found")
			return
		}
		if mutation.JobId != job.Id {
			apiutil.ApiResponseErrorBadRequest(c, fmt.Errorf("error: mutation does not belong to job"), "error: mutation does not belong to job")
			return
		}
	} else {
		mutations, err := ec.mutationRepository.GetAll(map[string]interface{}{"job_id": job.Id.Hex()})
		if err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
		for _, candidate := range mutations {
			if candidate.State == enum.MutationStateCompleted {
				mutation = candidate
				break
			}
		}
		if mutation == nil {
			apiutil.ApiResponseErrorBadRequest(c, fmt.Errorf("error: job has no completed mutation run"), "error: job has no completed mutation run")
			return
		}
	}
	if mutation.State != enum.MutationStateCompleted {
		apiutil.ApiResponseErrorBadRequest(c, fmt.Errorf("error: mutation run is not completed"), "error: mutation run is not completed")
		return
	}

	run, err := ec.conductor.RerunEvaluation(job, mutation, input.Plugins)
	if errors.Is(err, conductor.ErrEvaluationRunning) || errors.Is(err, conductor.ErrNoEvaluationCandidates) {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: "+err.Error())
		return
	}
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, run)
}
