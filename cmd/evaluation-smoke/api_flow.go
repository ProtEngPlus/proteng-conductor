package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/protengplus/proteng-conductor/apis/routes"
	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/rabbitmq/consumer"
	"github.com/protengplus/proteng-conductor/internal/rabbitmq/publisher"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/protengplus/proteng-conductor/repositories"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type evaluationDetail struct {
	Run     models.EvaluationRun `json:"run"`
	Mutants []struct {
		Results map[string]struct {
			State models.EvaluationState `json:"state"`
		} `json:"results"`
	} `json:"mutants"`
}

func newAPIFlowFixture() (*models.Job, *models.Mutation, []*models.MutationResult) {
	jobID, mutationID := primitive.NewObjectID(), primitive.NewObjectID()
	job := &models.Job{Id: jobID, Name: "BLX01 evaluation API smoke", UserId: "blx01-smoke", StageId: 3,
		State: enum.JobStateCompleted, InputProtein: "ACDEFGHIK", RunType: "manual"}
	mutation := &models.Mutation{Id: mutationID, JobId: jobID, RunId: 1, UserId: job.UserId,
		State: enum.MutationStateCompleted, InputProtein: job.InputProtein, Tool: "mutation"}
	candidates := []*models.MutationResult{
		{Id: primitive.NewObjectID(), JobId: jobID, MutationId: mutationID, UserId: job.UserId, ProteinSequence: "ACDEYGHIK", AssayScore: 0.8},
		{Id: primitive.NewObjectID(), JobId: jobID, MutationId: mutationID, UserId: job.UserId, ProteinSequence: "ACDEFGHIW", AssayScore: 0.6},
	}
	return job, mutation, candidates
}

func callAPI(method string, url string, body interface{}, data interface{}) (int, error) {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		payload = encoded
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if data == nil {
		return response.StatusCode, nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return response.StatusCode, err
	}
	return response.StatusCode, json.Unmarshal(envelope.Data, data)
}

func waitForRunDetail(ctx context.Context, url string, done <-chan error) (*evaluationDetail, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out; check the mock worker uses this broker and waits on evaluation.mock")
		case err := <-done:
			return nil, fmt.Errorf("smoke consumer stopped: %v", err)
		case <-ticker.C:
			var detail evaluationDetail
			status, err := callAPI(http.MethodGet, url, nil, &detail)
			if err != nil {
				return nil, err
			}
			if status != http.StatusOK {
				return nil, fmt.Errorf("evaluation detail: expected 200, got %d", status)
			}
			switch detail.Run.State {
			case models.EvaluationCompleted:
				return &detail, nil
			case models.EvaluationFailed:
				return nil, fmt.Errorf("evaluation failed: %s", detail.Run.Error)
			}
		}
	}
}

