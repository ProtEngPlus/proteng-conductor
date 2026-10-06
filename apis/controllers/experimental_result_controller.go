package controllers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type ExperimentalResultController struct {
	mutationResultRepository     repositories.MutationResultRepository
	experimentalResultRepository repositories.ExperimentalResultRepository
}

func NewExperimentalResultController(mutationResultRepository repositories.MutationResultRepository, experimentalResultRepository repositories.ExperimentalResultRepository) *ExperimentalResultController {
	return &ExperimentalResultController{mutationResultRepository: mutationResultRepository, experimentalResultRepository: experimentalResultRepository}
}

// GetAllExperimentalResults retrieves all experimental results
func (erc *ExperimentalResultController) GetAllExperimentalResults(c *gin.Context) {
	query := map[string]interface{}{}
	for _, key := range []string{"job_id", "mutation_id", "user_id"} {
		if value := c.Query(key); value != "" {
			query[key] = value
		}
	}

	experimentalResults, err := erc.experimentalResultRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, experimentalResults)
}

// UpsertExperimentalResult creates or overwrites the experimental result of a mutation result
func (erc *ExperimentalResultController) UpsertExperimentalResult(c *gin.Context) {
	var input struct {
		UserId           string    `json:"user_id"`
		ActualAssayScore *float32  `json:"actual_assay_score"`
		Note             string    `json:"note"`
		MeasuredAt       time.Time `json:"measured_at"`
	}
	if err := c.BindJSON(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}
	if input.ActualAssayScore == nil {
		apiutil.ApiResponseErrorBadRequest(c, nil, "error: actual_assay_score is required")
		return
	}

	mutationResultId := c.Param("mutationResultId")
	mutationResult, err := erc.mutationResultRepository.FindById(mutationResultId)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	experimentalResult := models.ExperimentalResult{
		UserId:           input.UserId,
		ActualAssayScore: *input.ActualAssayScore,
		Note:             input.Note,
		MeasuredAt:       input.MeasuredAt,
	}

	if experimentalResult.UserId != mutationResult.UserId {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: mutation result does not belong to user"))
		return
	}

	experimentalResult.MutationResultId = mutationResult.Id
	experimentalResult.MutationId = mutationResult.MutationId
	experimentalResult.JobId = mutationResult.JobId
	if experimentalResult.MeasuredAt.IsZero() {
		experimentalResult.MeasuredAt = time.Now()
	}

	if err := erc.experimentalResultRepository.Upsert(mutationResultId, &experimentalResult); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, experimentalResult)
}

// DeleteExperimentalResult clears the experimental result of a mutation result
func (erc *ExperimentalResultController) DeleteExperimentalResult(c *gin.Context) {
	mutationResultId := c.Param("mutationResultId")
	experimentalResult, err := erc.experimentalResultRepository.FindByMutationResultId(mutationResultId)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	if experimentalResult.UserId != c.Query("user_id") {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: experimental result does not belong to user"))
		return
	}

	if err := erc.experimentalResultRepository.DeleteByMutationResultId(mutationResultId); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, nil)
}
