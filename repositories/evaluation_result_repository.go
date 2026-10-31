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
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EvaluationResultRepository interface {
	UpsertAll(results []*models.EvaluationResult) error
}

type evaluationResultRepository struct {
	collection *mongo.Collection
}

func NewEvaluationResultRepository() (EvaluationResultRepository, error) {
	r := &evaluationResultRepository{collection: database.GetCollection("evaluation_results")}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "evaluation_run_id", Value: 1},
			{Key: "plugin", Value: 1},
			{Key: "mutation_result_id", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("evaluation_result_identity"),
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Upserts make retries after a partially successful bulk write safe.
// The run must remain ongoing until every result has been saved.
func (r *evaluationResultRepository) UpsertAll(results []*models.EvaluationResult) error {
	if len(results) == 0 {
		return fmt.Errorf("evaluation results cannot be empty")
	}
	now := time.Now().UTC()
	writes := make([]mongo.WriteModel, 0, len(results))
	for _, result := range results {
		if result == nil || result.EvaluationRunId.IsZero() || result.MutationResultId.IsZero() || result.Plugin == "" {
			return fmt.Errorf("evaluation result requires run, candidate, and plugin")
		}
		writes = append(writes, mongo.NewUpdateOneModel().SetUpsert(true).
			SetFilter(bson.M{
				"evaluation_run_id":  result.EvaluationRunId,
				"mutation_result_id": result.MutationResultId,
				"plugin":             result.Plugin,
			}).SetUpdate(bson.M{
			"$set":         bson.M{"state": result.State, "values": result.Values, "error": result.Error, "updated_at": now},
			"$setOnInsert": bson.M{"_id": primitive.NewObjectID(), "created_at": now},
		}))
	}
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()
	_, err := r.collection.BulkWrite(ctx, writes)
	return err
}
