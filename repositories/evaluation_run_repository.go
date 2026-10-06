package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type EvaluationRunRepository interface {
	Create(run *models.EvaluationRun) error
	FindById(id string) (*models.EvaluationRun, error)
	Finish(id primitive.ObjectID, state models.EvaluationState, message string) error
}

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
		return fmt.Errorf("evaluation run is no longer ongoing")
	}
	return nil
}
