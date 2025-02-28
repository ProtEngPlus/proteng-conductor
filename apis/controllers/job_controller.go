package controllers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/xeipuuv/gojsonschema"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type JobController struct {
	jobRepository            repositories.JobRepository
	mutationRepository       repositories.MutationRepository
	mutationResultRepository repositories.MutationResultRepository
	configurationRepository  repositories.ConfigurationRepository
	queryResultRepository    repositories.QueryResultRepository
	conductor                conductor.Conductor
}

func NewJobController(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository, mutationResultRepository repositories.MutationResultRepository, configurationRepository repositories.ConfigurationRepository, queryResultRepository repositories.QueryResultRepository, conductor conductor.Conductor) *JobController {
	return &JobController{jobRepository: jobRepository, mutationRepository: mutationRepository, mutationResultRepository: mutationResultRepository, configurationRepository: configurationRepository, queryResultRepository: queryResultRepository, conductor: conductor}
}

// GetAllJobs retrieves all jobs
func (jc *JobController) GetAllJobs(c *gin.Context) {
	query := map[string]interface{}{}
	if userID := c.Query("user_id"); userID != "" {
		query["user_id"] = userID
	}
	if states := c.QueryArray("state"); len(states) > 0 {
		query["state"] = states
	}
	if name := c.Query("name"); name != "" {
		query["name"] = name
	}
	if sort := c.Query("sort"); sort != "" {
		query["sort"] = sort
	}
	if order := c.Query("order"); order != "" {
		switch order {
		case "asc":
			query["order"] = 1
		case "desc":
			query["order"] = -1
		}
	}
	if createdAtFrom := c.Query("created_at_from"); createdAtFrom != "" {
		query["created_at_from"] = createdAtFrom
	}
	if createdAtTo := c.Query("created_at_to"); createdAtTo != "" {
		query["created_at_to"] = createdAtTo
	}

	jobs, err := jc.jobRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, jobs)
}

func (jc *JobController) GetJobDashboard(c *gin.Context) {
	query := map[string]interface{}{}
	userID := c.Query("user_id")
	if userID != "" {
		query["user_id"] = userID
	} else {
		err := fmt.Errorf("error: missing user_id")
		apiutil.ApiResponseErrorBadRequest(c, err, "error: missing user_id")
		return
	}

	jobs, err := jc.jobRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	var numberOfJobs models.NumberOfJobs

	for _, job := range jobs {
		switch job.State {
		case "CREATED":
			numberOfJobs.Created++
		case "PENDING":
			numberOfJobs.Pending++
		case "ONGOING":
			numberOfJobs.Ongoing++
		case "COMPLETED":
			numberOfJobs.Completed++
		case "FAILED":
			numberOfJobs.Failed++
		}
	}

	bestAssayScore, err := jc.mutationResultRepository.FindBestResult(userID)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	recentJob, err := jc.jobRepository.FindRecent(userID)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	var jobDashboard models.DashboardResponseData
	jobDashboard.NumberOfJobs = numberOfJobs
	jobDashboard.BestAssayScore = bestAssayScore
	jobDashboard.RecentJob = recentJob

	apiutil.ApiResponseOk(c, jobDashboard)
}

// GetJob retrieves a job by ID
func (jc *JobController) GetJob(c *gin.Context) {
	id := c.Param("id")
	job, err := jc.jobRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	apiutil.ApiResponseOk(c, job)
}

// CreateJob creates a new job
func (jc *JobController) CreateJob(c *gin.Context) {
	var job models.Job
	err := c.BindJSON(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	if err := job.Validate(false); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid job")
		return
	}

	err = validateJobOptions(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid options")
		return
	}

	err = jc.jobRepository.Create(&job)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	// Check if job is created with configuration, then duplicate the query result
	if job.RefJobId != primitive.NilObjectID && job.StageId > 0 {
		query := map[string]interface{}{
			"job_id": job.RefJobId,
		}

		queryResults, err := jc.queryResultRepository.GetAll(query)
		if err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
		queryResult := &models.QueryResult{}
		if len(queryResults) == 0 {
			queryResult = &models.QueryResult{
				JobId:        job.Id,
				InputProtein: job.InputProtein,
			}
		} else {
			queryResult = queryResults[0]
			queryResult.JobId = job.Id
		}
		if err = jc.queryResultRepository.Create(queryResult); err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
	}

	apiutil.ApiResponseOk(c, job)
}

// UpdateJob updates an existing job
func (jc *JobController) UpdateJob(c *gin.Context) {
	id := c.Param("id")
	job, err := jc.jobRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	err = c.BindJSON(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	err = jc.jobRepository.Update(id, job)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, job)
}

// DeleteJob deletes a job by ID
func (jc *JobController) DeleteJob(c *gin.Context) {
	id := c.Param("id")

	err := jc.jobRepository.Delete(id)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, nil)
}

// RunJob starts/retries a job by ID
func (jc *JobController) RunJob(c *gin.Context) {
	id := c.Param("id")

	job, err := jc.jobRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	if job.State == "ONGOING" {
		err = fmt.Errorf("error: job is already ongoing")
		apiutil.ApiResponseErrorBadRequest(c, err, "error: job is already ongoing")
		return
	}

	if job.State == "COMPLETED" {
		err = fmt.Errorf("error: job is already completed")
		apiutil.ApiResponseErrorBadRequest(c, err, "error: job is already completed")
		return
	}

	err = jc.conductor.RunJob(job)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, nil, "job running")
}

func validateJobOptions(job *models.Job) error {
	for _, service := range job.Meta {
		if _, ok := job.Options[service]; !ok {
			return fmt.Errorf("error: missing options for %s", service)
		}
		option := job.Options[service]
		schemaLoader := gojsonschema.NewStringLoader(config.GetSchema(service))
		optionLoader := gojsonschema.NewGoLoader(option)
		result, err := gojsonschema.Validate(schemaLoader, optionLoader)
		if err != nil {
			return err
		}
		if !result.Valid() {
			return fmt.Errorf(result.Errors()[0].String())
		}
	}
	return nil
}

func validateConfigurationOptions(configuration *models.Configuration) error {
	for _, service := range configuration.Meta {
		if _, ok := configuration.Options[service]; !ok {
			return fmt.Errorf("error: missing options for %s", service)
		}
		option := configuration.Options[service]
		schemaLoader := gojsonschema.NewStringLoader(config.GetSchema(service))
		optionLoader := gojsonschema.NewGoLoader(option)
		result, err := gojsonschema.Validate(schemaLoader, optionLoader)
		if err != nil {
			return err
		}
		if !result.Valid() {
			return fmt.Errorf(result.Errors()[0].String())
		}
	}
	return nil
}

func (jc *JobController) CreateConfigurations(c *gin.Context) {
	var configuration models.Configuration
	err := c.BindJSON(&configuration)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	err = validateConfigurationOptions(&configuration)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid options")
		return
	}

	err = jc.configurationRepository.Create(&configuration)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, configuration)
}

func (jc *JobController) GetAllConfigurations(c *gin.Context) {
	query := map[string]interface{}{}
	// check if user_id is provided
	userID := c.Query("user_id")
	if userID != "" {
		query["user_id"] = userID
	} else {
		err := fmt.Errorf("error: missing user_id")
		apiutil.ApiResponseErrorBadRequest(c, err, "error: missing user_id")
		return
	}
	// only return configurations with state "COMPLETED"
	query["state"] = []string{"COMPLETED"}

	configurations, err := jc.configurationRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, configurations)
}
