package conductor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"
	rmqPublisher "github.com/protengplus/proteng-conductor/internal/rabbitmq/publisher"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
)

type conductor struct {
	jobRepository            repositories.JobRepository
	mutationRepository       repositories.MutationRepository
	queryResultRepository    repositories.QueryResultRepository
	mutationResultRepository repositories.MutationResultRepository
	publisher                rmqPublisher.Publisher
}

type Conductor interface {
	Orchestrate(messagePayload string)
	OrchestrateJob(job *models.Job) error
	RunJob(job *models.Job) error
	RunMutation(mutation *models.Mutation) error
	sendJobStatusNotificationEmail(userID string, jobName string, jobState enum.JobState, stageID int, stageName string)
}

func NewConductor(
	jobRepository repositories.JobRepository,
	mutationRepository repositories.MutationRepository,
	queryResultRepository repositories.QueryResultRepository,
	mutationResultRepository repositories.MutationResultRepository,
	publisher rmqPublisher.Publisher,
) *conductor {
	return &conductor{
		jobRepository:            jobRepository,
		mutationRepository:       mutationRepository,
		queryResultRepository:    queryResultRepository,
		mutationResultRepository: mutationResultRepository,
		publisher:                publisher,
	}
}

func (con *conductor) Orchestrate(m string) {
	// Decode the incoming message
	var payload Payload
	if err := json.Unmarshal([]byte(m), &payload); err != nil {
		logger.Errorf("Conductor: Error unmarshal mq payload: %v", err)
		return
	}

	// Update job data
	job := con.updateJobData(payload.Data)
	if job == nil || job.State != enum.JobStateOnGoing {
		return
	}

	// Orchestrate next task
	err := con.OrchestrateJob(job)
	if err != nil {
		logger.Errorf("Conductor: Error orchestrate job %s: %v", job.Id.Hex(), err)
	}
}

// RunJob runs/retries a job
func (con *conductor) RunJob(job *models.Job) error {
	if job.State == enum.JobStateOnGoing {
		return fmt.Errorf("error: job is already ongoing")
	}
	if job.State == enum.JobStateCompleted {
		return fmt.Errorf("error: job is already completed")
	}
	if job.StageId == 2 && job.LabResult.Total == 0 {
		return fmt.Errorf("error: lab result is missing")
	}
	job.State = enum.JobStateOnGoing
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		logger.Errorf("Conductor: Runjob: Failed to update job %s: %v", job.Id.Hex(), err)
		return err
	}
	return con.OrchestrateJob(job)
}

// RunMutation runs/retries a mutation
func (con *conductor) RunMutation(mutation *models.Mutation) error {
	if mutation.State == enum.MutationStateOnGoing {
		return fmt.Errorf("error: mutation is currently running")
	}
	if mutation.State == enum.MutationStateCompleted {
		return fmt.Errorf("error: mutation is already completed")
	}
	job, err := con.jobRepository.FindById(mutation.JobId.Hex())
	if err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to find job %s: %v", mutation.JobId.Hex(), err)
		return err
	}
	if job.StageId != 3 || job.State != enum.JobStateCompleted {
		return fmt.Errorf("error: job is currently running")
	}
	mutation.State = enum.MutationStateOnGoing
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to update mutation job id %s: %v", mutation.JobId.Hex(), err)
		return err
	}
	job.State = enum.JobStateOnGoing
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to update job %s: %v", job.Id.Hex(), err)
		return err
	}
	reqBodyMap := PipelineRequest{
		JobId:      mutation.JobId.Hex(),
		MutationId: mutation.Id.Hex(),
		Input:      mutation.InputProtein,
		Config:     mutation.Options,
		Artifact:   job.Artifacts,
		Meta:       job.Meta,
	}

	err = con.sendJobToPipelineComponent(job.StageId, mutation.Tool, reqBodyMap)

	if err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to send job to pipeline component: %v", err)
		mutation.State = enum.MutationStateFailed
		if e := con.jobRepository.Update(job.Id.Hex(), job); e != nil {
			return e
		}
		job.State = enum.JobStateFailed
		if e := con.jobRepository.Update(job.Id.Hex(), job); e != nil {
			return e
		}
		con.jobRepository.AddErrorLog(job.Id.Hex(), err.Error())
		return err
	}
	return nil
}

