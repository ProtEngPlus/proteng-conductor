package conductor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
)

type conductor struct {
	jobRepository      repositories.JobRepository
	mutationRepository repositories.MutationRepository
}

type Conductor interface {
	Orchestrate(messagePayload string)
	OrchestrateJob(job *models.Job) error
	RunJob(job *models.Job) error
	RunMutation(mutation *models.Mutation) error

	updateJobData(data Data) *models.Job
	updateMutationData(data Data)
	getFirstMutation(job *models.Job) (*models.Mutation, error)
	startPipelineComponent(stageId int, request PipelineRequest) error
}

func NewConductor(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository) Conductor {
	return &conductor{jobRepository: jobRepository, mutationRepository: mutationRepository}
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
		mutation.State = enum.MutationStateFailed
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
		job.State = enum.JobStateFailed
		if e := con.jobRepository.Update(job.Id.Hex(), job); e != nil {
			return e
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

	if data.StageID == 3 {
		job.State = enum.JobStateCompleted
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s : %v", data.JobID, err)
			return nil
		}
		con.updateMutationData(data)
		return job
	}

	if data.Status == string(enum.JobStateFailed) {
		job.State = enum.JobStateFailed
		logger.Infof("Conductor: updateJobData: Job %s failed %v", data.JobID, data.Error)
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			logger.Errorf("Conductor: updateJobData: Failed to update job %s: %v", data.JobID, err)
			return nil
		}
		con.jobRepository.AddErrorLog(data.JobID, data.Error)
		return nil
	}

	state, stage_id := getNextStage(*job, data)
	job.State = state

	if state == enum.JobStateFailed {
		logger.Infof("Conductor: updateJobData: Error: Invalid stage")
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

	return job
}

func (con *conductor) getFirstMutation(job *models.Job) (*models.Mutation, error) {
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
	mutation.State = enum.MutationStateOnGoing
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		return nil, err
	}
	return mutation, nil
}

func (con *conductor) updateMutationData(data Data) {
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
		con.jobRepository.AddErrorLog(data.JobID, "Mutation error: "+data.Error)
		return
	}

	mutation.State = enum.MutationStateCompleted
	mutation.Result = data.MutationResult
	if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
		logger.Errorf("Conductor: updateMutationData: Failed to update mutation %s: %v", data.MutationID, err)
		return
	}
}

func (con *conductor) startPipelineComponent(stageId int, request PipelineRequest) error {
	logger.Debugf("Conductor: startPipelineComponent: %+v", request)

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

func getNextStage(job models.Job, data Data) (state enum.JobState, stage_id int) {
	switch data.StageID {
	case 0:
		return enum.JobStateOnGoing, 1
	case 1:
		if job.LabResult.Total == 0 {
			return enum.JobStatePending, 2
		}
		return enum.JobStateOnGoing, 2
	case 2:
		return enum.JobStateOnGoing, 3
	case 3:
		return enum.JobStateCompleted, 3
	default:
		return enum.JobStateFailed, data.StageID
	}
}