func runAPIFlowSmoke(mongoURI, amqpURL string) error {
	if err := validateLocalURI(mongoURI, "mongodb"); err != nil {
		return err
	}
	if err := validateLocalURI(amqpURL, "amqp"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(5*time.Second))
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = client.Disconnect(closeCtx)
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}
	database.Client, config.Config.MongoDb = client, smokeDB
	config.Config.RabbitMqUrl = amqpURL
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	channel, err := conn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	queue, err := channel.QueueDeclare("job_status_event", true, false, false, false, nil)
	if err != nil {
		return err
	}
	if queue.Consumers != 0 {
		return fmt.Errorf("stop other consumers on the isolated smoke broker first")
	}
	if err := channel.ExchangeDeclare("logs_topic", "topic", false, false, false, false, nil); err != nil {
		return err
	}
	runs := repositories.NewEvaluationRunRepository()
	results, err := repositories.NewEvaluationResultRepository()
	if err != nil {
		return err
	}
	job, mutation, candidates := newAPIFlowFixture()
	db := client.Database(smokeDB)
	if _, err := db.Collection("jobs").InsertOne(ctx, job); err != nil {
		return err
	}
	if _, err := db.Collection("mutations").InsertOne(ctx, mutation); err != nil {
		return err
	}
	if _, err := db.Collection("mutation_results").InsertMany(ctx, []interface{}{candidates[0], candidates[1]}); err != nil {
		return err
	}

	jobs := repositories.NewJobRepository()
	mutations := repositories.NewMutationRepository()
	mutationResults := repositories.NewMutationResultRepository()
	queryResults := repositories.NewQueryResultRepository()
	pub := publisher.NewPublisher()
	receiver := conductor.NewConductor(jobs, mutations, queryResults, mutationResults, pub,
		conductor.WithEvaluationRepositories(runs, results))
	done := make(chan error, 1)
	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer func() {
		stopConsumer()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()
	go func() { done <- consumer.NewConsumer(receiver).RunConsumer(consumerCtx, amqpURL, "job_status_event") }()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	routes.JobRoute(router, jobs, mutations, mutationResults, repositories.NewConfigurationRepository(), queryResults,
		repositories.NewExperimentalResultRepository(), runs, results, receiver)
	routes.EvaluationRoute(router, jobs, mutations, mutationResults, runs, results, receiver)
	server := httptest.NewServer(router)
	defer server.Close()
	owner := "?user_id=" + job.UserId
	jobURL := server.URL + "/jobs/" + job.Id.Hex()

	var plugins []string
	status, err := callAPI(http.MethodGet, server.URL+"/evaluation-plugins", nil, &plugins)
	if err != nil {
		return err
	}
	hasMock := false
	for _, plugin := range plugins {
		hasMock = hasMock || plugin == "mock"
	}
	if status != http.StatusOK || !hasMock {
		return fmt.Errorf("plugins: expected 200 with mock, got %d %v", status, plugins)
	}

	var started models.EvaluationRun
	status, err = callAPI(http.MethodPost, jobURL+"/evaluations", map[string]interface{}{"user_id": job.UserId, "plugins": []string{"mock"}}, &started)
	if err != nil {
		return err
	}
	if status != http.StatusOK || !started.Rerun || started.MutationId != mutation.Id {
		return fmt.Errorf("rerun: expected 200 with a rerun of mutation %s, got %d", mutation.Id.Hex(), status)
	}
	fmt.Printf("Waiting for the rerun: job=%s evaluation_run=%s\n", job.Id.Hex(), started.Id.Hex())
	detail, err := waitForRunDetail(ctx, server.URL+"/evaluations/"+started.Id.Hex()+owner, done)
	if err != nil {
		return err
	}
	if len(detail.Mutants) != 2 {
		return fmt.Errorf("evaluation detail: expected 2 mutants, got %d", len(detail.Mutants))
	}
	for i, mutant := range detail.Mutants {
		if mutant.Results["mock"].State != models.EvaluationCompleted {
			return fmt.Errorf("evaluation detail: mutant %d has no completed mock result", i)
		}
	}

	var listed []models.EvaluationRun
	status, err = callAPI(http.MethodGet, jobURL+"/evaluations"+owner, nil, &listed)
	if err != nil {
		return err
	}
	if status != http.StatusOK || len(listed) != 1 || listed[0].Id != started.Id || !listed[0].Rerun {
		return fmt.Errorf("job evaluations: expected the one rerun, got %d with %d runs", status, len(listed))
	}
	status, err = callAPI(http.MethodGet, server.URL+"/evaluations/"+started.Id.Hex()+"?user_id=someone-else", nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusForbidden {
		return fmt.Errorf("another user: expected 403, got %d", status)
	}
	var savedJob models.Job
	if err := db.Collection("jobs").FindOne(ctx, bson.M{"_id": job.Id}).Decode(&savedJob); err != nil {
		return err
	}
	if savedJob.State != enum.JobStateCompleted || savedJob.StageId != 3 {
		return fmt.Errorf("the rerun changed the job: %s at stage %d", savedJob.State, savedJob.StageId)
	}

	status, err = callAPI(http.MethodDelete, jobURL, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("delete job: expected 200, got %d", status)
	}
	leftRuns, err := db.Collection("evaluation_runs").CountDocuments(ctx, bson.M{"job_id": job.Id})
	if err != nil {
		return err
	}
	leftResults, err := db.Collection("evaluation_results").CountDocuments(ctx, bson.M{"evaluation_run_id": started.Id})
	if err != nil {
		return err
	}
	if leftRuns != 0 || leftResults != 0 {
		return fmt.Errorf("delete job left %d evaluation runs and %d results", leftRuns, leftResults)
	}
	fmt.Printf("PASS: plugins listed; rerun COMPLETED with 2 mutants over HTTP; job unchanged; 403 for another user; delete removed runs and results. Database: %s\n", smokeDB)
	return nil
}
