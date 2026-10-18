package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateEvaluationPlugins(t *testing.T) {
	valid := map[string][]string{
		"no plugins":  nil,
		"empty list":  {},
		"mock plugin": {"mock"},
	}
	for name, plugins := range valid {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, validateEvaluationPlugins(plugins))
		})
	}

	invalid := map[string][]string{
		"empty name":        {""},
		"unknown plugin":    {"kcat"},
		"routing wildcard":  {"#"},
		"surrounding space": {" mock"},
		"duplicate plugin":  {"mock", "mock"},
	}
	for name, plugins := range invalid {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, validateEvaluationPlugins(plugins))
		})
	}
}