// OrchestrateJob sends a request to the next task
func (con *conductor) OrchestrateJob(job *models.Job) error {
	// Get next task
	reqBodyMap := PipelineRequest{
		JobId:    job.Id.Hex(),
		Input:    job.InputProtein,
		Config:   job.Options[job.Meta[job.StageId]],
		Artifact: job.Artifacts,
		Meta:     job.Meta,
	}
	if job.StageId == 0 {
		query_result, err := con.getCurrentQueryResult(job)
		if err != nil {
			return err
		}
		if query_result.State == enum.QueryResultStateCompleted {
			if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
				return err
			}
			return nil
		}
		query_result.State = enum.QueryResultStateOnGoing
		if err := con.queryResultRepository.Update(query_result.Id.Hex(), query_result); err != nil {
			return err
		}
		reqBodyMap.QueryResultId = query_result.Id.Hex()
		job.RunTime["query"] = models.StageRunTime{ StartTime: time.Now(), EndTime: time.Time{} }
	}
	if job.StageId == 1 {
		query_result, err := con.getCurrentQueryResult(job)
		if err != nil {
			return err
		}
		reqBodyMap.QueryResult = query_result.Result
		job.RunTime["evotune"] = models.StageRunTime{ StartTime: time.Now(), EndTime: time.Time{} }
	}
	if job.StageId == 2 {
		reqBodyMap.LabResult = job.LabResult
		job.RunTime["fittop"] = models.StageRunTime{ StartTime: time.Now(), EndTime: time.Time{} }
	}
	if job.StageId == 3 {
		mutation, err := con.getCurrentMutation(job)
		if err != nil {
			return err
		}
		if mutation.State == enum.MutationStateCompleted {
			job.State = enum.JobStateCompleted
			if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
				return err
			}
			return nil
		}
		mutation.State = enum.MutationStateOnGoing
		if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
			return err
		}
		reqBodyMap.MutationId = mutation.Id.Hex()
		reqBodyMap.Config = mutation.Options
		job.RunTime["mutation"] = models.StageRunTime{ StartTime: time.Now(), EndTime: time.Time{} }
	}

	job.UpdatedAt = time.Now();
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		return err
	}

	err := con.sendJobToPipelineComponent(job.StageId, job.Meta[job.StageId], reqBodyMap)

	if err != nil {
		logger.Errorf("Conductor: OrchestrateJob: Failed to send job to pipeline component: %v", err)
		job.State = enum.JobStateFailed
		if e := con.jobRepository.Update(job.Id.Hex(), job); e != nil {
			return e
		}
		if job.StageId == 3 {
			mutation, err := con.mutationRepository.FindById(reqBodyMap.MutationId)
			if err != nil {
				return err
			}
			mutation.State = enum.MutationStateFailed
			if e := con.mutationRepository.Update(mutation.Id.Hex(), mutation); e != nil {
				return e
			}
		}
		con.jobRepository.AddErrorLog(job.Id.Hex(), err.Error())
		return err
	}
	return nil
}

func (con *conductor) updateJobData(data Data) *models.Job {
	job, err := con.jobRepository.FindById(data.JobID)
	if err != nil {
		logger.Errorf("Conductor: updateJobData: Failed to find job %s : %v", data.JobID, err)
		return nil
	}

	if data.StageID != job.StageId {
		logger.Errorf("Conductor: updateJobData: Error JobID %s: stage mismatch", data.JobID)
		con.jobRepository.AddErrorLog(data.JobID, "error: stage mismatch")
		return nil
	}

	if data.StageID == 0 {
		con.updateQueryResultData(data)
		job.RunTime["query"] = models.StageRunTime{ StartTime: job.RunTime["query"].StartTime, EndTime: time.Now() }
	}

	if data.StageID == 1 {
		job.RunTime["evotune"] = models.StageRunTime{ StartTime: job.RunTime["evotune"].StartTime, EndTime: time.Now() }
	}

	if data.StageID == 2 {
		job.RunTime["fittop"] = models.StageRunTime{ StartTime: job.RunTime["fittop"].StartTime, EndTime: time.Now() }
	}

	if data.StageID == 3 {
		con.updateMutationData(data)
		job.RunTime["mutation"] = models.StageRunTime{ StartTime: job.RunTime["mutation"].StartTime, EndTime: time.Now() }
		// Send email notification considering notification settings
		if job.IsNotificationOn {
			con.sendJobStatusNotificationEmail(job.UserId, job.Name, enum.JobStateCompleted, job.StageId, job.Meta[job.StageId])
		}
		return nil
	}

	if data.Status == string(enum.JobStateFailed) {
		job.State = enum.JobStateFailed
		logger.Infof("Conductor: updateJobData: Job %s failed %v", data.JobID, data.Error)
		// Send email notification considering notification settings
		if job.IsNotificationOn {
			con.sendJobStatusNotificationEmail(job.UserId, job.Name, enum.JobStateFailed, job.StageId, job.Meta[job.StageId])
		}
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
			return nil
		}
		con.jobRepository.AddErrorLog(data.JobID, data.Error)
		return nil
	}

	state, stage_id := getNextStage(*job, data)
	job.State = state

	if state == enum.JobStateFailed && (stage_id != 2 || (stage_id == 2 && job.LabResult.Total != 0)) {
		logger.Infof("Conductor: updateJobData: Error: Invalid stage")
		// Send email notification considering notification settings
		if job.IsNotificationOn {
			con.sendJobStatusNotificationEmail(job.UserId, job.Name, enum.JobStateFailed, job.StageId, job.Meta[job.StageId])
		}
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
			return nil
		}
		con.jobRepository.AddErrorLog(data.JobID, "error: invalid stage")
		return nil
	}

	if job.Artifacts == nil {
		job.Artifacts = make(map[string]models.Artifact)
	}
	if job.StageId < 3 {
		job.Artifacts[job.Meta[job.StageId]] = data.Artifact
	}
	job.StageId = stage_id
	if err := con.jobRepository.Update(data.JobID, job); err != nil {
		logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
		return nil
	}

	// Send email notification considering notification settings
	if job.IsNotificationOn && (job.RunType != "auto" || job.State == enum.JobStateFailed) {
		con.sendJobStatusNotificationEmail(job.UserId, job.Name, job.State, job.StageId, job.Meta[job.StageId])
	}

	return job
}

