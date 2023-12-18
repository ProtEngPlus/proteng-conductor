package controllers

import (
	"github.com/gin-gonic/gin"

	"proteng-conductor/models"
	"proteng-conductor/repositories"
	"proteng-conductor/services/conductor"
	"proteng-conductor/utils/apiutil"
)

type JobController struct {
	jobRepository repositories.JobRepository
	conductor     *conductor.Conductor
}

func NewJobController(jobRepository repositories.JobRepository, conductor *conductor.Conductor) *JobController {
	return &JobController{jobRepository: jobRepository, conductor: conductor}
}

// GetAllJobs retrieves all jobs
func (jc *JobController) GetAllJobs(c *gin.Context) {
	jobs, err := jc.jobRepository.GetAll()
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
		apiutil.ApiResponseErrorBadRequest(c, err, "error: job is already ongoing")
		return
	}

	if job.State == "COMPLETED" {
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
