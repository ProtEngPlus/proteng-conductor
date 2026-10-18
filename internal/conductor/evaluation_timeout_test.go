package conductor

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
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

	t.Run("lookup failure is reported", func(tt *testing.T) {
		f := newEvaluationFixture(tt)
		f.runs.staleErr = errors.New("database unavailable")

		require.ErrorIs(tt, f.con.sweepStaleEvaluations(cutoff), f.runs.staleErr)
	})
}
