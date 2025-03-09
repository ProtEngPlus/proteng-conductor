package controllers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type MutationController struct {
	jobRepository            repositories.JobRepository
	mutationRepository       repositories.MutationRepository
	mutationResultRepository repositories.MutationResultRepository
	conductor                conductor.Conductor
}

func NewMutationController(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository, mutationResultRepository repositories.MutationResultRepository, conductor conductor.Conductor) *MutationController {
	return &MutationController{jobRepository: jobRepository, mutationRepository: mutationRepository, mutationResultRepository: mutationResultRepository, conductor: conductor}
}

// GetAllMutations retrieves all mutations
func (mc *MutationController) GetAllMutations(c *gin.Context) {
	query := map[string]interface{}{}
	if jobID := c.Query("job_id"); jobID != "" {
		query["job_id"] = jobID
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
	if isBookmark := c.Query("is_bookmark"); isBookmark != "" {
		query["is_bookmark"] = isBookmark
	}
	if userID := c.Query("user_id"); userID != "" {
		query["user_id"] = userID
	}

	mutations, err := mc.mutationRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	// if the query has is_bookmark = true, get jobName from jobId that is kept in each mutation
	if query["is_bookmark"] == "true" {
		mutationsWithJobName := make([]models.MutationWithJobName, 0, len(mutations))
		for _, mutation := range mutations {
			job, err := mc.jobRepository.FindById(mutation.JobId.Hex())
			if err != nil {
				apiutil.ApiResponseNotFound(c, err)
				return
			}
			mutationWithJobName := models.MutationWithJobName{
				Id:             mutation.Id,
				Name:           mutation.Name,
				JobId:          mutation.JobId,
				JobName:        job.Name,
				JobDescription: job.Description,
				Options:        mutation.Options,
				Tool:           mutation.Tool,
				State:          mutation.State,
				IsBookmark:     mutation.IsBookmark,
				CreatedAt:      mutation.CreatedAt,
				CompleteAt:     mutation.CompleteAt,
			}
			mutationsWithJobName = append(mutationsWithJobName, mutationWithJobName)

		}
		apiutil.ApiResponseOk(c, mutationsWithJobName)
	} else {
		apiutil.ApiResponseOk(c, mutations)
	}
}

// GetMutationHistograms retrieves histograms from all mutations
func (mc *MutationController) GetMutationHistograms(c *gin.Context) {
	query := map[string]interface{}{}
	if jobID := c.Query("job_id"); jobID != "" {
		query["job_id"] = jobID
	}

	mutations, err := mc.mutationRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	histograms := make([]models.MutationHistogram, 0, len(mutations))
	for _, mutation := range mutations {
		if len(mutation.HistogramData) != 0 {
			histograms = append(histograms, models.MutationHistogram{
				Name: mutation.Name,
				Data: mutation.HistogramData,
			})
		}
	}

	apiutil.ApiResponseOk(c, histograms)
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
		apiutil.ApiResponseNotFound(c, err, "error: job is not found")
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

// RunMutation starts/retries a mutation by ID
func (mc *MutationController) RunMutation(c *gin.Context) {
	id := c.Param("id")

	mutation, err := mc.mutationRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	err = mc.conductor.RunMutation(mutation)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutation)
}

// DownloadMutationResults downloads mutation results by ID
func (mc *MutationController) DownloadMutationResults(c *gin.Context) {
	id := c.Param("id")

	mutation, err := mc.mutationRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}

	query := map[string]interface{}{}
	query["mutation_id"] = id
	query["sort"] = "assay_score"
	query["order"] = -1
	if isBookmark := c.Query("is_bookmark"); isBookmark != "" {
		query["is_bookmark"] = isBookmark
	}

	mutationResults, err := mc.mutationResultRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	// Create a CSV file
	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)

	header := []string{"protein_sequence", "mutation_positions", "assay_score"}
	if err := writer.Write(header); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	record := []string{mutation.InputProtein, "-", "-"}
	if err := writer.Write(record); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	for _, result := range mutationResults {
		mutationPositions := strings.Join(result.MutationPositions, ",")
		record = []string{result.ProteinSequence, mutationPositions, fmt.Sprintf("%.19f", result.AssayScore)}
		if err := writer.Write(record); err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="mutation_results_%s.csv"`, id))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Status(200)
	_, err = c.Writer.Write(csvBuffer.Bytes())
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
}

// GetAllMutationResults retrieves all mutationResults
func (mc *MutationController) GetAllMutationResults(c *gin.Context) {
	query := map[string]interface{}{}
	if mutationID := c.Query("mutation_id"); mutationID != "" {
		query["mutation_id"] = mutationID
	}
	if jobID := c.Query("job_id"); jobID != "" {
		query["job_id"] = jobID
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
	if isBookmark := c.Query("is_bookmark"); isBookmark != "" {
		query["is_bookmark"] = isBookmark
	}
	if minValue := c.Query("min_value"); minValue != "" {
		query["min_value"] = minValue
	}
	if maxValue := c.Query("max_value"); maxValue != "" {
		query["max_value"] = maxValue
	}

	mutationResults, err := mc.mutationResultRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, mutationResults)
}

// UpdateMutationResult updates an existing mutationResult
func (mc *MutationController) UpdateMutationResult(c *gin.Context) {
	resultId := c.Param("result_id")
	mutationResult, err := mc.mutationResultRepository.FindById(resultId)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return
	}
	err = c.BindJSON(&mutationResult)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	err = mc.mutationResultRepository.Update(resultId, mutationResult)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	// If the mutation result is bookmarked, update the mutation to be bookmarked
	if mutationResult.IsBookmark {
		mutation, err := mc.mutationRepository.FindById(mutationResult.MutationId.Hex())
		if err != nil {
			apiutil.ApiResponseNotFound(c, err)
			return
		}
		mutation.IsBookmark = true
		err = mc.mutationRepository.Update(mutationResult.MutationId.Hex(), mutation)
		if err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
	}

	apiutil.ApiResponseOk(c, mutationResult)
}
