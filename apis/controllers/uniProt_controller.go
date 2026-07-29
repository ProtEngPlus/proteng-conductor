package controllers

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/storage"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
	"io"
	"net/http"
	"strings"
)

type UniProtController struct {
	storageService storage.StorageService
}

func NewUniProtController(storageService storage.StorageService) *UniProtController {
	return &UniProtController{storageService: storageService}
}

func (up *UniProtController) GetProteinSequenceFromId(c *gin.Context) {
	uniProtId := c.Param("uniProtId")

	url := fmt.Sprintf("https://www.uniprot.org/uniprot/%s.fasta", uniProtId)

	// Create new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
		return
	}

	// Fetch new request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err, "error: cannot fetch sequence with this id")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		apiutil.ApiResponseInternalServerError(c, err, "error: cannot fetch sequence with this id")
		return
	}

	// Decode the sequence
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err, "error: cannot retrieve body from response")
		return
	}

	reader := io.NopCloser(bytes.NewReader(bodyBytes))

	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err, "error: cannot retrieve body from response")
		return
	}
	defer gzipReader.Close()

	bodyBytes, err = io.ReadAll(gzipReader)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err, "error: cannot retrieve body from response")
		return
	}

	rawString := string(bodyBytes)
	result := formatSequence(rawString)

	sequence := models.UniProtSequence{
		Sequence: result,
	}

	apiutil.ApiResponseOk(c, sequence)
}

func formatSequence(fasta string) string {
	var sequence strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(fasta))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, ">") {
			continue
		}

		sequence.WriteString(line)
	}
	return sequence.String()
}
