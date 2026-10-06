package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestEvaluationRunValidate(t *testing.T) {
	valid := func() *EvaluationRun {
		return &EvaluationRun{
			JobId: primitive.NewObjectID(), MutationId: primitive.NewObjectID(),
			Plugins: []string{"mock"}, MutationResultIds: []primitive.ObjectID{primitive.NewObjectID()},
		}
	}
	require.NoError(t, valid().Validate())
	var missing *EvaluationRun
	require.Error(t, missing.Validate())
	cases := map[string]func(*EvaluationRun){
		"no job":              func(r *EvaluationRun) { r.JobId = primitive.NilObjectID },
		"no mutation":         func(r *EvaluationRun) { r.MutationId = primitive.NilObjectID },
		"no candidates":       func(r *EvaluationRun) { r.MutationResultIds = nil },
		"zero candidate":      func(r *EvaluationRun) { r.MutationResultIds[0] = primitive.NilObjectID },
		"duplicate candidate": func(r *EvaluationRun) { r.MutationResultIds = append(r.MutationResultIds, r.MutationResultIds[0]) },
		"no plugins":          func(r *EvaluationRun) { r.Plugins = nil },
		"empty plugin":        func(r *EvaluationRun) { r.Plugins[0] = " " },
		"multiple plugins":    func(r *EvaluationRun) { r.Plugins = append(r.Plugins, "kcat") },
		"wrong index":         func(r *EvaluationRun) { r.CurrentPluginIndex = 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) { r := valid(); mutate(r); require.Error(t, r.Validate()) })
	}
}
