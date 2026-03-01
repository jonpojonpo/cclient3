package api

import "fmt"

type APIError struct {
	StatusCode int
	Type       string `json:"type"`
	ErrorBody  struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d (%s): %s", e.StatusCode, e.ErrorBody.Type, e.ErrorBody.Message)
}

func (e *APIError) IsRateLimit() bool {
	return e.StatusCode == 429
}

func (e *APIError) IsAuth() bool {
	return e.StatusCode == 401
}

func (e *APIError) IsOverloaded() bool {
	return e.StatusCode == 529
}
