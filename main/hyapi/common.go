package hyapi

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	"resty.dev/v3"
)

// APIResponse represents the structure of the outer API response
type APIResponse struct {
	RetCode int                        `json:"retcode"` // Return code
	Message string                     `json:"message"` // Message
	Data    map[string]json.RawMessage `json:"data"`    // Data
}

func getApiResponse(client *resty.Client, hostname, api, launcherID, language string) APIResponse {
	// Build the URL
	url := fmt.Sprintf("https://%s/hyp/hyp-connect/api/%s?launcher_id=%s&language=%s",
		hostname, api, launcherID, language)

	// Make the API call
	var apiResponse APIResponse
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		Get(url)
	// Handle errors
	if err != nil {
		log.Panic().Err(err).Str("url", url).Msg("failed to fetch API")
	}

	// Manually unmarshal the JSON response body
	err = json.Unmarshal(resp.Bytes(), &apiResponse)
	if err != nil {
		log.Panic().Err(err).Str("url", url).Msg("failed to unmarshal API response")
	}
	// Handle errors
	if err != nil {
		log.Panic().Err(err).Str("url", url).Msg("failed to fetch API")
	}

	return apiResponse
}

func unmarshalNestedApiData[T any](rawResponse APIResponse, nestedKey string) T {
	// Unmarshal the nested key into the generic type T
	var result T
	err := json.Unmarshal(rawResponse.Data[nestedKey], &result)
	if err != nil {
		log.Panic().Err(err).Str("nestedKey", nestedKey).Msg("failed to unmarshal API response data")
	}

	return result
}

// callAPIWithClient fetches data from the API using the provided client and unpacks the response
func callAPIWithClient[T any](client *resty.Client, hostname, api, launcherID, language string, nestedKey string) T {
	// Make the API call and automatically deserialize the result into APIResponse struct
	apiResponse := getApiResponse(client, hostname, api, launcherID, language)

	// Unmarshal the nested data
	return unmarshalNestedApiData[T](apiResponse, nestedKey)
}

// callAPI initializes a Resty client, calls the API, and unpacks the response
func callAPI[T any](hostname, api, launcherID, language string, nestedKey string) T {
	// Initialize Resty client
	client := resty.New()
	defer client.Close()

	// Delegate to callAPIWithClient
	return callAPIWithClient[T](client, hostname, api, launcherID, language, nestedKey)
}
