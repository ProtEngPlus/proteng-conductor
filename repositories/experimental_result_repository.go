package repositories

import (
	"context"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate mockgen -source=experimental_result_repository.go -destination=mock_repository/mock_experimental_result_repository.go -package=mock_repository

type ExperimentalResultRepository interface {
	Upsert(mutationResultId string, experimentalResult *models.ExperimentalResult) error
	FindByMutationResultId(mutationResultId string) (*models.ExperimentalResult, error)
	DeleteByMutationResultId(mutationResultId string) error
	DeleteByJobId(jobId string) error
	DeleteByMutationId(mutationId string) error
	GetAll(query map[string]interface{}) ([]*models.ExperimentalResult, error)
}

type experimentalResultRepository struct {
	collection *mongo.Collection
}

func NewExperimentalResultRepository() ExperimentalResultRepository {
	return &experimentalResultRepository{collection: database.GetCollection("experimental_results")}
}

func (er *experimentalResultRepository) GetAll(query map[string]interface{}) ([]*models.ExperimentalResult, error) {
	experimentalResults := []*models.ExperimentalResult{}
	filter := bson.M{}

	for _, key := range []string{"job_id", "mutation_id"} {
		if value, ok := query[key]; ok {
			objectId, err := primitive.ObjectIDFromHex(value.(string))
			if err != nil {
				return nil, err
			}
			filter[key] = objectId
		}
	}
	if userId, ok := query["user_id"]; ok {
		filter["user_id"] = userId
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	cursor, err := er.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &experimentalResults); err != nil {
		return nil, err
	}

	return experimentalResults, nil
}

func (er *experimentalResultRepository) FindByMutationResultId(mutationResultId string) (*models.ExperimentalResult, error) {
	objectId, err := primitive.ObjectIDFromHex(mutationResultId)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"mutation_result_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	var experimentalResult models.ExperimentalResult
	err = er.collection.FindOne(ctx, filter).Decode(&experimentalResult)
	if err != nil {
		return nil, err
	}

	return &experimentalResult, nil
}

func (er *experimentalResultRepository) Upsert(mutationResultId string, experimentalResult *models.ExperimentalResult) error {
	objectId, err := primitive.ObjectIDFromHex(mutationResultId)
	if err != nil {
		return err
	}

	filter := bson.M{"mutation_result_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"mutation_id":        experimentalResult.MutationId,
			"job_id":             experimentalResult.JobId,
			"user_id":            experimentalResult.UserId,
			"actual_assay_score": experimentalResult.ActualAssayScore,
			"note":               experimentalResult.Note,
			"measured_at":        experimentalResult.MeasuredAt,
		},
	}

	options := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	return er.collection.FindOneAndUpdate(ctx, filter, update, options).Decode(experimentalResult)
}

func (er *experimentalResultRepository) DeleteByMutationResultId(mutationResultId string) error {
	return er.deleteManyByObjectId("mutation_result_id", mutationResultId)
}

func (er *experimentalResultRepository) DeleteByJobId(jobId string) error {
	return er.deleteManyByObjectId("job_id", jobId)
}

func (er *experimentalResultRepository) DeleteByMutationId(mutationId string) error {
	return er.deleteManyByObjectId("mutation_id", mutationId)
}

func (er *experimentalResultRepository) deleteManyByObjectId(key string, id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = er.collection.DeleteMany(ctx, bson.M{key: objectId})
	return err
}
