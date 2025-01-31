package repositories

import (
	"context"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate mockgen -source=job_repository.go -destination=mock_repository/mock_job_repository.go -package=mock_repository

type JobRepository interface {
	Create(job *models.Job) error
	FindById(id string) (*models.Job, error)
	FindRecent(userID string) (*models.RecentJob, error)
	Update(id string, job *models.Job) error
	Delete(id string) error
	GetAll(query map[string]interface{}) ([]*models.Job, error)
	AddErrorLog(id string, log string) error
}

type jobRepository struct {
	collection *mongo.Collection
}

func NewJobRepository() JobRepository {
	collection := database.GetCollection("jobs")
	collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{
			"name": "text",
		},
	})
	return &jobRepository{collection: collection}
}

func (jr *jobRepository) GetAll(query map[string]interface{}) ([]*models.Job, error) {
	var jobs []*models.Job
	filter := bson.M{}

	if len(query) > 0 {
		if userID, ok := query["user_id"]; ok {
			filter["user_id"] = userID
		}
		if states, ok := query["state"]; ok {
			filter["state"] = bson.M{"$in": states}
		}
		if name, ok := query["name"]; ok {
			filter["$text"] = bson.M{"$search": name}
		}
		if favorite, ok := query["favorite"]; ok {
			filter["is_favorite"] = favorite
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
		options.SetSort(bson.D{{Key: "created_at", Value: -1}})
	}

	cursor, err := jr.collection.Find(context.Background(), filter, options)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var job models.Job
		if err := cursor.Decode(&job); err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (jr *jobRepository) FindById(id string) (*models.Job, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objectId}

	var job models.Job
	err = jr.collection.FindOne(context.Background(), filter).Decode(&job)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (jr *jobRepository) FindRecent(userID string) (*models.RecentJob, error) {
	filter := bson.M{}
	filter["user_id"] = userID

	options := options.Find()
	options.SetSort(bson.D{{Key: "created_at", Value: -1}})
	options.SetLimit(1)
	options.SetProjection(bson.M{
		"_id":         1,
		"name":        1,
		"description": 1,
	})

	cursor, err := jr.collection.Find(context.Background(), filter, options)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	var job models.RecentJob
	if cursor.Next(context.Background()) {
		if err := cursor.Decode(&job); err != nil {
			return nil, err
		}
	} else {
		return nil, nil
	}

	return &job, nil
}

func (jr *jobRepository) Create(job *models.Job) error {
	job.Id = primitive.NewObjectID()
	job.State = enum.JobStateCreated
	job.CreatedAt = time.Now()
	job.ErrorLogs = []models.ErrLog{}

	_, err := jr.collection.InsertOne(context.Background(), job)
	if err != nil {
		return err
	}

	return nil
}

func (jr *jobRepository) Update(id string, job *models.Job) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"name":               job.Name,
			"state":              string(job.State),
			"stage_id":           job.StageId,
			"lab_result":         job.LabResult,
			"options":            job.Options,
			"artifact":           job.Artifacts,
			"meta":               job.Meta,
			"input_protein":      job.InputProtein,
			"run_type":           job.RunType,
			"description":        job.Description,
			"is_notification_on": job.IsNotificationOn,
			"complete_at":        job.CompleteAt,
		},
	}

	_, err = jr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (jr *jobRepository) Delete(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectId}

	_, err = jr.collection.DeleteOne(context.Background(), filter)
	if err != nil {
		return err
	}

	return nil
}

func (jr *jobRepository) AddErrorLog(id string, log string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		logger.Errorf("JobRepository: AddErrorLog: %s", err.Error())
		return err
	}

	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$push": bson.M{
			"error_logs": models.ErrLog{
				Content:   log,
				Timestamp: time.Now(),
			},
		},
	}

	_, err = jr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		logger.Errorf("JobRepository: AddErrorLog: %s", err.Error())
		return err
	}

	return nil
}
