// backend/tests/helpers_test.go

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// testApp and testToken are initialized in main_test.go

// CreateTestBusinessEntity creates a business entity for test setup
// Returns the created entity's ID
func CreateTestBusinessEntity(t *testing.T, app *fiber.App, token, code, name string) string {
	t.Helper()

	body := map[string]interface{}{
		"code":             code,
		"name":             name,
		"default_currency": "USD",
		"status":           "active",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/business-entities", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test business entity: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test business entity: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if id, ok := result["id"].(string); ok {
		return id
	}
	t.Fatalf("Response missing id field")
	return ""
}

// MakeAuthenticatedRequest is a helper for making authenticated requests
func MakeAuthenticatedRequest(t *testing.T, app *fiber.App, method, path, token string, body interface{}) *http.Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// DecodeResponse decodes a JSON response into the provided target
func DecodeResponse(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}
