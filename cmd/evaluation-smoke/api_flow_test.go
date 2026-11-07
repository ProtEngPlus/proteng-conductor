package main

import (
	"testing"

	"github.com/protengplus/proteng-conductor/models/enum"
	"github.com/stretchr/testify/require"
)

func TestAPIFlowFixtureIsACompletedJob(t *testing.T) {
	job, mutation, candidates := newAPIFlowFixture()
	require.Equal(t, enum.JobStateCompleted, job.State)
	require.Equal(t, 3, job.StageId)
	require.Equal(t, job.Id, mutation.JobId)
	require.Equal(t, enum.MutationStateCompleted, mutation.State)
	require.Len(t, candidates, 2)
	for _, candidate := range candidates {
		require.Equal(t, job.Id, candidate.JobId)
		require.Equal(t, mutation.Id, candidate.MutationId)
		require.Equal(t, job.UserId, candidate.UserId)
	}
}

func TestAPIFlowRequiresLocalServices(t *testing.T) {
	require.Error(t, runAPIFlowSmoke("mongodb://example.invalid", "amqp://localhost"))
	require.Error(t, runAPIFlowSmoke("mongodb://localhost", "amqp://example.invalid"))
}
