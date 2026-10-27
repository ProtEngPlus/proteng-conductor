package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EvaluationRunRepository interface {
	Create(run *models.EvaluationRun) error
	FindById(id string) (*models.EvaluationRun, error)
	FindByJob(jobId string) ([]*models.EvaluationRun, error)
	Finish(id primitive.ObjectID, state models.EvaluationState, message string) error
	FindStale(pluginStartedBefore time.Time) ([]*models.EvaluationRun, error)
	Advance(id primitive.ObjectID, fromIndex int, pluginError string) error
}

// ErrEvaluationRunChanged means a guarded update found the run already finished
// or moved past the expected plugin, so another delivery handled it first.
var ErrEvaluationRunChanged = errors.New("evaluation run already moved on")

type evaluationRunRepository struct {
	collection *mongo.Collection
}

func NewEvaluationRunRepository() EvaluationRunRepository {
	return &evaluationRunRepository{collection: database.GetCollection("evaluation_runs")}
}

func (r *evaluationRunRepository) Create(run *models.EvaluationRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Id.IsZero() {
		run.Id = primitive.NewObjectID()
	}
	run.State = models.EvaluationOngoing
	run.Error = ""
	run.CreatedAt = time.Now().UTC()
	run.PluginStartedAt = run.CreatedAt
	run.CompleteAt = time.Time{}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	_, err := r.collection.InsertOne(ctx, run)
	return err
}

func (r *evaluationRunRepository) FindById(id string) (*models.EvaluationRun, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	var run models.EvaluationRun
	if err := r.collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *evaluationRunRepository) FindByJob(jobId string) ([]*models.EvaluationRun, error) {
	objectID, err := primitive.ObjectIDFromHex(jobId)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	cursor, err := r.collection.Find(ctx, bson.M{"job_id": objectID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	runs := []*models.EvaluationRun{}
	if err := cursor.All(ctx, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// Finish only transitions ongoing runs. A repeated callback cannot reopen history.
func (r *evaluationRunRepository) Finish(id primitive.ObjectID, state models.EvaluationState, message string) error {
	if state != models.EvaluationCompleted && state != models.EvaluationFailed {
		return fmt.Errorf("evaluation finish requires a terminal state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": id, "state": models.EvaluationOngoing},
		bson.M{"$set": bson.M{"state": state, "error": message, "complete_at": time.Now().UTC()}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("evaluation run is no longer ongoing: %w", ErrEvaluationRunChanged)
	}
	return nil
}

// Advance moves an ongoing run from plugin fromIndex to the next one, recording
// why the plugin failed when pluginError is set. It changes nothing when the run
// is finished or already past fromIndex, so a redelivery cannot skip a plugin.
func (r *evaluationRunRepository) Advance(id primitive.ObjectID, fromIndex int, pluginError string) error {
	update := bson.M{"$set": bson.M{"current_plugin_index": fromIndex + 1, "plugin_started_at": time.Now().UTC()}}
	if pluginError != "" {
		update["$push"] = bson.M{"plugin_errors": pluginError}
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": id, "state": models.EvaluationOngoing, "current_plugin_index": fromIndex},
		update,
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrEvaluationRunChanged
	}
	return nil
}

// FindStale returns ongoing runs whose current plugin was dispatched before the cutoff.
func (r *evaluationRunRepository) FindStale(pluginStartedBefore time.Time) ([]*models.EvaluationRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	cursor, err := r.collection.Find(ctx, bson.M{
		"state":             models.EvaluationOngoing,
		"plugin_started_at": bson.M{"$lt": pluginStartedBefore.UTC()},
	})
	if err != nil {
		return nil, err
	}
	var runs []*models.EvaluationRun
	if err := cursor.All(ctx, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}
