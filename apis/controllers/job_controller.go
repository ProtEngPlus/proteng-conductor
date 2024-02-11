package controllers

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xeipuuv/gojsonschema"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type JobController struct {
	jobRepository repositories.JobRepository
	conductor     conductor.Conductor
}

func NewJobController(jobRepository repositories.JobRepository, conductor conductor.Conductor) *JobController {
	return &JobController{jobRepository: jobRepository, conductor: conductor}
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
		query["$text"] = bson.M{"$search": name}
	}
	jobs, err := jc.jobRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, jobs)
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

	apiutil.ApiResponseOk(c, job)
}

func (jc *JobController) CreateDuplicateJob(c *gin.Context) {
	id := c.Param("id")
	stageId, err := strconv.Atoi(c.Param("stage"))
	if err != nil || stageId < 0 || stageId > 2 {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid stage id")
		return
	}
	refJob, err := jc.jobRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	var job models.Job
	err = c.BindJSON(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}
	for i := 0; i <= stageId; i++ {
		job.Artifacts[refJob.Meta[i]] = refJob.Artifacts[refJob.Meta[i]]
	}
	job.RefJobId = refJob.Id
	job.StageId = stageId + 1

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
