package repositories

import (
	"testing"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func useEvaluationMockDatabase(mt *mtest.T) {
	oldClient, oldDB := database.Client, config.Config.MongoDb
	database.Client, config.Config.MongoDb = mt.Client, mt.DB.Name()
	mt.Cleanup(func() { database.Client, config.Config.MongoDb = oldClient, oldDB })
}

func TestEvaluationRepositories(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("result identity index", func(mt *mtest.T) {
		useEvaluationMockDatabase(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		_, err := NewEvaluationResultRepository()
		require.NoError(mt, err)
		event := mt.GetStartedEvent()
		require.Equal(mt, "createIndexes", event.CommandName)
		index := event.Command.Lookup("indexes").Array().Index(0).Value().Document()
		require.True(mt, index.Lookup("unique").Boolean())
		keys := index.Lookup("key").Document()
		require.Equal(mt, int32(1), keys.Lookup("evaluation_run_id").Int32())
		require.Equal(mt, int32(1), keys.Lookup("plugin").Int32())
		require.Equal(mt, int32(1), keys.Lookup("mutation_result_id").Int32())
	})
	mt.Run("index failure stops initialization", func(mt *mtest.T) {
		useEvaluationMockDatabase(mt)
		mt.AddMockResponses(mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 85, Message: "conflicting index"}))
		_, err := NewEvaluationResultRepository()
		require.Error(mt, err)
	})
	mt.Run("create and find registered run", func(mt *mtest.T) {
		useEvaluationMockDatabase(mt)
		r := NewEvaluationRunRepository()
		run := &models.EvaluationRun{
			JobId: primitive.NewObjectID(), MutationId: primitive.NewObjectID(),
			MutationResultIds: []primitive.ObjectID{primitive.NewObjectID()}, Plugins: []string{"mock"},
		}
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		require.NoError(mt, r.Create(run))
		require.False(mt, run.Id.IsZero())
		require.Equal(mt, models.EvaluationOngoing, run.State)
		require.False(mt, run.CreatedAt.IsZero())
		mt.GetStartedEvent() // insert
		encoded, err := bson.Marshal(run)
		require.NoError(mt, err)
		var doc bson.D
		require.NoError(mt, bson.Unmarshal(encoded, &doc))
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_runs", mtest.FirstBatch, doc))
		found, err := r.FindById(run.Id.Hex())
		require.NoError(mt, err)
		require.Equal(mt, run.Id, found.Id)
		require.Equal(mt, run.MutationResultIds, found.MutationResultIds)
		event := mt.GetStartedEvent()
		require.Equal(mt, run.Id, event.Command.Lookup("filter").Document().Lookup("_id").ObjectID())
		require.Error(mt, r.Create(&models.EvaluationRun{}))
		_, err = r.FindById("bad")
		require.Error(mt, err)
	})
	mt.Run("finish uses ongoing state guard", func(mt *mtest.T) {
		r := &evaluationRunRepository{collection: mt.Coll}
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.Finish(id, models.EvaluationCompleted, ""))
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document()
		require.Equal(mt, "ONGOING", update.Lookup("q").Document().Lookup("state").StringValue())
		require.Equal(mt, id, update.Lookup("q").Document().Lookup("_id").ObjectID())
		require.Equal(mt, "COMPLETED", update.Lookup("u").Document().Lookup("$set").Document().Lookup("state").StringValue())
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}))
		require.Error(mt, r.Finish(id, models.EvaluationFailed, "late"))
		require.Error(mt, r.Finish(id, models.EvaluationOngoing, ""))
	})
	mt.Run("upsert identity and generic values", func(mt *mtest.T) {
		r := &evaluationResultRepository{collection: mt.Coll}
		runID, candidateID := primitive.NewObjectID(), primitive.NewObjectID()
		result := &models.EvaluationResult{
			EvaluationRunId: runID, MutationResultId: candidateID, Plugin: "mock",
			State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.5},
		}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.UpsertAll([]*models.EvaluationResult{result}))
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document()
		require.True(mt, update.Lookup("upsert").Boolean())
		filter := update.Lookup("q").Document()
		require.Equal(mt, runID, filter.Lookup("evaluation_run_id").ObjectID())
		require.Equal(mt, candidateID, filter.Lookup("mutation_result_id").ObjectID())
		require.Equal(mt, "mock", filter.Lookup("plugin").StringValue())
		require.Equal(mt, 0.5, update.Lookup("u").Document().Lookup("$set").Document().Lookup("values").Document().Lookup("mock_score").Double())
		mt.AddMockResponses(mtest.CreateWriteErrorsResponse(mtest.WriteError{Index: 0, Code: 121, Message: "write rejected"}))
		require.Error(mt, r.UpsertAll([]*models.EvaluationResult{result}))
		require.Error(mt, r.UpsertAll(nil))
		require.Error(mt, r.UpsertAll([]*models.EvaluationResult{nil}))
	})
}
