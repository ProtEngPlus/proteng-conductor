package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

// newMutationFlowFixture is an auto job waiting at stage 3 for its mutation,
// with the mock evaluation plugin selected.
func newMutationFlowFixture() (*models.Job, *models.Mutation) {
	jobID, mutationID := primitive.NewObjectID(), primitive.NewObjectID()
	job := &models.Job{Id: jobID, Name: "BLX01 mutation flow smoke", UserId: "blx01-smoke", StageId: 3,
		State: enum.JobStateOnGoing, InputProtein: "ACDEFGHIK", RunType: "auto",
		EvaluationPlugins: []string{"mock"}, Meta: []string{"blast", "unirep", "ridgecv", "mutation"},
		Options: map[string]interface{}{"mutation": map[string]interface{}{}},
		RunTime: map[string]models.StageRunTime{"mutation": {StartTime: time.Now()}}}
	mutation := &models.Mutation{Id: mutationID, JobId: jobID, RunId: 1, UserId: job.UserId,
		State: enum.MutationStateOnGoing, InputProtein: job.InputProtein, Tool: "mutation"}
	return job, mutation
}

// mutationFinishedMessage is what the mutation worker publishes to job_status_event.
func mutationFinishedMessage(job *models.Job, mutation *models.Mutation) ([]byte, error) {
	return json.Marshal(conductor.Payload{
		ServiceName: "mutation",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Data: conductor.Data{
			JobID: job.Id.Hex(), MutationID: mutation.Id.Hex(), StageID: 3, Status: "COMPLETED",
			MutationResult: map[string]float32{"ACDEYGHIK": 0.8, "ACDEFGHIW": 0.6},
		},
	})
}

func runMutationFlowSmoke(mongoURI, amqpURL string) error {
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
	job, mutation := newMutationFlowFixture()
	db := client.Database(smokeDB)
	if _, err := db.Collection("jobs").InsertOne(ctx, job); err != nil {
		return err
	}
	if _, err := db.Collection("mutations").InsertOne(ctx, mutation); err != nil {
		return err
	}
	pub := publisher.NewPublisher()
	receiver := conductor.NewConductor(repositories.NewJobRepository(), repositories.NewMutationRepository(),
		repositories.NewQueryResultRepository(), repositories.NewMutationResultRepository(), pub,
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

	body, err := mutationFinishedMessage(job, mutation)
	if err != nil {
		return err
	}
	if err := pub.PublishDefaultExchange(ctx, "job_status_event", body); err != nil {
		return err
	}
	fmt.Printf("Waiting for mutation -> evaluation -> completed: job=%s\n", job.Id.Hex())
	if err := waitForJobState(ctx, db, job.Id, enum.JobStateCompleted, done); err != nil {
		return err
	}
	if err := checkMutationFlow(ctx, db, job, mutation); err != nil {
		return err
	}

	// A redelivered mutation message must not store results or start a run again.
	if err := pub.PublishDefaultExchange(ctx, "job_status_event", body); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	if err := checkMutationFlow(ctx, db, job, mutation); err != nil {
		return fmt.Errorf("after redelivery: %w", err)
	}
	fmt.Printf("PASS: mutation stored 2 results; 1 EvaluationRun COMPLETED with 2 results; job COMPLETED at stage 3; redelivery changed nothing. Database: %s\n", smokeDB)
	return nil
}

func waitForJobState(ctx context.Context, db *mongo.Database, jobID primitive.ObjectID, want enum.JobState, done <-chan error) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out; check the mock worker uses this broker and waits on evaluation.mock")
		case err := <-done:
			return fmt.Errorf("smoke consumer stopped: %v", err)
		case <-ticker.C:
			var job models.Job
			if err := db.Collection("jobs").FindOne(ctx, bson.M{"_id": jobID}).Decode(&job); err != nil {
				return err
			}
			if job.State == want {
				return nil
			}
		}
	}
}

func checkMutationFlow(ctx context.Context, db *mongo.Database, job *models.Job, mutation *models.Mutation) error {
	var savedJob models.Job
	if err := db.Collection("jobs").FindOne(ctx, bson.M{"_id": job.Id}).Decode(&savedJob); err != nil {
		return err
	}
	if savedJob.State != enum.JobStateCompleted || savedJob.StageId != 3 {
		return fmt.Errorf("job should be COMPLETED at stage 3, got %s at stage %d", savedJob.State, savedJob.StageId)
	}
	var savedMutation models.Mutation
	if err := db.Collection("mutations").FindOne(ctx, bson.M{"_id": mutation.Id}).Decode(&savedMutation); err != nil {
		return err
	}
	if savedMutation.State != enum.MutationStateCompleted {
		return fmt.Errorf("mutation should be COMPLETED, got %s", savedMutation.State)
	}
	stored, err := db.Collection("mutation_results").CountDocuments(ctx, bson.M{"mutation_id": mutation.Id})
	if err != nil {
		return err
	}
	if stored != 2 {
		return fmt.Errorf("expected 2 mutation results, got %d", stored)
	}
	var runs []models.EvaluationRun
	cursor, err := db.Collection("evaluation_runs").Find(ctx, bson.M{"job_id": job.Id})
	if err != nil {
		return err
	}
	if err := cursor.All(ctx, &runs); err != nil {
		return err
	}
	if len(runs) != 1 || runs[0].State != models.EvaluationCompleted || len(runs[0].MutationResultIds) != 2 {
		return fmt.Errorf("expected 1 COMPLETED evaluation run over 2 candidates, got %d runs", len(runs))
	}
	evaluated, err := db.Collection("evaluation_results").CountDocuments(ctx,
		bson.M{"evaluation_run_id": runs[0].Id, "plugin": "mock", "state": models.EvaluationCompleted})
	if err != nil {
		return err
	}
	if evaluated != 2 {
		return fmt.Errorf("expected 2 completed evaluation results, got %d", evaluated)
	}
	return nil
}
