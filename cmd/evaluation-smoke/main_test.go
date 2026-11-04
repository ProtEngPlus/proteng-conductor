package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmokeTargetsAreLocal(t *testing.T) {
	require.NoError(t, validateLocalURI("mongodb://127.0.0.1:27018", "mongodb"))
	require.NoError(t, validateLocalURI("amqp://guest:guest@localhost:5674/", "amqp"))
	require.NoError(t, validateLocalURI("mongodb://[::1]:27018", "mongodb"))
	require.Error(t, validateLocalURI("mongodb://example.invalid", "mongodb"))
	require.Error(t, validateLocalURI("mongodb://localhost,example.invalid", "mongodb"))
	require.Error(t, validateLocalURI("https://localhost", "mongodb"))
	require.Error(t, runSmoke("mongodb://example.invalid", "amqp://localhost"))
	require.Error(t, runSmoke("mongodb://localhost", "amqp://example.invalid"))
}

func TestSmokeFixtureMatchesWorkerContract(t *testing.T) {
	job, mutation, candidates, run := newSmokeFixture()
	require.NoError(t, run.Validate())
	require.Equal(t, job.Id, mutation.JobId)
	for i, candidate := range candidates {
		require.Equal(t, job.Id, candidate.JobId)
		require.Equal(t, mutation.Id, candidate.MutationId)
		require.Equal(t, job.UserId, candidate.UserId)
		require.Equal(t, candidate.Id, run.MutationResultIds[i])
	}
	request := evaluationRequest(job, candidates, run)
	require.Equal(t, "mock", request["plugin"])
	require.Equal(t, job.Id.Hex(), request["job_id"])
	require.Equal(t, run.Id.Hex(), request["evaluation_run_id"])
	require.Len(t, request["mutants"], 2)
}
