package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"proteng-conductor/models"
	"proteng-conductor/repositories"
)

type JobController struct {
	jobRepository repositories.JobRepository
}

func NewJobController(jobRepository repositories.JobRepository) *JobController {
	return &JobController{jobRepository: jobRepository}
}

// GetAllJobs retrieves all jobs
func (jc *JobController) GetAllJobs(c *gin.Context) {
	jobs, err := jc.jobRepository.GetAll()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

// GetJob retrieves a job by ID
func (jc *JobController) GetJob(c *gin.Context) {
	id := c.Param("id")
	job, err := jc.jobRepository.FindById(id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

// CreateJob creates a new job
func (jc *JobController) CreateJob(c *gin.Context) {
	var job models.Job
	err := c.BindJSON(&job)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = jc.jobRepository.Create(&job)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

// UpdateJob updates an existing job
func (jc *JobController) UpdateJob(c *gin.Context) {
	id := c.Param("id")
	var job models.Job
	err := c.BindJSON(&job)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = jc.jobRepository.Update(id, &job)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

// DeleteJob deletes a job by ID
func (jc *JobController) DeleteJob(c *gin.Context) {
	id := c.Param("id")

	err := jc.jobRepository.Delete(id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
