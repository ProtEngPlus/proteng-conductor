// evaluation-smoke runs the Phase 3.2 round trip on local, isolated services.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/logger"
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

const smokeDB = "blx01_smoke"

func main() {
	mongoURI := flag.String("mongo-uri", "mongodb://127.0.0.1:27018", "local smoke MongoDB")
	amqpURL := flag.String("rabbitmq-url", "amqp://guest:guest@127.0.0.1:5674/", "local smoke RabbitMQ")
	flag.Parse()
	logger.InitZap()
	if err := runSmoke(*mongoURI, *amqpURL); err != nil {
		fmt.Fprintln(os.Stderr, "Smoke test failed:", err)
		os.Exit(1)
	}
}

func validateLocalURI(raw, scheme string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != scheme {
		return fmt.Errorf("smoke URI must use %s", scheme)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("smoke services must be on localhost")
	}
	return nil
}

func newSmokeFixture() (*models.Job, *models.Mutation, []*models.MutationResult, *models.EvaluationRun) {
	jobID, mutationID := primitive.NewObjectID(), primitive.NewObjectID()
	job := &models.Job{Id: jobID, Name: "BLX01 phase 3.2 smoke", UserId: "blx01-smoke", StageId: 3,
		State: enum.JobStateCompleted, InputProtein: "ACDEFGHIK", RunType: "manual"}
	mutation := &models.Mutation{Id: mutationID, JobId: jobID, UserId: job.UserId,
		State: enum.MutationStateCompleted, InputProtein: job.InputProtein, Tool: "mutation"}
	candidates := []*models.MutationResult{
		{Id: primitive.NewObjectID(), JobId: jobID, MutationId: mutationID, UserId: job.UserId, ProteinSequence: "ACDEFGHIK", AssayScore: 1},
		{Id: primitive.NewObjectID(), JobId: jobID, MutationId: mutationID, UserId: job.UserId, ProteinSequence: "ACDEYGHIK", AssayScore: 2},
	}
	run := &models.EvaluationRun{JobId: jobID, MutationId: mutationID, Plugins: []string{"mock"},
		MutationResultIds: []primitive.ObjectID{candidates[0].Id, candidates[1].Id}}
	return job, mutation, candidates, run
}

func evaluationRequest(job *models.Job, candidates []*models.MutationResult, run *models.EvaluationRun) map[string]interface{} {
	mutants := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		mutants = append(mutants, map[string]interface{}{"mutation_result_id": candidate.Id.Hex(),
			"sequence": candidate.ProteinSequence, "assay_score": candidate.AssayScore})
	}
	return map[string]interface{}{"job_id": job.Id.Hex(), "evaluation_run_id": run.Id.Hex(),
		"plugin": "mock", "wild_type": map[string]string{"sequence": job.InputProtein},
		"mutants": mutants, "config": map[string]interface{}{}}
}

func runSmoke(mongoURI, amqpURL string) error {
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
	job, mutation, candidates, run := newSmokeFixture()
	for name, documents := range map[string][]interface{}{
		"jobs": {job}, "mutations": {mutation}, "mutation_results": {candidates[0], candidates[1]},
	} {
		if _, err := client.Database(smokeDB).Collection(name).InsertMany(ctx, documents); err != nil {
			return err
		}
	}
	if err := runs.Create(run); err != nil {
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
	body, err := json.Marshal(evaluationRequest(job, candidates, run))
	if err != nil {
		return err
	}
	if err := pub.PublishWithTopic(ctx, "evaluation.mock", body); err != nil {
		return err
	}
	fmt.Printf("Waiting for worker: job=%s evaluation_run=%s\n", job.Id.Hex(), run.Id.Hex())
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out; check worker uses this broker and is waiting on evaluation.mock")
		case err := <-done:
			return fmt.Errorf("smoke consumer stopped: %v", err)
		case <-ticker.C:
			stored, err := runs.FindById(run.Id.Hex())
			if err != nil {
				return err
			}
			if stored.State == models.EvaluationFailed {
				return fmt.Errorf("worker failed: %s", stored.Error)
			}
			if stored.State != models.EvaluationCompleted {
				continue
			}
			filter := bson.M{"evaluation_run_id": run.Id}
			count, err := client.Database(smokeDB).Collection("evaluation_results").CountDocuments(ctx, filter)
			if err != nil {
				return err
			}
			if count != 2 {
				return fmt.Errorf("expected two persisted results, got %d", count)
			}
			for i, candidate := range candidates {
				var result models.EvaluationResult
				if err := client.Database(smokeDB).Collection("evaluation_results").FindOne(ctx,
					bson.M{"evaluation_run_id": run.Id, "mutation_result_id": candidate.Id, "plugin": "mock"}).Decode(&result); err != nil {
					return err
				}
				if result.State != models.EvaluationCompleted || result.Values["mock_score"] != 0.50+float64(i)*0.01 {
					return fmt.Errorf("unexpected result for candidate %s: %#v", candidate.Id.Hex(), result.Values)
				}
			}
			var savedJob models.Job
			if err := client.Database(smokeDB).Collection("jobs").FindOne(ctx, bson.M{"_id": job.Id}).Decode(&savedJob); err != nil {
				return err
			}
			if savedJob.StageId != 3 || savedJob.State != enum.JobStateCompleted {
				return fmt.Errorf("Evaluation changed the parent job")
			}
			fmt.Printf("PASS: EvaluationRun COMPLETED; two EvaluationResults (0.50, 0.51); parent job still COMPLETED at stage 3. Database: %s\n", smokeDB)
			return nil
		}
	}
}
