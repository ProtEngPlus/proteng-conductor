package repositories

import (
	"context"
	"strconv"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate mockgen -source=mutation_result_repository.go -destination=mock_repository/mock_mutation_result_repository.go -package=mock_repository

type MutationResultRepository interface {
	Create(mutationResult *models.MutationResult) error
	FindById(id string) (*models.MutationResult, error)
	FindBestResult(userId string) (*models.BestAssayScore, error)
	Update(id string, mutationResult *models.MutationResult) error
	Delete(id string) error
	DeleteByJobId(jobId string) error
	DeleteByMutationId(mutationId string) error
	GetAll(query map[string]interface{}) ([]*models.MutationResult, error)
}

type mutationResultRepository struct {
	collection *mongo.Collection
}

func NewMutationResultRepository() MutationResultRepository {
	return &mutationResultRepository{collection: database.GetCollection("mutation_results")}
}

func (mrr *mutationResultRepository) GetAll(query map[string]interface{}) ([]*models.MutationResult, error) {
	var mutationResults []*models.MutationResult
	filter := bson.M{}

	if len(query) > 0 {
		if mutationID, ok := query["mutation_id"]; ok {
			mutationID, err := primitive.ObjectIDFromHex(mutationID.(string))
			if err != nil {
				return nil, err
			}
			filter["mutation_id"] = mutationID
		}
		if jobID, ok := query["job_id"]; ok {
			jobID, err := primitive.ObjectIDFromHex(jobID.(string))
			if err != nil {
				return nil, err
			}
			filter["job_id"] = jobID
		}
		if isBookmark, ok := query["is_bookmark"]; ok {
			isBookmark, err := strconv.ParseBool(isBookmark.(string))
			if err != nil {
				return nil, err
			}
			filter["is_bookmark"] = isBookmark
		}
	}

	options := options.Find()
	if sort, ok := query["sort"]; ok {
		if order, ok := query["order"]; ok {
			options.SetSort(bson.D{{Key: sort.(string), Value: order.(int)}})
		} else {
			options.SetSort(bson.D{{Key: sort.(string), Value: -1}})
		}
		rangeFilter := bson.M{}
		if minValueStr, ok := query["min_value"]; ok {
			minValue, err := strconv.ParseFloat(minValueStr.(string), 64)
			if err != nil {
				return nil, err
			}
			rangeFilter["$gte"] = minValue
		}
		if maxValueStr, ok := query["max_value"]; ok {
			maxValue, err := strconv.ParseFloat(maxValueStr.(string), 64)
			if err != nil {
				return nil, err
			}
			rangeFilter["$lte"] = maxValue
		}
		if len(rangeFilter) > 0 {
			filter[sort.(string)] = rangeFilter
		}
	} else {
		options.SetSort(bson.D{{Key: "id", Value: -1}})
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	cursor, err := mrr.collection.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var mutationResult models.MutationResult
		if err := cursor.Decode(&mutationResult); err != nil {
			return nil, err
		}
		mutationResults = append(mutationResults, &mutationResult)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return mutationResults, nil
}

func (mrr *mutationResultRepository) FindById(id string) (*models.MutationResult, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	var mutationResult models.MutationResult
	err = mrr.collection.FindOne(ctx, filter).Decode(&mutationResult)
	if err != nil {
		return nil, err
	}

	return &mutationResult, nil
}

func (mrr *mutationResultRepository) FindBestResult(userId string) (*models.BestAssayScore, error) {
	filter := bson.M{
		"user_id": userId,
	}
	// Sort by assay_score in descending order to get the best result
	options := options.FindOne().SetSort(bson.D{{Key: "assay_score", Value: -1}})
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	var mutationResult models.MutationResult
	err := mrr.collection.FindOne(ctx, filter, options).Decode(&mutationResult)
	if err != nil {
		return nil, nil
	}

	bestAssayScore := models.BestAssayScore{
		Id:         mutationResult.JobId,
		AssayScore: mutationResult.AssayScore,
	}

	return &bestAssayScore, nil
}

func (mrr *mutationResultRepository) Create(mutationResult *models.MutationResult) error {
	mutationResult.Id = primitive.NewObjectID()

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err := mrr.collection.InsertOne(ctx, mutationResult)
	if err != nil {
		return err
	}

	return nil
}

func (mrr *mutationResultRepository) Update(id string, mutationResult *models.MutationResult) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"is_bookmark": mutationResult.IsBookmark,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mrr.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (mrr *mutationResultRepository) Delete(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mrr.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

func (mrr *mutationResultRepository) DeleteByJobId(jobId string) error {
	objectId, err := primitive.ObjectIDFromHex(jobId)
	if err != nil {
		return err
	}

	filter := bson.M{"job_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mrr.collection.DeleteMany(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

func (mrr *mutationResultRepository) DeleteByMutationId(mutationId string) error {
	objectId, err := primitive.ObjectIDFromHex(mutationId)
	if err != nil {
		return err
	}

	filter := bson.M{"mutation_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mrr.collection.DeleteMany(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}
