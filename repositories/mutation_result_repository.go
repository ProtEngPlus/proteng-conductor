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
		// Filter min/max value of sort field
		minValue := 0.0
		maxValue := 1.0
		err := error(nil)
		if minValueStr, ok := query["min_value"]; ok {
			minValue, err = strconv.ParseFloat(minValueStr.(string), 64)
			if err != nil {
				return nil, err
			}
		}
		if maxValueStr, ok := query["max_value"]; ok {
			maxValue, err = strconv.ParseFloat(maxValueStr.(string), 64)
			if err != nil {
				return nil, err
			}
		}
		filter[sort.(string)] = bson.M{
			"$gte": minValue,
			"$lte": maxValue,
		}
	} else {
		options.SetSort(bson.D{{Key: "id", Value: -1}})
	}

	cursor, err := mrr.collection.Find(context.Background(), filter, options)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
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

	var mutationResult models.MutationResult
	err = mrr.collection.FindOne(context.Background(), filter).Decode(&mutationResult)
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
	var mutationResult models.MutationResult
	err := mrr.collection.FindOne(context.Background(), filter, options).Decode(&mutationResult)
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

	_, err := mrr.collection.InsertOne(context.Background(), mutationResult)
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

	_, err = mrr.collection.UpdateOne(context.Background(), filter, update)
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

	_, err = mrr.collection.DeleteOne(context.Background(), filter)
	if err != nil {
		return err
	}

	return nil
}
