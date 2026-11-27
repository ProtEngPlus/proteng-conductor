package repositories

import (
	"testing"

	"github.com/protengplus/proteng-conductor/models"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestJobLatestEvaluation(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("create ignores evaluation fields from the request", func(mt *mtest.T) {
		r := &jobRepository{collection: mt.Coll}
		runID := primitive.NewObjectID()
		job := &models.Job{Name: "job", EvaluationState: models.EvaluationOngoing, EvaluationRunId: &runID}
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		require.NoError(mt, r.Create(job))
		require.Empty(mt, job.EvaluationState)
		require.Nil(mt, job.EvaluationRunId)
		inserted := mt.GetStartedEvent().Command.Lookup("documents").Array().Index(0).Value().Document()
		_, err := inserted.LookupErr("evaluation_state")
		require.Error(mt, err)
		_, err = inserted.LookupErr("evaluation_run_id")
		require.Error(mt, err)
	})
	mt.Run("pending clears the run", func(mt *mtest.T) {
		r := &jobRepository{collection: mt.Coll}
		jobID := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.SetLatestEvaluation(jobID.Hex(), primitive.NilObjectID, models.EvaluationPending))
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document()
		require.Equal(mt, jobID, update.Lookup("q").Document().Lookup("_id").ObjectID())
		u := update.Lookup("u").Document()
		require.Equal(mt, "PENDING", u.Lookup("$set").Document().Lookup("evaluation_state").StringValue())
		_, err := u.Lookup("$unset").Document().LookupErr("evaluation_run_id")
		require.NoError(mt, err)
	})
	mt.Run("ongoing records the run", func(mt *mtest.T) {
		r := &jobRepository{collection: mt.Coll}
		jobID, runID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.SetLatestEvaluation(jobID.Hex(), runID, models.EvaluationOngoing))
		u := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document().Lookup("u").Document()
		set := u.Lookup("$set").Document()
		require.Equal(mt, "ONGOING", set.Lookup("evaluation_state").StringValue())
		require.Equal(mt, runID, set.Lookup("evaluation_run_id").ObjectID())
		_, err := u.LookupErr("$unset")
		require.Error(mt, err)
	})
	mt.Run("invalid latest evaluations are rejected", func(mt *mtest.T) {
		r := &jobRepository{collection: mt.Coll}
		jobID, runID := primitive.NewObjectID().Hex(), primitive.NewObjectID()
		require.Error(mt, r.SetLatestEvaluation(jobID, runID, models.EvaluationPending))
		require.Error(mt, r.SetLatestEvaluation(jobID, primitive.NilObjectID, models.EvaluationOngoing))
		require.Error(mt, r.SetLatestEvaluation(jobID, runID, models.EvaluationCompleted))
		require.Error(mt, r.SetLatestEvaluation("bad", runID, models.EvaluationOngoing))
		require.Nil(mt, mt.GetStartedEvent())
	})
	mt.Run("finish only matches the latest run", func(mt *mtest.T) {
		r := &jobRepository{collection: mt.Coll}
		jobID, runID := primitive.NewObjectID(), primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.FinishLatestEvaluation(jobID.Hex(), runID, models.EvaluationFailed))
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document()
		q := update.Lookup("q").Document()
		require.Equal(mt, jobID, q.Lookup("_id").ObjectID())
		require.Equal(mt, runID, q.Lookup("evaluation_run_id").ObjectID())
		require.Equal(mt, "FAILED", update.Lookup("u").Document().Lookup("$set").Document().Lookup("evaluation_state").StringValue())
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}))
		require.NoError(mt, r.FinishLatestEvaluation(jobID.Hex(), runID, models.EvaluationCompleted))
		mt.GetStartedEvent()
		require.Error(mt, r.FinishLatestEvaluation(jobID.Hex(), runID, models.EvaluationOngoing))
		require.Error(mt, r.FinishLatestEvaluation(jobID.Hex(), primitive.NilObjectID, models.EvaluationCompleted))
		require.Error(mt, r.FinishLatestEvaluation("bad", runID, models.EvaluationCompleted))
		require.Nil(mt, mt.GetStartedEvent())
	})
}
