package conductor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func (c *Conductor) StartPipelineComponent(stageId int, request PipelineRequest) error {

	reqBody, err := json.Marshal(request)

	if err != nil {
		return err
	}

	steps := []string{"SEQUENCER", "EVOTUNE", "FIT_TOP", "MUTATION"}

	req, err := http.NewRequest("POST", os.Getenv(steps[stageId]+"_URL"), bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("error: %s", resp.Status)
	}

	return nil
}
