package repositories

import (
	"context"
	"strconv"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate mockgen -source=mutation_repository.go -destination=mock_repository/mock_mutation_repository.go -package=mock_repository

type MutationRepository interface {
	Create(mutation *models.Mutation) error
	FindById(id string) (*models.Mutation, error)
	Update(id string, mutation *models.Mutation) error
	Delete(id string) error
	DeleteByJobId(jobId string) error
	GetAll(query map[string]interface{}) ([]*models.Mutation, error)
}

type mutationRepository struct {
	collection *mongo.Collection
}

func NewMutationRepository() MutationRepository {
	return &mutationRepository{collection: database.GetCollection("mutations")}
}

func (mr *mutationRepository) GetAll(query map[string]interface{}) ([]*models.Mutation, error) {
	var mutations []*models.Mutation
	filter := bson.M{}

	if len(query) > 0 {
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
			if err != nil {
				return nil, err
			}
			filter["is_bookmark"] = isBookmark
		}
		if userID, ok := query["user_id"]; ok {
			filter["user_id"] = userID
		}
	}

	options := options.Find()
	if sort, ok := query["sort"]; ok {
		if order, ok := query["order"]; ok {
			options.SetSort(bson.D{{Key: sort.(string), Value: order.(int)}})
		} else {
			options.SetSort(bson.D{{Key: sort.(string), Value: -1}})
		}
	} else {
		options.SetSort(bson.D{{Key: "run_id", Value: -1}})
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	cursor, err := mr.collection.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var mutation models.Mutation
		if err := cursor.Decode(&mutation); err != nil {
			return nil, err
		}
		mutations = append(mutations, &mutation)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return mutations, nil
}

func (mr *mutationRepository) FindById(id string) (*models.Mutation, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	var mutation models.Mutation
	err = mr.collection.FindOne(ctx, filter).Decode(&mutation)
	if err != nil {
		return nil, err
	}

	return &mutation, nil
}

func (mr *mutationRepository) Create(mutation *models.Mutation) error {
	mutation.Id = primitive.NewObjectID()
	mutation.State = enum.MutationStatePending
	mutation.CreatedAt = time.Now()

	var sortFilter = bson.M{
		"job_id": mutation.JobId.Hex(),
		"sort":   "run_id",
		"order":  -1,
	}
	thisJobMutations, err := mr.GetAll(sortFilter)
	if err != nil {
		return err
	}

	if len(thisJobMutations) == 0 {
		mutation.RunId = 1
	} else {
		mutation.RunId = thisJobMutations[0].RunId + 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mr.collection.InsertOne(ctx, mutation)
	if err != nil {
		return err
	}

	return nil
}

func (mr *mutationRepository) Update(id string, mutation *models.Mutation) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"name":           mutation.Name,
			"state":          string(mutation.State),
			"options":        mutation.Options,
			"tool":           mutation.Tool,
			"input_protein":  mutation.InputProtein,
			"is_bookmark":    mutation.IsBookmark,
			"histogram_data": mutation.HistogramData,
			"complete_at":    mutation.CompleteAt,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mr.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (mr *mutationRepository) Delete(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mr.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

func (mr *mutationRepository) DeleteByJobId(jobId string) error {
	objectId, err := primitive.ObjectIDFromHex(jobId)
	if err != nil {
		return err
	}

	filter := bson.M{"job_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = mr.collection.DeleteMany(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}
