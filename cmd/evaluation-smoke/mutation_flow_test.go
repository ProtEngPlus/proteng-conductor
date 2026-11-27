package main

import (
	"encoding/json"
	"testing"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
)

func TestMutationFlowFixtureWaitsForMutation(t *testing.T) {
	job, mutation := newMutationFlowFixture()
	require.Equal(t, 3, job.StageId)
	require.Equal(t, enum.JobStateOnGoing, job.State)
	require.Equal(t, "auto", job.RunType)
	require.Equal(t, []string{"mock"}, job.EvaluationPlugins)
	require.NotNil(t, job.RunTime)
	require.Equal(t, job.Id, mutation.JobId)
	require.Equal(t, 1, mutation.RunId)
	require.Equal(t, enum.MutationStateOnGoing, mutation.State)
}

func TestMutationFinishedMessageMatchesWorkerPayload(t *testing.T) {
	job, mutation := newMutationFlowFixture()
	body, err := mutationFinishedMessage(job, mutation)
	require.NoError(t, err)

	var payload conductor.Payload
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, job.Id.Hex(), payload.Data.JobID)
	require.Equal(t, mutation.Id.Hex(), payload.Data.MutationID)
	require.Equal(t, 3, payload.Data.StageID)
	require.Equal(t, "COMPLETED", payload.Data.Status)
	require.Len(t, payload.Data.MutationResult, 2)
	for sequence := range payload.Data.MutationResult {
		require.Len(t, sequence, len(job.InputProtein)) // substitutions only, like the worker
	}
}
