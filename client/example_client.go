package client

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

type ExampleClient struct {
	httpClient *http.Client
}

func (client *ExampleClient) GetWeather() error {
	resp, err := client.httpClient.Get("https://www.metaweather.com/api/location/1047378/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	logrus.Infof("Response: %v", resp)

	return nil
}

func ProvideExampleClient(httpClient *http.Client) *ExampleClient {
	return &ExampleClient{
		httpClient: httpClient,
	}
}