func (con *conductor) getCurrentMutation(job *models.Job) (*models.Mutation, error) {
	query := map[string]interface{}{
		"job_id": job.Id.Hex(),
	}
	mutations, err := con.mutationRepository.GetAll(query)
	if err != nil {
		return nil, err
	}
	mutation := &models.Mutation{}
	if len(mutations) == 0 {
		mutation = &models.Mutation{
			JobId:        job.Id,
			InputProtein: job.InputProtein,
			Options:      job.Options[job.Meta[3]].(map[string]interface{}),
			Tool:         job.Meta[3],
			UserId:       job.UserId,
		}
		if err = con.mutationRepository.Create(mutation); err != nil {
			return nil, err
		}
	} else {
		mutation = mutations[0]
	}
	return mutation, nil
}

func (con *conductor) getCurrentQueryResult(job *models.Job) (*models.QueryResult, error) {
	query := map[string]interface{}{
		"job_id": job.Id,
	}

	queryResults, err := con.queryResultRepository.GetAll(query)
	if err != nil {
		return nil, err
	}
	queryResult := &models.QueryResult{}
	if len(queryResults) == 0 {
		queryResult = &models.QueryResult{
			JobId:        job.Id,
			InputProtein: job.InputProtein,
		}
		if err = con.queryResultRepository.Create(queryResult); err != nil {
			return nil, err
		}
	} else {
		queryResult = queryResults[0]
	}
	return queryResult, nil
}

func (con *conductor) updateQueryResultData(data Data) {
	job, err := con.jobRepository.FindById(data.JobID)
	if err != nil {
		logger.Errorf("Conductor: updateQueryResultData: Failed to find job %s: %v", data.JobID, err)
		return
	}
	query_result, err := con.queryResultRepository.FindById(data.QueryResultId)
	if err != nil {
		logger.Errorf("Conductor: updateQueryResultData: Failed to find query result %s: %v", data.QueryResultId, err)
		return
	}

	if data.Status == string(enum.JobStateFailed) {
		query_result.State = enum.QueryResultStateFailed
		logger.Infof("Conductor: updateQueryResultData: Query Result %s failed: %v", data.JobID, data.Error)
		if err := con.queryResultRepository.Update(data.QueryResultId, query_result); err != nil {
			logger.Errorf("Conductor: updateQueryResultData: Failed to update query result %s: %v", data.QueryResultId, err)
			return
		}
		job.State = enum.JobStateFailed
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateQueryResultData: Failed to update job %s: %v", data.JobID, err)
			return
		}
		con.jobRepository.AddErrorLog(data.JobID, "Query Result error: "+data.Error)
		return
	}

	query_result.State = enum.QueryResultStateCompleted
	query_result.Result = data.QueryResult
	if err := con.queryResultRepository.Update(data.QueryResultId, query_result); err != nil {
		logger.Errorf("Conductor: updateQueryResultData: Failed to update mutation %s: %v", data.MutationID, err)
		return
	}
}

