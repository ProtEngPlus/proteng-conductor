package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/internal/validator"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

func init() {
	// CreateJob validates the job struct with the shared validator
	validator.Init()
}

func TestValidateProteinSequence_Accepts(t *testing.T) {
	t.Parallel()

	for name, sequence := range map[string]string{
		"all 20 standard":  standardAminoAcids,
		"lowercase":        "msiqffrval",
		"single residue":   "M",
		"test protein":     testProtein,
		"mixed case input": "MsIqFf",
	} {
		assert.NoError(t, validateProteinSequence("input protein", sequence), name)
	}
}

func TestValidateProteinSequence_Rejects(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sequence string
		reason   string
	}{
		"selenocysteine":     {"MSIUQ", `"U" at position 4`},
		"lowercase u":        {"msiuq", `"u" at position 4`},
		"pyrrolysine":        {"OMSIQ", `"O" at position 1`},
		"ambiguous residues": {"MSBZX", `"B" at position 3`},
		"unknown residue":    {"MSIQX", `"X" at position 5`},
		"digit":              {"MS1Q", `"1" at position 3`},
		"whitespace":         {"MSI Q", `" " at position 4`},
		"stop codon":         {"MSIQ*", `"*" at position 5`},
		"empty":              {"", "is empty"},
	}

	for name, tc := range tests {
		err := validateProteinSequence("input protein", tc.sequence)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.reason, name)
		assert.Contains(t, err.Error(), "input protein", name)
	}
}

func TestValidateSequences(t *testing.T) {
	t.Parallel()

	labResult := func(sequences ...string) models.LabResult {
		return models.LabResult{Total: len(sequences), Sequences: sequences, Scores: make([]float32, len(sequences))}
	}

	assert.NoError(t, validateSequences(testProtein, models.LabResult{}))
	assert.NoError(t, validateSequences(testProtein, labResult("MSIQ", "msiq")))

	err := validateSequences("MSIUQ", labResult("MSIQ"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input protein")

	err = validateSequences(testProtein, labResult("MSIQ", "MSUQ"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lab result sequence 2")
}

func newSequenceTestJobController(t *testing.T) (*JobController, *mock_repository.MockJobRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	jobs := mock_repository.NewMockJobRepository(ctrl)
	controller := NewJobController(
		jobs,
		mock_repository.NewMockMutationRepository(ctrl),
		mock_repository.NewMockMutationResultRepository(ctrl),
		mock_repository.NewMockConfigurationRepository(ctrl),
		mock_repository.NewMockQueryResultRepository(ctrl),
		mock_repository.NewMockExperimentalResultRepository(ctrl),
		mock_repository.NewMockAssayTypeRepository(ctrl),
		&fakeConductor{},
	)
	return controller, jobs
}

func sendJSON(t *testing.T, method string, body interface{}, params gin.Params, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	handler(c)
	return w
}

func jobBody(inputProtein string, labSequences ...string) map[string]interface{} {
	body := map[string]interface{}{
		"name":          "job",
		"user_id":       "user",
		"run_type":      "auto",
		"input_protein": inputProtein,
		"meta":          []string{},
		"options":       map[string]interface{}{},
	}
	if len(labSequences) > 0 {
		body["lab_result"] = map[string]interface{}{
			"total":     len(labSequences),
			"sequences": labSequences,
			"scores":    make([]float32, len(labSequences)),
		}
	}
	return body
}

func TestCreateJob_RejectsNonStandardAminoAcids(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body   map[string]interface{}
		reason string
	}{
		"input protein": {jobBody("MSIUQ"), "input protein"},
		"lab result":    {jobBody(testProtein, "MSIQ", "MSUQ"), "lab result sequence 2"},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(tt *testing.T) {
			// the mocks have no expectations, so touching a repository fails the test
			controller, _ := newSequenceTestJobController(tt)

			w := sendJSON(tt, http.MethodPost, map[string]interface{}{"job": tc.body}, nil, controller.CreateJob)

			assert.Equal(tt, http.StatusBadRequest, w.Code)
			assert.Contains(tt, w.Body.String(), tc.reason)
		})
	}
}

func TestCreateConfigurations_RejectsNonStandardAminoAcids(t *testing.T) {
	t.Parallel()
	controller, _ := newSequenceTestJobController(t)

	body := jobBody("MSIUQ")
	body["ref_job_id"] = primitive.NewObjectID().Hex()
	w := sendJSON(t, http.MethodPost, body, nil, controller.CreateConfigurations)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "input protein")
}

func TestUpdateJob_RejectsNonStandardAminoAcidsInLabResult(t *testing.T) {
	t.Parallel()
	controller, jobs := newSequenceTestJobController(t)
	id := primitive.NewObjectID()
	jobs.EXPECT().FindById(id.Hex()).Return(&models.Job{Id: id, InputProtein: testProtein}, nil)

	body := map[string]interface{}{
		"lab_result": map[string]interface{}{"total": 1, "sequences": []string{"MSUQ"}, "scores": []float32{1}},
	}
	w := sendJSON(t, http.MethodPut, body, gin.Params{{Key: "id", Value: id.Hex()}}, controller.UpdateJob)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "lab result sequence 1")
}

func TestUpdateJob_AcceptsStandardAminoAcids(t *testing.T) {
	t.Parallel()
	controller, jobs := newSequenceTestJobController(t)
	id := primitive.NewObjectID()
	jobs.EXPECT().FindById(id.Hex()).Return(&models.Job{Id: id, InputProtein: testProtein}, nil)
	jobs.EXPECT().Update(id.Hex(), gomock.Any()).Return(nil)

	body := map[string]interface{}{
		"lab_result": map[string]interface{}{"total": 1, "sequences": []string{"MSIQ"}, "scores": []float32{1}},
	}
	w := sendJSON(t, http.MethodPut, body, gin.Params{{Key: "id", Value: id.Hex()}}, controller.UpdateJob)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateMutation_RejectsNonStandardAminoAcids(t *testing.T) {
	t.Parallel()
	// the mocks have no expectations, so touching a repository fails the test
	tc := newCreateMutationTest(t)

	body := map[string]interface{}{
		"name":          "new mutation",
		"job_id":        primitive.NewObjectID().Hex(),
		"input_protein": "MSIUQ",
		"tool":          "mutation",
		"options":       jsonOptions(t, mutationJSON("")),
	}
	w := sendJSON(t, http.MethodPost, body, nil, tc.controller.CreateMutation)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "input protein")
	assert.Empty(t, tc.conductor.ran)
}
