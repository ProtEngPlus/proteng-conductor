package conductor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories"
)

type Conductor struct {
	jobRepository      repositories.JobRepository
	mutationRepository repositories.MutationRepository
}

func NewConductor(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository) *Conductor {
	return &Conductor{jobRepository: jobRepository, mutationRepository: mutationRepository}
}

func (con *Conductor) Orchestrate(m string) {
	// Decode the incoming message
	var payload Payload
	if err := json.Unmarshal([]byte(m), &payload); err != nil {
		logger.Errorf("Conductor: Error unmarshal mq payload: %v", err)
		return
	}

	// Update job data
	job := con.updateJobData(payload.Data)
	if job == nil || job.State != "ONGOING" {
		return
	}

	// Orchestrate next task
	err := con.OrchestrateJob(job)
	if err != nil {
		logger.Errorf("Conductor: Error orchestrate job %s: %v", job.Id.Hex(), err)
	}
}

// RunJob runs/retries a job
func (con *Conductor) RunJob(job *models.Job) error {
	if job.State == "ONGOING" {
		return fmt.Errorf("error: job is already ongoing")
	}
	if job.State == "COMPLETED" {
		return fmt.Errorf("error: job is already completed")
	}
	if job.StageId == 2 && job.LabResult.Total == 0 {
		return fmt.Errorf("error: lab result is missing")
	}
	job.State = "ONGOING"
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		logger.Errorf("Conductor: Runjob: Failed to update job %s: %v", job.Id.Hex(), err)
		return err
	}
	return con.OrchestrateJob(job)
}

// RunMutation runs/retries a mutation
func (con *Conductor) RunMutation(mutation *models.Mutation) error {
	if mutation.State == "ONGOING" {
		return fmt.Errorf("error: mutation is currently running")
	}
	if mutation.State == "COMPLETED" {
		return fmt.Errorf("error: mutation is already completed")
	}
	job, err := con.jobRepository.FindById(mutation.JobId.Hex())
	if err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to find job %s: %v", mutation.JobId.Hex(), err)
		return err
	}
	if job.StageId != 3 || job.State != "COMPLETED" {
		return fmt.Errorf("error: job is currently running")
	}
	mutation.State = "ONGOING"
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		logger.Errorf("Conductor: RunMutation: Failed to update mutation job id %s: %v", mutation.JobId.Hex(), err)
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

	err = con.startPipelineComponent(job.StageId, reqBodyMap)

	if err != nil {
		mutation.State = "FAILED"
		if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
			return err
		}
		return err
	}
	return nil
}

// OrchestrateJob sends a request to the next task
func (con *Conductor) OrchestrateJob(job *models.Job) error {
	// Get next task
	reqBodyMap := PipelineRequest{
		JobId:    job.Id.Hex(),
		Input:    job.InputProtein,
		Config:   job.Options[job.Meta[job.StageId]],
		Artifact: job.Artifacts,
		Meta:     job.Meta,
	}
	if job.StageId == 2 {
		reqBodyMap.LabResult = job.LabResult
	}
	if job.StageId == 3 {
		mutation, err := con.getFirstMutation(job)
		if err != nil {
			return err
		}
		reqBodyMap.MutationId = mutation.Id.Hex()
		reqBodyMap.Config = mutation.Options
	}

	err := con.startPipelineComponent(job.StageId, reqBodyMap)

	if err != nil {
		job.State = "FAILED"
		if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
			return err
		}
		return err
	}
	return nil
}

func (con *Conductor) updateJobData(data Data) *models.Job {
	job, err := con.jobRepository.FindById(data.JobID)
	if err != nil {
		logger.Errorf("Conductor: updateJobData: Failed to find job %s : %v", data.JobID, err)
		return nil
	}

	if data.StageID != job.StageId {
		logger.Errorf("Conductor: updateJobData: Error JobID %s: stage mismatch", data.JobID)
		return nil
	}

	if data.StageID == 3 {
		job.State = "COMPLETED"
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s : %v", data.JobID, err)
			return nil
		}
		con.updateMutationData(data)
		return job
	}

	if data.Status == "FAILED" {
		job.State = "FAILED"
		logger.Infof("Conductor: updateJobData: Job %s failed %v", data.JobID, data.Error)
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
			return nil
		}
		return nil
	}

	state, stage_id := getNextStage(*job, data)
	job.State = state

	if state == "FAILED" {
		logger.Infof("Conductor: updateJobData: Error: Invalid stage")
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
			return nil
		}
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

	return job
}

func (con *Conductor) getFirstMutation(job *models.Job) (*models.Mutation, error) {
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
		}
		if err = con.mutationRepository.Create(mutation); err != nil {
			return nil, err
		}
	} else {
		mutation = mutations[0]
	}
	mutation.State = "ONGOING"
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		return nil, err
	}
	return mutation, nil
}

func (con *Conductor) updateMutationData(data Data) {
	mutation, err := con.mutationRepository.FindById(data.MutationID)
	if err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to find mutation %s: %v", data.MutationID, err)
		return
	}

	if data.Status == "FAILED" {
		mutation.State = "FAILED"
		logger.Infof("Conductor: updateMutationData: Mutation %s failed: %v", data.JobID, data.Error)
		if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
			logger.Errorf("Conductor: updateMutationData: Failed to update mutation %s: %v", data.MutationID, err)
			return
		}
		return
	}

	mutation.State = "COMPLETED"
	mutation.Result = data.MutationResult
	if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to update mutation %s: %v", data.MutationID, err)
		return
	}
}

func (c *Conductor) startPipelineComponent(stageId int, request PipelineRequest) error {

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

func getNextStage(job models.Job, data Data) (state string, stage_id int) {
	switch data.StageID {
	case 0:
		return "ONGOING", 1
	case 1:
		if job.LabResult.Total == 0 {
			return "PENDING", 2
		}
		return "ONGOING", 2
	case 2:
		return "ONGOING", 3
	case 3:
		return "COMPLETED", 3
	default:
		return "FAILED", data.StageID
	}
}
