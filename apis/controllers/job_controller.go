package controllers

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type JobController struct {
	jobRepository                repositories.JobRepository
	mutationRepository           repositories.MutationRepository
	mutationResultRepository     repositories.MutationResultRepository
	configurationRepository      repositories.ConfigurationRepository
	queryResultRepository        repositories.QueryResultRepository
	experimentalResultRepository repositories.ExperimentalResultRepository
	assayTypeRepository          repositories.AssayTypeRepository
	conductor                    conductor.Conductor
}

func NewJobController(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository, mutationResultRepository repositories.MutationResultRepository, configurationRepository repositories.ConfigurationRepository, queryResultRepository repositories.QueryResultRepository, experimentalResultRepository repositories.ExperimentalResultRepository, assayTypeRepository repositories.AssayTypeRepository, conductor conductor.Conductor) *JobController {
	return &JobController{jobRepository: jobRepository, mutationRepository: mutationRepository, mutationResultRepository: mutationResultRepository, configurationRepository: configurationRepository, queryResultRepository: queryResultRepository, experimentalResultRepository: experimentalResultRepository, assayTypeRepository: assayTypeRepository, conductor: conductor}
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

	names := map[primitive.ObjectID]string{}
	for _, job := range jobs {
		jc.fillAssayTypeName(job, names)
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

	jc.fillAssayTypeName(job, map[primitive.ObjectID]string{})

	apiutil.ApiResponseOk(c, job)
}

// CreateJob creates a new job
func (jc *JobController) CreateJob(c *gin.Context) {
	var requestBody struct {
		Job         models.Job          `json:"job" binding:"required"`
		QueryResult *models.QueryResult `json:"query_result,omitempty"`
	}

	err := c.BindJSON(&requestBody)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	job := requestBody.Job

	if err := job.Validate(false); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid job")
		return
	}

	if err := validateSequences(job.InputProtein, job.LabResult); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid sequence")
		return
	}

	err = validateJobOptions(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid options")
		return
	}

	if status, err := jc.checkAssay(&job, job.UserId, job.LabResult.Total > 0); err != nil {
		respondAssayError(c, status, err)
		return
	}

	err = jc.jobRepository.Create(&job)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if requestBody.QueryResult != nil { // Check if job is created with configuration, and have updated query result, then duplicate the query result
		queryResult := requestBody.QueryResult
		queryResult.JobId = job.Id

		if err = jc.queryResultRepository.Create(queryResult); err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
	} else if job.RefJobId != primitive.NilObjectID && job.StageId > 0 { // Check if job is created with configuration, and no updated query result, then duplicate the query result
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
	ownerId := job.UserId
	oldAssayTypeId := job.AssayTypeId
	oldSequences := append([]string{}, job.LabResult.Sequences...)
	err = c.BindJSON(&job)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	if err := validateSequences(job.InputProtein, job.LabResult); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid sequence")
		return
	}

	labChanged := !slices.Equal(oldSequences, job.LabResult.Sequences)
	if status, err := jc.checkAssay(job, ownerId, labChanged); err != nil {
		respondAssayError(c, status, err)
		return
	}

	err = jc.jobRepository.Update(id, job)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if job.AssayTypeId != oldAssayTypeId {
		if err := jc.moveResultsToAssayType(job); err != nil {
			apiutil.ApiResponseInternalServerError(c, err)
			return
		}
	}

	jc.fillAssayTypeName(job, map[primitive.ObjectID]string{})

	apiutil.ApiResponseOk(c, job)
}

