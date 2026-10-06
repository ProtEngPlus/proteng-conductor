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

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/repositories/mock_repository"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testProtein = "MSIQFFRVALIPFFAAFCLPVFAHPETLVKVKDAEDQLGARVGYIEL" // 47 residues

func jsonOptions(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var options map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &options))
	return options
}

func mutationJSON(extra string) string {
	base := `"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5,
		"num_mutations_low": 1, "num_mutations_high": 3, "amino_acid_set": "20 standard"`
	if extra == "" {
		return "{" + base + "}"
	}
	return "{" + base + ", " + extra + "}"
}

func TestValidateServiceOptions_Mutation_Accepts(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"two regions":                    mutationJSON(`"mutate_regions": [[3, 10], [18, 25]]`),
		"no regions field":               mutationJSON(``),
		"empty regions":                  mutationJSON(`"mutate_regions": []`),
		"regions in any order":           mutationJSON(`"mutate_regions": [[18, 25], [3, 10]]`),
		"regions that only touch":        mutationJSON(`"mutate_regions": [[3, 10], [11, 15]]`),
		"first position only":            `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "num_mutations_low": 1, "num_mutations_high": 1, "amino_acid_set": "20 standard", "mutate_regions": [[1, 1]]}`,
		"up to the last position":        mutationJSON(`"mutate_regions": [[40, 47]]`),
		"single region, one trajectory":  `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 1, "num_mutations_low": 1, "num_mutations_high": 3, "amino_acid_set": "20 standard", "mutate_regions": [[3, 10]]}`,
		"fewest trajectories for two":    `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 3, "num_mutations_low": 1, "num_mutations_high": 3, "amino_acid_set": "20 standard", "mutate_regions": [[3, 10], [18, 25]]}`,
		"amino acid set with U":          `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "num_mutations_low": 1, "num_mutations_high": 3, "amino_acid_set": "20 standard + U"}`,
		"exact number of mutations":      `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "num_mutations_low": 2, "num_mutations_high": 2, "amino_acid_set": "20 standard"}`,
		"leftover mutate_pos_range":      mutationJSON(`"mutate_pos_range": 5`),
		"ten regions with eleven trajec": `{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 11, "num_mutations_low": 1, "num_mutations_high": 3, "amino_acid_set": "20 standard", "mutate_regions": [[1, 2], [4, 5], [7, 8], [10, 11], [13, 14], [16, 17], [19, 20], [22, 23], [25, 26], [28, 29]]}`,
	}

	for name, raw := range tests {
		raw := raw
		t.Run(name, func(tt *testing.T) {
			err := validateServiceOptions("mutation", jsonOptions(tt, raw), testProtein)
			assert.NoError(tt, err)
		})
	}
}

