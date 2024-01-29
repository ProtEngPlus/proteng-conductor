package repositories

import (
	"context"
	"proteng-conductor/database"
	"time"

	"proteng-conductor/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type JobRepository interface {
	Create(job *models.Job) error
	FindById(id string) (*models.Job, error)
	Update(id string, job *models.Job) error
	Delete(id string) error
	GetAll(query map[string]interface{}) ([]*models.Job, error)
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
	}

	cursor, err := jr.collection.Find(context.Background(), bson.M{})
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

func (jr *jobRepository) Create(job *models.Job) error {
	job.Id = primitive.NewObjectID()
	job.State = "CREATED"
	job.CreatedAt = time.Now()

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
			"job_name":      job.Name,
			"state":         job.State,
			"stage_id":      job.StageId,
			"lab_result":    job.LabResult,
			"options":       job.Options,
			"artifact":      job.Artifacts,
			"meta":          job.Meta,
			"input_protein": job.InputProtein,
			"ref_job_id":    job.RefJobId,
			"complete_at":   job.CompleteAt,
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
