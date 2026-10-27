package repositories

import (
	"testing"
	"time"

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
		require.Equal(mt, run.CreatedAt, run.PluginStartedAt)
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
	mt.Run("find stale ongoing runs", func(mt *mtest.T) {
		r := &evaluationRunRepository{collection: mt.Coll}
		cutoff := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		run := &models.EvaluationRun{Id: primitive.NewObjectID(), JobId: primitive.NewObjectID(), State: models.EvaluationOngoing}
		encoded, err := bson.Marshal(run)
		require.NoError(mt, err)
		var doc bson.D
		require.NoError(mt, bson.Unmarshal(encoded, &doc))
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_runs", mtest.FirstBatch, doc))
		runs, err := r.FindStale(cutoff)
		require.NoError(mt, err)
		require.Len(mt, runs, 1)
		require.Equal(mt, run.Id, runs[0].Id)
		filter := mt.GetStartedEvent().Command.Lookup("filter").Document()
		require.Equal(mt, "ONGOING", filter.Lookup("state").StringValue())
		require.Equal(mt, cutoff, filter.Lookup("plugin_started_at").Document().Lookup("$lt").Time().UTC())
		mt.AddMockResponses(mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 2, Message: "bad query"}))
		_, err = r.FindStale(cutoff)
		require.Error(mt, err)
	})
	mt.Run("advance is guarded by state and index", func(mt *mtest.T) {
		r := &evaluationRunRepository{collection: mt.Coll}
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		require.NoError(mt, r.Advance(id, 0, "mock: failed"))
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document()
		q := update.Lookup("q").Document()
		require.Equal(mt, id, q.Lookup("_id").ObjectID())
		require.Equal(mt, "ONGOING", q.Lookup("state").StringValue())
		require.Equal(mt, int32(0), q.Lookup("current_plugin_index").Int32())
		u := update.Lookup("u").Document()
		require.Equal(mt, int32(1), u.Lookup("$set").Document().Lookup("current_plugin_index").Int32())
		require.Equal(mt, "mock: failed", u.Lookup("$push").Document().Lookup("plugin_errors").StringValue())
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}))
		require.ErrorIs(mt, r.Advance(id, 0, ""), ErrEvaluationRunChanged)
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
	mt.Run("find runs of a job newest first", func(mt *mtest.T) {
		r := &evaluationRunRepository{collection: mt.Coll}
		jobID := primitive.NewObjectID()
		run := &models.EvaluationRun{Id: primitive.NewObjectID(), JobId: jobID, State: models.EvaluationCompleted}
		encoded, err := bson.Marshal(run)
		require.NoError(mt, err)
		var doc bson.D
		require.NoError(mt, bson.Unmarshal(encoded, &doc))
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_runs", mtest.FirstBatch, doc))
		runs, err := r.FindByJob(jobID.Hex())
		require.NoError(mt, err)
		require.Len(mt, runs, 1)
		require.Equal(mt, run.Id, runs[0].Id)
		command := mt.GetStartedEvent().Command
		require.Equal(mt, jobID, command.Lookup("filter").Document().Lookup("job_id").ObjectID())
		require.Equal(mt, int32(-1), command.Lookup("sort").Document().Lookup("created_at").Int32())
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_runs", mtest.FirstBatch))
		runs, err = r.FindByJob(jobID.Hex())
		require.NoError(mt, err)
		require.NotNil(mt, runs)
		require.Empty(mt, runs)
		_, err = r.FindByJob("bad")
		require.Error(mt, err)
	})
	mt.Run("find results of a run", func(mt *mtest.T) {
		r := &evaluationResultRepository{collection: mt.Coll}
		runID := primitive.NewObjectID()
		result := &models.EvaluationResult{
			Id: primitive.NewObjectID(), EvaluationRunId: runID, MutationResultId: primitive.NewObjectID(), Plugin: "mock",
			State: models.EvaluationCompleted, Values: map[string]interface{}{"mock_score": 0.5},
		}
		encoded, err := bson.Marshal(result)
		require.NoError(mt, err)
		var doc bson.D
		require.NoError(mt, bson.Unmarshal(encoded, &doc))
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_results", mtest.FirstBatch, doc))
		results, err := r.FindByRun(runID.Hex())
		require.NoError(mt, err)
		require.Len(mt, results, 1)
		require.Equal(mt, result.MutationResultId, results[0].MutationResultId)
		require.Equal(mt, 0.5, results[0].Values["mock_score"])
		require.Equal(mt, runID, mt.GetStartedEvent().Command.Lookup("filter").Document().Lookup("evaluation_run_id").ObjectID())
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".evaluation_results", mtest.FirstBatch))
		results, err = r.FindByRun(runID.Hex())
		require.NoError(mt, err)
		require.NotNil(mt, results)
		require.Empty(mt, results)
		_, err = r.FindByRun("bad")
		require.Error(mt, err)
	})
}