func TestValidateServiceOptions_Mutation_Rejects(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		options string
		reason  string
	}{
		"old style options without the new fields": {
			`{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "mutate_pos_range": 5}`,
			"num_mutations_low",
		},
		"missing amino_acid_set": {
			`{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "num_mutations_low": 1, "num_mutations_high": 3}`,
			"amino_acid_set",
		},
		"unknown amino_acid_set":             {mutationJSON(`"amino_acid_set": "everything"`), "amino_acid_set"},
		"low below one":                      {mutationJSON(`"num_mutations_low": 0`), "num_mutations_low"},
		"low is not an integer":              {mutationJSON(`"num_mutations_low": 1.5`), "num_mutations_low"},
		"high below low":                     {mutationJSON(`"num_mutations_low": 3, "num_mutations_high": 2`), "must be at least num_mutations_low"},
		"no trajectories":                    {mutationJSON(`"num_trajectories": 0`), "num_trajectories"},
		"region with three numbers":          {mutationJSON(`"mutate_regions": [[3, 10, 12]]`), "mutate_regions"},
		"region that is not an array":        {mutationJSON(`"mutate_regions": [3, 10]`), "mutate_regions"},
		"regions is null":                    {mutationJSON(`"mutate_regions": null`), "mutate_regions"},
		"more than ten regions":              {mutationJSON(`"num_trajectories": 12, "mutate_regions": [[1, 2], [4, 5], [7, 8], [10, 11], [13, 14], [16, 17], [19, 20], [22, 23], [25, 26], [28, 29], [31, 32]]`), "mutate_regions"},
		"region starting at zero":            {mutationJSON(`"mutate_regions": [[0, 5]]`), "mutate_regions"},
		"region past the protein":            {mutationJSON(`"mutate_regions": [[40, 48]]`), "does not fit"},
		"region ending before it starts":     {mutationJSON(`"mutate_regions": [[10, 3]]`), "does not fit"},
		"overlapping regions":                {mutationJSON(`"mutate_regions": [[3, 10], [8, 15]]`), "overlap"},
		"regions sharing one position":       {mutationJSON(`"mutate_regions": [[3, 10], [10, 15]]`), "overlap"},
		"overlap found when out of order":    {mutationJSON(`"mutate_regions": [[18, 25], [3, 10], [20, 30]]`), "overlap"},
		"more mutations than the region has": {mutationJSON(`"mutate_regions": [[3, 4]]`), "positions that can mutate"},
		"a region smaller than the minimum": {
			`{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "num_mutations_low": 3, "num_mutations_high": 3, "amino_acid_set": "20 standard", "mutate_regions": [[3, 4], [10, 20]]}`,
			"fewer than num_mutations_low",
		},
		"two regions with two trajectories": {mutationJSON(`"num_trajectories": 2, "mutate_regions": [[3, 10], [18, 25]]`), "num_trajectories"},
		"three regions with three trajectories": {
			mutationJSON(`"num_trajectories": 3, "mutate_regions": [[3, 5], [10, 12], [20, 22]]`),
			"num_trajectories (3) must be at least 4 for 3 mutate_regions",
		},
		"options that are not an object": {`null`, "Expected: object"},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(tt *testing.T) {
			var options interface{}
			require.NoError(tt, json.Unmarshal([]byte(tc.options), &options))
			err := validateServiceOptions("mutation", options, testProtein)
			require.Error(tt, err)
			assert.Contains(tt, err.Error(), tc.reason)
		})
	}
}

func TestValidateServiceOptions_ShortProtein(t *testing.T) {
	t.Parallel()

	err := validateServiceOptions("mutation", jsonOptions(t, mutationJSON(``)), "MK")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positions that can mutate")
}

func TestValidateServiceOptions_OtherServicesAreUnchanged(t *testing.T) {
	t.Parallel()

	blast := `{"program": "blastp", "database": "nr", "hitlist_size": 50, "expect": 10, "perc_ident": 85, "random_state": 50, "seq_length": 300, "hsp_cov": 1}`
	assert.NoError(t, validateServiceOptions("blast", jsonOptions(t, blast), testProtein))

	err := validateServiceOptions("blast", jsonOptions(t, `{"program": "blastp"}`), testProtein)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database")

	assert.NoError(t, validateServiceOptions("unknown-service", jsonOptions(t, `{"anything": 1}`), testProtein))
}

func TestValidateJobAndConfigurationOptions_UseTheirOwnProtein(t *testing.T) {
	t.Parallel()

	options := map[string]interface{}{
		"mutation": jsonOptions(t, mutationJSON(`"mutate_regions": [[40, 47]]`)),
	}

	job := models.Job{Meta: []string{"mutation"}, Options: options, InputProtein: testProtein}
	assert.NoError(t, validateJobOptions(&job))
	job.InputProtein = "MSIQFFRVAL" // now region 40-47 lies past the end
	assert.ErrorContains(t, validateJobOptions(&job), "does not fit")

	configuration := models.Configuration{Meta: []string{"mutation"}, Options: options, InputProtein: testProtein}
	assert.NoError(t, validateConfigurationOptions(&configuration))
	configuration.InputProtein = "MSIQFFRVAL"
	assert.ErrorContains(t, validateConfigurationOptions(&configuration), "does not fit")

	job = models.Job{Meta: []string{"mutation"}, Options: map[string]interface{}{}, InputProtein: testProtein}
	assert.ErrorContains(t, validateJobOptions(&job), "missing options for mutation")
}

