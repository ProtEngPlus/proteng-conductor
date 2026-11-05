package controllers

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/protengplus/proteng-conductor/models"
)

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func dedupeResults(results []*models.ExperimentalResult) []*models.ExperimentalResult {
	sorted := make([]*models.ExperimentalResult, 0, len(results))
	for _, result := range results {
		if result.ProteinSequence != "" {
			sorted = append(sorted, result)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].MeasuredAt.After(sorted[j].MeasuredAt)
	})

	seen := map[string]bool{}
	kept := make([]*models.ExperimentalResult, 0, len(sorted))
	for _, result := range sorted {
		key := strings.ToUpper(result.ProteinSequence)
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, result)
	}
	return kept
}

func safeName(name string) string {
	name = unsafeNameChars.ReplaceAllString(name, "_")
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}

func exportFileName(prefix string, assayTypeName string) string {
	if assayTypeName == "" {
		return safeName(prefix) + ".csv"
	}
	return safeName(prefix+"_"+assayTypeName) + ".csv"
}

func writeResultsCsv(c *gin.Context, fileName string, results []*models.ExperimentalResult) {
	var buffer bytes.Buffer
	for _, result := range results {
		buffer.WriteString(result.ProteinSequence)
		buffer.WriteByte(',')
		buffer.WriteString(strconv.FormatFloat(float64(result.ActualAssayScore), 'g', -1, 32))
		buffer.WriteByte('\n')
	}

	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Header("X-Row-Count", strconv.Itoa(len(results)))
	c.Data(200, "text/csv; charset=utf-8", buffer.Bytes())
}

func splitIds(value string) []string {
	ids := []string{}
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func objectIdsToHex(ids []primitive.ObjectID) []string {
	hex := make([]string, len(ids))
	for i, id := range ids {
		hex[i] = id.Hex()
	}
	return hex
}