// DeleteJob deletes a job by ID
func (jc *JobController) DeleteJob(c *gin.Context) {
	id := c.Param("id")

	if err := jc.jobRepository.Delete(id); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if err := jc.experimentalResultRepository.DeleteByJobId(id); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if err := jc.mutationResultRepository.DeleteByJobId(id); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if err := jc.mutationRepository.DeleteByJobId(id); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if err := jc.configurationRepository.DeleteByJobId(id); err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	if err := jc.queryResultRepository.DeleteByJobId(id); err != nil {
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

func respondAssayError(c *gin.Context, status int, err error) {
	switch status {
	case http.StatusForbidden:
		apiutil.ApiResponseForbidden(c, err)
	case http.StatusNotFound:
		apiutil.ApiResponseNotFound(c, err)
	case http.StatusInternalServerError:
		apiutil.ApiResponseInternalServerError(c, err)
	default:
		apiutil.ApiResponseErrorBadRequest(c, err, err.Error())
	}
}

func (jc *JobController) fillAssayTypeName(job *models.Job, names map[primitive.ObjectID]string) {
	if job.AssayTypeId == primitive.NilObjectID {
		return
	}
	if name, ok := names[job.AssayTypeId]; ok {
		job.AssayTypeName = name
		return
	}
	if assayType, err := jc.assayTypeRepository.FindById(job.AssayTypeId.Hex()); err == nil {
		names[job.AssayTypeId] = assayType.Name
		job.AssayTypeName = assayType.Name
	}
}

func (jc *JobController) checkAssay(job *models.Job, ownerId string, checkLabResult bool) (int, error) {
	if job.AssayTypeId != primitive.NilObjectID {
		assayType, err := jc.assayTypeRepository.FindById(job.AssayTypeId.Hex())
		if err != nil {
			return http.StatusNotFound, fmt.Errorf("error: assay type not found")
		}
		if assayType.UserId != ownerId {
			return http.StatusForbidden, fmt.Errorf("error: assay type does not belong to user")
		}
	}

	if checkLabResult || len(job.LabResult.Sources) > 0 {
		if err := job.LabResult.Validate(); err != nil {
			return http.StatusBadRequest, err
		}
	}

	libraryRows := map[string]float32{}
	if len(job.LibraryResultIds) > 0 {
		if job.AssayTypeId == primitive.NilObjectID {
			return http.StatusBadRequest, fmt.Errorf("error: library results need an assay type")
		}
		results, err := jc.experimentalResultRepository.GetByIds(objectIdsToHex(job.LibraryResultIds))
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if len(results) != len(job.LibraryResultIds) {
			return http.StatusNotFound, fmt.Errorf("error: library result not found")
		}
		for _, result := range results {
			if result.UserId != ownerId {
				return http.StatusForbidden, fmt.Errorf("error: library result does not belong to user")
			}
			if result.AssayTypeId != job.AssayTypeId {
				return http.StatusBadRequest, fmt.Errorf("error: library result has a different assay type")
			}
			libraryRows[strings.ToUpper(result.ProteinSequence)] = result.ActualAssayScore
		}
	}

	for i, source := range job.LabResult.Sources {
		if source != models.SourceLibrary || i >= len(job.LabResult.Sequences) || i >= len(job.LabResult.Scores) {
			continue
		}
		score, ok := libraryRows[strings.ToUpper(job.LabResult.Sequences[i])]
		if !ok || score != job.LabResult.Scores[i] {
			return http.StatusBadRequest, fmt.Errorf("error: library row %d does not match library_result_ids", i)
		}
	}

	return http.StatusOK, nil
}

func (jc *JobController) moveResultsToAssayType(job *models.Job) error {
	jobId := job.Id.Hex()
	if err := jc.experimentalResultRepository.UpdateAssayTypeByJobId(jobId, job.AssayTypeId); err != nil {
		return err
	}

	results, err := jc.experimentalResultRepository.GetAll(map[string]interface{}{"job_id": jobId})
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.ProteinSequence != "" {
			continue
		}
		mutationResult, err := jc.mutationResultRepository.FindById(result.MutationResultId.Hex())
		if err != nil {
			continue
		}
		result.ProteinSequence = mutationResult.ProteinSequence
		if err := jc.experimentalResultRepository.Upsert(result.MutationResultId.Hex(), result); err != nil {
			return err
		}
	}
	return nil
}

func validateJobOptions(job *models.Job) error {
	for _, service := range job.Meta {
		if _, ok := job.Options[service]; !ok {
			return fmt.Errorf("error: missing options for %s", service)
		}
		if err := validateServiceOptions(service, job.Options[service], job.InputProtein); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigurationOptions(configuration *models.Configuration) error {
	for _, service := range configuration.Meta {
		if _, ok := configuration.Options[service]; !ok {
			return fmt.Errorf("error: missing options for %s", service)
		}
		if err := validateServiceOptions(service, configuration.Options[service], configuration.InputProtein); err != nil {
			return err
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

	if err := validateSequences(configuration.InputProtein, configuration.LabResult); err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid sequence")
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
