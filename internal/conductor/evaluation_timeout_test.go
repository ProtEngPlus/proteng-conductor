package conductor

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestSweepStaleEvaluations(t *testing.T) {
	cutoff := time.Now().Add(-evaluationTimeout)

	t.Run("stale run fails and completes its job", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
		f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)

		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))

		require.Equal(tt, models.EvaluationFailed, f.runs.run.State)
		require.Contains(tt, f.runs.run.Error, "timed out: no result from evaluation.mock")
		require.Equal(tt, enum.JobStateCompleted, f.job.State)
		require.Zero(tt, f.results.writes)
	})

	t.Run("recent run is left alone", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.runs.run.PluginStartedAt = cutoff.Add(time.Minute)
		// no Update expected

		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))

		require.Equal(tt, models.EvaluationOngoing, f.runs.run.State)
		require.Equal(tt, enum.JobStateOnGoing, f.job.State)
	})

	t.Run("late callback after the timeout changes nothing", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
		f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)
		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))

		require.NoError(tt, f.con.OrchestrateWithError(f.message(tt)))

		require.Equal(tt, models.EvaluationFailed, f.runs.run.State)
		require.Zero(tt, f.results.writes)
		require.Equal(tt, 1, f.runs.finishes)
	})

	t.Run("deleted candidate stops the run and completes its job", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
		delete(f.candidates, f.runs.run.MutationResultIds[0].Hex())
		f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)

		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))
		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff)) // nothing left to sweep

		require.Equal(tt, models.EvaluationFailed, f.runs.run.State)
		require.Contains(tt, f.runs.run.Error, "timed out")
		require.Contains(tt, f.runs.run.Error, "run stopped")
		require.Equal(tt, 1, f.runs.finishes)
		require.Equal(tt, enum.JobStateCompleted, f.job.State)
	})

	t.Run("deleted job stops the run", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
		f.jobErr = mongo.ErrNoDocuments
		// no Update expected: there is no job left to complete

		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))

		require.Equal(tt, models.EvaluationFailed, f.runs.run.State)
		require.Contains(tt, f.runs.run.Error, "run stopped")
	})

	t.Run("temporary lookup failure keeps the run for the next sweep", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.job.State = enum.JobStateOnGoing
		f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
		f.jobErr = errors.New("database unavailable")
		// no Update expected

		require.NoError(tt, f.con.sweepStaleEvaluations(cutoff))

		require.Equal(tt, models.EvaluationOngoing, f.runs.run.State)
		require.Zero(tt, f.runs.finishes)
		require.Equal(tt, enum.JobStateOnGoing, f.job.State)
	})

	t.Run("lookup failure is reported", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.runs.staleErr = errors.New("database unavailable")

		require.ErrorIs(tt, f.con.sweepStaleEvaluations(cutoff), f.runs.staleErr)
	})
}
