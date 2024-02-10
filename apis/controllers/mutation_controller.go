package controllers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type MutationController struct {
	jobRepository      repositories.JobRepository
	mutationRepository repositories.MutationRepository
	conductor          *conductor.Conductor
}

func NewMutationController(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository, conductor *conductor.Conductor) *MutationController {
	return &MutationController{jobRepository: jobRepository, mutationRepository: mutationRepository, conductor: conductor}
}

// GetAllMutations retrieves all mutations
func (mc *MutationController) GetAllMutations(c *gin.Context) {
	query := map[string]interface{}{}
	if jobID := c.Query("job_id"); jobID != "" {
		query["job_id"] = jobID
	}
	mutations, err := mc.mutationRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutations)
}

// GetMutation retrieves a mutation by ID
func (mc *MutationController) GetMutation(c *gin.Context) {
	id := c.Param("id")
	mutation, err := mc.mutationRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutation)
}

// CreateMutation creates a new mutation
func (mc *MutationController) CreateMutation(c *gin.Context) {
	var mutation models.Mutation
	err := c.BindJSON(&mutation)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	job, err := mc.jobRepository.FindById(mutation.JobId.Hex())
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	if job.State != "COMPLETED" {
		err = fmt.Errorf("error: job is currently running")
		apiutil.ApiResponseErrorBadRequest(c, err, "error: job is currently running")
		return
	}

	err = mc.mutationRepository.Create(&mutation)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	err = mc.conductor.RunMutation(&mutation)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutation)
}

// UpdateMutation updates an existing mutation
func (mc *MutationController) UpdateMutation(c *gin.Context) {
	id := c.Param("id")
	mutation, err := mc.mutationRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	err = c.BindJSON(&mutation)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	err = mc.mutationRepository.Update(id, mutation)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutation)
}

// DeleteMutation deletes a mutation by ID
func (mc *MutationController) DeleteMutation(c *gin.Context) {
	id := c.Param("id")

	err := mc.mutationRepository.Delete(id)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, nil)
}
