package controllers

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type AssayTypeController struct {
	assayTypeRepository          repositories.AssayTypeRepository
	experimentalResultRepository repositories.ExperimentalResultRepository
	mutationResultRepository     repositories.MutationResultRepository
	jobRepository                repositories.JobRepository
}

func NewAssayTypeController(assayTypeRepository repositories.AssayTypeRepository, experimentalResultRepository repositories.ExperimentalResultRepository, mutationResultRepository repositories.MutationResultRepository, jobRepository repositories.JobRepository) *AssayTypeController {
	return &AssayTypeController{assayTypeRepository: assayTypeRepository, experimentalResultRepository: experimentalResultRepository, mutationResultRepository: mutationResultRepository, jobRepository: jobRepository}
}

type assayTypeInput struct {
	UserId      string `json:"user_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func validateAssayTypeInput(input *assayTypeInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		return fmt.Errorf("error: name is required")
	}
	if utf8.RuneCountInString(input.Name) > 50 {
		return fmt.Errorf("error: name is longer than 50 characters")
	}
	if utf8.RuneCountInString(input.Description) > 200 {
		return fmt.Errorf("error: description is longer than 200 characters")
	}
	return nil
}

func (ac *AssayTypeController) nameTaken(userId string, name string, exceptId primitive.ObjectID) (bool, error) {
	assayTypes, err := ac.assayTypeRepository.GetAllByUser(userId)
	if err != nil {
		return false, err
	}
	for _, assayType := range assayTypes {
		if assayType.Id != exceptId && strings.EqualFold(assayType.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func (ac *AssayTypeController) findOwned(c *gin.Context, userId string) (*models.AssayType, bool) {
	if userId == "" {
		apiutil.ApiResponseErrorBadRequest(c, nil, "error: missing user_id")
		return nil, false
	}
	assayType, err := ac.assayTypeRepository.FindById(c.Param("id"))
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
		return nil, false
	}
	if assayType.UserId != userId {
		apiutil.ApiResponseForbidden(c, fmt.Errorf("error: assay type does not belong to user"))
		return nil, false
	}
	return assayType, true
}

func (ac *AssayTypeController) GetAllAssayTypes(c *gin.Context) {
	userId := c.Query("user_id")
	if userId == "" {
		apiutil.ApiResponseErrorBadRequest(c, nil, "error: missing user_id")
		return
	}

	assayTypes, err := ac.assayTypeRepository.GetAllByUser(userId)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	results, err := ac.experimentalResultRepository.GetAll(map[string]interface{}{"user_id": userId})
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	byType := map[primitive.ObjectID][]*models.ExperimentalResult{}
	for _, result := range results {
		byType[result.AssayTypeId] = append(byType[result.AssayTypeId], result)
	}

	summaries := make([]models.AssayTypeSummary, 0, len(assayTypes))
	for _, assayType := range assayTypes {
		kept := dedupeResults(byType[assayType.Id])
		jobs := map[primitive.ObjectID]bool{}
		for _, result := range kept {
			jobs[result.JobId] = true
		}
		summaries = append(summaries, models.AssayTypeSummary{AssayType: *assayType, JobCount: len(jobs), ResultCount: len(kept)})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})

	apiutil.ApiResponseOk(c, summaries)
}

func (ac *AssayTypeController) CreateAssayType(c *gin.Context) {
	var input assayTypeInput
	if err := c.BindJSON(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}
	if input.UserId == "" {
		apiutil.ApiResponseErrorBadRequest(c, nil, "error: missing user_id")
		return
	}
	if err := validateAssayTypeInput(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, err.Error())
		return
	}

	taken, err := ac.nameTaken(input.UserId, input.Name, primitive.NilObjectID)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	if taken {
		apiutil.ApiResponseConflict(c, fmt.Errorf("error: assay type name already exists"))
		return
	}

	assayType := models.AssayType{UserId: input.UserId, Name: input.Name, Description: input.Description}
	if err := ac.assayTypeRepository.Create(&assayType); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, assayType)
}

func (ac *AssayTypeController) GetAssayType(c *gin.Context) {
	assayType, ok := ac.findOwned(c, c.Query("user_id"))
	if !ok {
		return
	}
	apiutil.ApiResponseOk(c, assayType)
}

func (ac *AssayTypeController) UpdateAssayType(c *gin.Context) {
	var input assayTypeInput
	if err := c.BindJSON(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}
	assayType, ok := ac.findOwned(c, input.UserId)
	if !ok {
		return
	}
	if err := validateAssayTypeInput(&input); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, err.Error())
		return
	}

	taken, err := ac.nameTaken(input.UserId, input.Name, assayType.Id)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	if taken {
		apiutil.ApiResponseConflict(c, fmt.Errorf("error: assay type name already exists"))
		return
	}

	assayType.Name = input.Name
	assayType.Description = input.Description
	if err := ac.assayTypeRepository.Update(assayType.Id.Hex(), assayType); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, assayType)
}

func (ac *AssayTypeController) DeleteAssayType(c *gin.Context) {
	assayType, ok := ac.findOwned(c, c.Query("user_id"))
	if !ok {
		return
	}

	jobCount, err := ac.jobRepository.CountByAssayType(assayType.Id.Hex())
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	resultCount, err := ac.experimentalResultRepository.CountByAssayType(assayType.Id.Hex())
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	if jobCount > 0 || resultCount > 0 {
		apiutil.ApiResponseConflict(c, fmt.Errorf("error: assay type is in use"))
		return
	}

	if err := ac.assayTypeRepository.Delete(assayType.Id.Hex()); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	apiutil.ApiResponseOk(c, nil)
}

func (ac *AssayTypeController) typeResults(assayType *models.AssayType) ([]*models.ExperimentalResult, []*models.ExperimentalResult, error) {
	all, err := ac.experimentalResultRepository.GetAll(map[string]interface{}{"user_id": assayType.UserId, "assay_type_id": assayType.Id.Hex()})
	if err != nil {
		return nil, nil, err
	}
	return all, dedupeResults(all), nil
}

func (ac *AssayTypeController) GetLibraryJobs(c *gin.Context) {
	assayType, ok := ac.findOwned(c, c.Query("user_id"))
	if !ok {
		return
	}

	all, kept, err := ac.typeResults(assayType)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	counts := map[primitive.ObjectID]int{}
	for _, result := range kept {
		counts[result.JobId]++
	}

	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	list := models.LibraryJobList{Jobs: []models.LibraryJob{}, DuplicatesCollapsed: len(all) - len(kept)}
	for jobId, count := range counts {
		job, err := ac.jobRepository.FindById(jobId.Hex())
		if err != nil {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(job.Name), query) {
			continue
		}
		list.Jobs = append(list.Jobs, models.LibraryJob{JobId: jobId, JobName: job.Name, CompletedAt: job.CompleteAt, ResultCount: count})
	}
	sort.Slice(list.Jobs, func(i, j int) bool {
		return list.Jobs[i].CompletedAt.After(list.Jobs[j].CompletedAt)
	})

	apiutil.ApiResponseOk(c, list)
}

func (ac *AssayTypeController) GetLibraryResults(c *gin.Context) {
	assayType, ok := ac.findOwned(c, c.Query("user_id"))
	if !ok {
		return
	}
	jobId := c.Query("job_id")
	if _, err := primitive.ObjectIDFromHex(jobId); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid job_id")
		return
	}

	_, kept, err := ac.typeResults(assayType)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	positions := map[primitive.ObjectID][]string{}
	mutationResults, err := ac.mutationResultRepository.GetAll(map[string]interface{}{"job_id": jobId})
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	for _, mutationResult := range mutationResults {
		positions[mutationResult.Id] = mutationResult.MutationPositions
	}

	results := []models.LibraryResult{}
	for _, result := range kept {
		if result.JobId.Hex() != jobId {
			continue
		}
		results = append(results, models.LibraryResult{
			Id:                result.Id,
			ProteinSequence:   result.ProteinSequence,
			MutationPositions: positions[result.MutationResultId],
			ActualAssayScore:  result.ActualAssayScore,
			MeasuredAt:        result.MeasuredAt,
			Note:              result.Note,
		})
	}

	apiutil.ApiResponseOk(c, results)
}

func (ac *AssayTypeController) ExportLibrary(c *gin.Context) {
	assayType, ok := ac.findOwned(c, c.Query("user_id"))
	if !ok {
		return
	}

	_, kept, err := ac.typeResults(assayType)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if ids := splitIds(c.Query("ids")); len(ids) > 0 {
		wanted := map[string]bool{}
		for _, id := range ids {
			wanted[id] = true
		}
		picked := []*models.ExperimentalResult{}
		for _, result := range kept {
			if wanted[result.Id.Hex()] {
				picked = append(picked, result)
			}
		}
		kept = picked
	}

	if len(kept) == 0 {
		apiutil.ApiResponseNotFound(c, fmt.Errorf("error: no results to export"))
		return
	}

	writeResultsCsv(c, exportFileName("Library", assayType.Name), kept)
}
