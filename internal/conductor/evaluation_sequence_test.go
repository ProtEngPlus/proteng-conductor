package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
)

// newSequenceFixture is a job waiting at stage 3 for a two-plugin run.
func newSequenceFixture(t *testing.T) *evaluationFixture {
	f := newEvaluationFixture(t)
	f.job.State = enum.JobStateOnGoing
	f.runs.run.Plugins = []string{"mock", "kcat"}
	return f
}

func (f *evaluationFixture) expectDispatchTo(plugin string, err error, sent *[]byte) {
	f.deps.publisher.EXPECT().PublishWithTopic(gomock.Any(), "evaluation."+plugin, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, body []byte) error {
			if sent != nil {
				*sent = body
			}
			return err
		})
}

func (f *evaluationFixture) replyFrom(t *testing.T, plugin, status string) error {
	f.data.Plugin = plugin
	if status == "FAILED" {
		f.data.Status, f.data.Error, f.data.EvaluationResults = "FAILED", plugin+" broke", nil
	} else {
		f.data.Status, f.data.Error = "COMPLETED", ""
		f.data.EvaluationResults = nil
		for _, id := range f.runs.run.MutationResultIds {
			f.data.EvaluationResults = append(f.data.EvaluationResults, EvaluationResultData{
				MutationResultID: id.Hex(), Values: map[string]interface{}{plugin + "_score": 1.0},
			})
		}
	}
	return f.con.OrchestrateWithError(f.message(t))
}

func TestEvaluationRunsPluginsInOrder(t *testing.T) {
	f := newSequenceFixture(t)
	var sent []byte
	f.expectDispatchTo("kcat", nil, &sent)

	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED"))

	require.Equal(t, models.EvaluationOngoing, f.runs.run.State)
	require.Equal(t, 1, f.runs.run.CurrentPluginIndex)
	require.Equal(t, enum.JobStateOnGoing, f.job.State)
	var request map[string]interface{}
	require.NoError(t, json.Unmarshal(sent, &request))
	require.Equal(t, "kcat", request["plugin"])
	require.Len(t, request["mutants"], 2)

	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)
	require.NoError(t, f.replyFrom(t, "kcat", "COMPLETED"))

	require.Equal(t, models.EvaluationCompleted, f.runs.run.State)
	require.Empty(t, f.runs.run.Error)
	require.Equal(t, enum.JobStateCompleted, f.job.State)
	require.Equal(t, 2, f.results.writes)
}

func TestEvaluationFailedPluginDoesNotStopTheNext(t *testing.T) {
	f := newSequenceFixture(t)
	f.expectDispatchTo("kcat", nil, nil)

	require.NoError(t, f.replyFrom(t, "mock", "FAILED"))
	require.Equal(t, 1, f.runs.run.CurrentPluginIndex)
	require.Equal(t, []string{"mock: mock broke"}, f.runs.run.PluginErrors)

	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)
	require.NoError(t, f.replyFrom(t, "kcat", "COMPLETED"))

	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
	require.Equal(t, "mock: mock broke", f.runs.run.Error)
	require.Equal(t, enum.JobStateCompleted, f.job.State)
}

func TestEvaluationRedeliveredEarlierPluginIsIgnored(t *testing.T) {
	f := newSequenceFixture(t)
	f.expectDispatchTo("kcat", nil, nil)
	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED"))

	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED")) // no second dispatch expected

	require.Equal(t, 1, f.results.writes)
	require.Equal(t, 1, f.runs.advances)
}

func TestEvaluationCallbackFromALaterPluginIsRejected(t *testing.T) {
	f := newSequenceFixture(t)

	require.ErrorIs(t, f.con.receiveEvaluation(Data{
		JobID: f.data.JobID, EvaluationRunID: f.data.EvaluationRunID, StageID: EvaluationStageID,
		Plugin: "kcat", Status: "FAILED", Error: "too early",
	}), ErrInvalidEvaluation)
	require.Zero(t, f.results.writes)
}

func TestEvaluationUnreachableLastPluginFinishesTheRun(t *testing.T) {
	f := newSequenceFixture(t)
	f.expectDispatchTo("kcat", errors.New("broker down"), nil)
	f.deps.jobRepository.EXPECT().Update(f.job.Id.Hex(), gomock.Any()).Return(nil)

	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED"))

	require.Equal(t, models.EvaluationFailed, f.runs.run.State)
	require.Contains(t, f.runs.run.Error, "kcat: dispatch failed: broker down")
	require.Equal(t, enum.JobStateCompleted, f.job.State)
}

func TestEvaluationAdvanceFailureIsRetried(t *testing.T) {
	f := newSequenceFixture(t)
	failure := errors.New("database unavailable")
	f.runs.advanceErr = failure

	require.ErrorIs(t, f.replyFrom(t, "mock", "COMPLETED"), failure)
	require.Equal(t, 0, f.runs.run.CurrentPluginIndex)

	f.runs.advanceErr = nil
	f.expectDispatchTo("kcat", nil, nil)
	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED"))
	require.Equal(t, 1, f.runs.run.CurrentPluginIndex)
}

func TestSweeperMovesATimedOutPluginOn(t *testing.T) {
	f := newSequenceFixture(t)
	cutoff := time.Now().Add(-evaluationTimeout)
	f.runs.run.PluginStartedAt = cutoff.Add(-time.Minute)
	f.expectDispatchTo("kcat", nil, nil)

	require.NoError(t, f.con.sweepStaleEvaluations(cutoff))

	require.Equal(t, models.EvaluationOngoing, f.runs.run.State)
	require.Equal(t, 1, f.runs.run.CurrentPluginIndex)
	require.Len(t, f.runs.run.PluginErrors, 1)
	require.Contains(t, f.runs.run.PluginErrors[0], "mock: timed out")
	require.Equal(t, enum.JobStateOnGoing, f.job.State)

	require.NoError(t, f.replyFrom(t, "mock", "COMPLETED")) // late reply from the timed-out plugin
	require.Zero(t, f.results.writes)
}
