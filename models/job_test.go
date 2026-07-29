package models

import (
	"testing"

	"github.com/protengplus/proteng-conductor/internal/validator"

	"github.com/stretchr/testify/assert"
)

func TestValidateLabResult(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		lr := LabResult{
			Total:     3,
			Names:     []string{"a", "b", "c"},
			Sequences: []string{"a", "b", "c"},
			Scores:    []float32{1.0, 2.0, 3.0},
		}
		err := lr.Validate()
		assert.Nil(t, err)
	})

	t.Run("invalid length mismatch", func(t *testing.T) {
		lr := LabResult{
			Total:     3,
			Names:     []string{"a", "b", "c"},
			Sequences: []string{"a", "b", "c"},
			Scores:    []float32{1.0, 2.0},
		}
		err := lr.Validate()
		assert.NotNil(t, err)
	})

	t.Run("invalid total", func(t *testing.T) {
		lr := LabResult{
			Total:     2,
			Names:     []string{"a", "b", "c"},
			Sequences: []string{"a", "b", "c"},
			Scores:    []float32{1.0, 2.0, 3.0},
		}
		err := lr.Validate()
		assert.NotNil(t, err)
	})
}

func TestValidateJob(t *testing.T) {
	validator.Init()

	t.Run("valid", func(t *testing.T) {
		job := Job{
			Name:         "test",
			StageId:      1,
			UserId:       "test",
			LabResult:    LabResult{Total: 3, Names: []string{"a", "b", "c"}, Sequences: []string{"a", "b", "c"}, Scores: []float32{1.0, 2.0, 3.0}},
			Options:      map[string]interface{}{"a": 1},
			Artifacts:    map[string]Artifact{"a": {BucketName: "test", Path: "test", Url: "test"}},
			Meta:         []string{"test"},
			InputProtein: "test",
			RunType:      "auto",
		}
		err := job.Validate(true)
		assert.Nil(t, err)
	})

	t.Run("invalid lab result", func(t *testing.T) {
		job := Job{
			Name:         "test",
			StageId:      1,
			UserId:       "test",
			Options:      map[string]interface{}{"a": 1},
			InputProtein: "test",
		}
		err := job.Validate(true)
		assert.NotNil(t, err)
	})

	t.Run("invalid name", func(t *testing.T) {
		job := Job{
			StageId:      1,
			UserId:       "test",
			Options:      map[string]interface{}{"a": 1},
			InputProtein: "test",
		}
		err := job.Validate(false)
		assert.NotNil(t, err)
	})

	t.Run("invalid stage id", func(t *testing.T) {
		job := Job{
			StageId:      4,
			Name:         "test",
			UserId:       "test",
			Options:      map[string]interface{}{"a": 1},
			InputProtein: "test",
		}
		err := job.Validate(false)
		assert.NotNil(t, err)
	})

	t.Run("invalid user id", func(t *testing.T) {
		job := Job{
			StageId:      1,
			Name:         "test",
			Options:      map[string]interface{}{"a": 1},
			InputProtein: "test",
		}
		err := job.Validate(false)
		assert.NotNil(t, err)
	})

	t.Run("invalid input protein", func(t *testing.T) {
		job := Job{
			StageId: 1,
			Name:    "test",
			UserId:  "test",
			Options: map[string]interface{}{"a": 1},
		}
		err := job.Validate(false)
		assert.NotNil(t, err)
	})
}