func (con *conductor) updateMutationData(data Data) {
	job, err := con.jobRepository.FindById(data.JobID)
	if err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to find job %s: %v", data.JobID, err)
		return
	}
	mutation, err := con.mutationRepository.FindById(data.MutationID)
	if err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to find mutation %s: %v", data.MutationID, err)
		return
	}

	if data.Status == string(enum.JobStateFailed) {
		mutation.State = enum.MutationStateFailed
		logger.Infof("Conductor: updateMutationData: Mutation %s failed: %v", data.JobID, data.Error)
		if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
			logger.Errorf("Conductor: updateMutationData: Failed to update mutation %s: %v", data.MutationID, err)
			return
		}
		job.State = enum.JobStateFailed
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateMutationData: Failed to update job %s: %v", data.JobID, err)
			return
		}
		con.jobRepository.AddErrorLog(data.JobID, "Mutation error: "+data.Error)
		return
	}

	mutation.State = enum.MutationStateCompleted
	mutation.CompleteAt = time.Now()

	histogramData := [40]int{}
	for protein_sequence, assay_score := range data.MutationResult {
		newMutationResult := &models.MutationResult{
			MutationId:        mutation.Id,
			JobId:             job.Id,
			UserId:            job.UserId,
			ProteinSequence:   protein_sequence,
			MutationPositions: findMutationPositions(protein_sequence, mutation.InputProtein),
			AssayScore:        assay_score,
			IsBookmark:        false,
		}
		if err := con.mutationResultRepository.Create(newMutationResult); err != nil {
			logger.Errorf("Conductor: updateMutationData: Failed to create new mutation result for mutation %s: %v", data.MutationID, err)
			return
		}
		// Update histogram data
		bin := int(assay_score/0.1) + 20
		if bin < 0 {
			bin = 0
		}
		if bin >= len(histogramData) {
			bin = len(histogramData) - 1
		}
		histogramData[bin]++
	}

	mutation.HistogramData = histogramData[:]

	if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to update mutation %s: %v", data.MutationID, err)
		return
	}

	job.State = enum.JobStateCompleted
	if mutation.RunId == 1 {
		job.CompleteAt = time.Now()
	}
	if err := con.jobRepository.Update(data.JobID, job); err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to update job %s : %v", data.JobID, err)
		return
	}
}

/*
Deprecated, as we are now using RabbitMQ to trigger the pipeline components.

use sendJobToPipelineComponent instead
*/
func (con *conductor) startPipelineComponent(stageId int, request PipelineRequest) error {

	reqBody, err := json.Marshal(request)

	if err != nil {
		return err
	}

	var url string
	switch stageId {
	case 0:
		url = config.Config.SequencerUrl
	case 1:
		url = config.Config.EvotuneUrl
	case 2:
		url = config.Config.FittopUrl
	case 3:
		url = config.Config.MutationUrl
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("error: %s", resp.Status)
	}

	return nil
}

func (con *conductor) sendJobToPipelineComponent(stageId int, tool string, request PipelineRequest) error {
	ctx := context.Background()

	reqBody, err := json.Marshal(request)

	if err != nil {
		return err
	}

	var jobStage string
	switch stageId {
	case 0:
		jobStage = "query"
	case 1:
		jobStage = "evotune"
	case 2:
		jobStage = "fittop"
	case 3:
		jobStage = "mutation"
	}

	routingKey := strings.Join([]string{jobStage, tool}, ".")

	err = con.publisher.PublishWithTopic(ctx, routingKey, reqBody)

	if err != nil {
		return err
	}

	logger.Infof("Conductor: job sent to exchange with topic %s", routingKey)

	return nil
}

func getNextStage(job models.Job, data Data) (state enum.JobState, stage_id int) {
	switch data.StageID {
	case 0:
		if job.RunType == "auto" {
			return enum.JobStateOnGoing, 1
		}
		return enum.JobStatePending, 1
	case 1:
		if job.LabResult.Total == 0 {
			return enum.JobStateFailed, 2
		}
		if job.RunType == "auto" {
			return enum.JobStateOnGoing, 2
		}
		return enum.JobStatePending, 2
	case 2:
		if job.RunType == "auto" {
			return enum.JobStateOnGoing, 3
		}
		return enum.JobStatePending, 3
	case 3:
		return enum.JobStateCompleted, 3
	default:
		return enum.JobStateFailed, data.StageID
	}
}

func (con *conductor) sendJobStatusNotificationEmail(userID string, jobName string, jobState enum.JobState, stageID int, stageName string) {
	ctx := context.Background()

	message := map[string]string{
		"user_id":    userID,
		"job_name":   jobName,
		"job_state":  string(jobState),
		"stage_id":   fmt.Sprintf("%d", stageID),
		"stage_name": stageName,
	}
	reqBody, err := json.Marshal(message)

	if err != nil {
		logger.Errorf("Conductor: Error marshal email notification message: %v", err)
		return
	}

	err = con.publisher.PublishDefaultExchange(ctx, "job_status_email_notification", reqBody)
	if err != nil {
		logger.Errorf("Conductor: Error sending email notification message: %v", err)
		return
	}
	logger.Infof("Conductor: Job status notification sent")
}

func findMutationPositions(proteinSequence string, inputProtein string) []string {
	mutationPositions := []string{}
	for i := 0; i < len(proteinSequence) && i < len(inputProtein); i++ {
		if proteinSequence[i] != inputProtein[i] {
			mutationPositions = append(mutationPositions, fmt.Sprintf("%c%d%c", inputProtein[i], i+1, proteinSequence[i]))
		}
	}
	return mutationPositions
}
