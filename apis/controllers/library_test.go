package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
)

func TestDedupeResults(t *testing.T) {
	now := time.Now()
	older := &models.ExperimentalResult{Id: primitive.NewObjectID(), ProteinSequence: "mkv", ActualAssayScore: 1, MeasuredAt: now.Add(-time.Hour)}
	newer := &models.ExperimentalResult{Id: primitive.NewObjectID(), ProteinSequence: "MKV", ActualAssayScore: 2, MeasuredAt: now}
	other := &models.ExperimentalResult{Id: primitive.NewObjectID(), ProteinSequence: "AAA", ActualAssayScore: 3, MeasuredAt: now.Add(-time.Minute)}
	empty := &models.ExperimentalResult{Id: primitive.NewObjectID(), MeasuredAt: now}

	kept := dedupeResults([]*models.ExperimentalResult{older, other, newer, empty})

	assert.Equal(t, []*models.ExperimentalResult{newer, other}, kept)
}

func TestExportFileName(t *testing.T) {
	assert.Equal(t, "My_job_Fluorescence_1.csv", exportFileName("My job", "Fluorescence 1"))
	assert.Equal(t, "My_job.csv", exportFileName("My job", ""))
	assert.Equal(t, "Library_Yield.csv", exportFileName("Library", "Yield"))
	assert.Equal(t, 100+len(".csv"), len(exportFileName(strings.Repeat("a", 150), "x")))
}

func TestWriteResultsCsv(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeResultsCsv(c, "a.csv", []*models.ExperimentalResult{
		{ProteinSequence: "MKV", ActualAssayScore: 1.5},
		{ProteinSequence: "AAA", ActualAssayScore: 0.1},
		{ProteinSequence: "CCC", ActualAssayScore: 3},
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "MKV,1.5\nAAA,0.1\nCCC,3\n", w.Body.String())
	assert.Equal(t, "3", w.Header().Get("X-Row-Count"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), `filename="a.csv"`)
}