type fakeConductor struct {
	conductor.Conductor
	ran []*models.Mutation
}

func (f *fakeConductor) RunMutation(mutation *models.Mutation) error {
	f.ran = append(f.ran, mutation)
	return nil
}

type createMutationTest struct {
	controller *MutationController
	jobs       *mock_repository.MockJobRepository
	mutations  *mock_repository.MockMutationRepository
	conductor  *fakeConductor
}

func newCreateMutationTest(t *testing.T) *createMutationTest {
	t.Helper()
	ctrl := gomock.NewController(t)
	tc := &createMutationTest{
		jobs:      mock_repository.NewMockJobRepository(ctrl),
		mutations: mock_repository.NewMockMutationRepository(ctrl),
		conductor: &fakeConductor{},
	}
	results := mock_repository.NewMockMutationResultRepository(ctrl)
	tc.controller = NewMutationController(tc.jobs, tc.mutations, results, tc.conductor)
	return tc
}

func (tc *createMutationTest) post(t *testing.T, jobID string, options string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"name":          "new mutation",
		"job_id":        jobID,
		"input_protein": testProtein,
		"tool":          "mutation",
		"options":       jsonOptions(t, options),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/mutations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	tc.controller.CreateMutation(c)
	return w
}

func TestCreateMutation_ValidOptionsStartTheMutation(t *testing.T) {
	t.Parallel()
	tc := newCreateMutationTest(t)
	jobID := primitive.NewObjectID()

	tc.jobs.EXPECT().FindById(jobID.Hex()).Return(&models.Job{Id: jobID, State: "COMPLETED"}, nil)
	tc.mutations.EXPECT().Create(gomock.Any()).Return(nil)

	w := tc.post(t, jobID.Hex(), mutationJSON(`"mutate_regions": [[3, 10], [18, 25]]`))

	assert.Equal(t, http.StatusOK, w.Code)
	require.Len(t, tc.conductor.ran, 1)
	assert.Equal(t, jobID, tc.conductor.ran[0].JobId)
	assert.Equal(t, [][]interface{}{{float64(3), float64(10)}, {float64(18), float64(25)}}, toPairs(tc.conductor.ran[0].Options["mutate_regions"]))
}

func TestCreateMutation_InvalidOptionsAreRejectedBeforeAnythingIsStored(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		options string
		reason  string
	}{
		"regions overlap":       {mutationJSON(`"mutate_regions": [[3, 10], [8, 15]]`), "overlap"},
		"too few trajectories":  {mutationJSON(`"num_trajectories": 2, "mutate_regions": [[3, 10], [18, 25]]`), "num_trajectories"},
		"region past protein":   {mutationJSON(`"mutate_regions": [[40, 60]]`), "does not fit"},
		"the old options alone": {`{"temperature": 0.01, "num_iterations": 25, "num_trajectories": 5, "mutate_pos_range": 5}`, "num_mutations_low"},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(tt *testing.T) {
			// the mocks have no expectations, so touching a repository fails the test
			test := newCreateMutationTest(tt)

			w := test.post(tt, primitive.NewObjectID().Hex(), tc.options)

			assert.Equal(tt, http.StatusBadRequest, w.Code)
			assert.Contains(tt, w.Body.String(), tc.reason)
			assert.Empty(tt, test.conductor.ran)
		})
	}
}

func toPairs(value interface{}) [][]interface{} {
	pairs := [][]interface{}{}
	for _, pair := range value.([]interface{}) {
		pairs = append(pairs, pair.([]interface{}))
	}
	return pairs
}
