package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPartyCRUD(t *testing.T) {
	token := testToken
	uniqueCode := fmt.Sprintf("PARTY_%d", time.Now().UnixNano())
	var partyID string

	t.Run("create party", func(t *testing.T) {
		body := map[string]interface{}{
			"party_type":    "person",
			"party_code":    uniqueCode,
			"display_name":  "John Doe",
			"full_name":     "John Michael Doe",
			"first_name":    "John",
			"last_name":     "Doe",
			"primary_phone": "+1234567890",
			"primary_email": "john.doe@example.com",
			"nationality":   "US",
			"status":        "active",
			"notes":         "Test party",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/parties", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.NotEmpty(t, result["id"])
		assert.Equal(t, "person", result["party_type"])
		assert.Equal(t, uniqueCode, result["party_code"])
		assert.Equal(t, "John Doe", result["display_name"])
		partyID = result["id"].(string)
	})

	t.Run("get party by id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties/"+partyID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, partyID, result["id"])
		assert.Equal(t, "John Doe", result["display_name"])
	})

	t.Run("update party", func(t *testing.T) {
		body := map[string]interface{}{
			"display_name":  "John M. Doe",
			"primary_phone": "+0987654321",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("PATCH", "/api/v1/parties/"+partyID, bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "John M. Doe", result["display_name"])
	})

	t.Run("list parties includes created party", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?page=1&per_page=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})
}

func TestPartyValidation(t *testing.T) {
	token := testToken

	t.Run("create party without required fields", func(t *testing.T) {
		body := map[string]interface{}{
			"full_name": "Missing Required Fields",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/parties", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("create party without party_type", func(t *testing.T) {
		body := map[string]interface{}{
			"party_code":    fmt.Sprintf("NOTYPE_%d", time.Now().UnixNano()),
			"display_name":  "No Type",
			"primary_phone": "+1234567890",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/parties", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("create party without primary_phone", func(t *testing.T) {
		body := map[string]interface{}{
			"party_type":   "person",
			"party_code":   fmt.Sprintf("NOPHONE_%d", time.Now().UnixNano()),
			"display_name": "No Phone",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/parties", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestPartyFiltering(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	personCode := fmt.Sprintf("PERSON_%d", timestamp)
	orgCode := fmt.Sprintf("ORG_%d", timestamp)
	inactiveCode := fmt.Sprintf("INACTIVE_%d", timestamp)

	personID := CreateTestParty(t, testApp, token, personCode, "Person Party", "person", "active")
	_ = CreateTestParty(t, testApp, token, orgCode, "Organization Party", "organization", "active")
	_ = CreateTestParty(t, testApp, token, inactiveCode, "Inactive Party", "person", "inactive")

	t.Run("filter by party_type=person", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?party_type=person&page=1&per_page=50", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		data := result["data"].([]interface{})
		for _, item := range data {
			party := item.(map[string]interface{})
			assert.Equal(t, "person", party["party_type"])
		}
	})

	t.Run("filter by party_type=organization", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?party_type=organization&page=1&per_page=50", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
		for _, item := range data {
			party := item.(map[string]interface{})
			assert.Equal(t, "organization", party["party_type"])
		}
	})

	t.Run("filter by status=active", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?status=active&page=1&per_page=50", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		data := result["data"].([]interface{})
		for _, item := range data {
			party := item.(map[string]interface{})
			assert.Equal(t, "active", party["status"])
		}
	})

	t.Run("filter by status=inactive", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?status=inactive&page=1&per_page=50", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
		for _, item := range data {
			party := item.(map[string]interface{})
			assert.Equal(t, "inactive", party["status"])
		}
	})

	t.Run("combined filter by party_type and status", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/parties?party_type=person&status=active&page=1&per_page=50", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		data := result["data"].([]interface{})
		for _, item := range data {
			party := item.(map[string]interface{})
			assert.Equal(t, "person", party["party_type"])
			assert.Equal(t, "active", party["status"])
		}
	})

	_ = personID
}

func TestUnitOwnershipCRUD(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("OWNENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Ownership Test Entity")

	branchCode := fmt.Sprintf("OWNBRANCH_%d", timestamp)
	branchID := CreateTestBranch(t, testApp, token, entityID, branchCode, "Ownership Test Branch")

	projectCode := fmt.Sprintf("OWNPROJ_%d", timestamp)
	projectID := CreateTestProject(t, testApp, token, entityID, branchID, projectCode, "Ownership Test Project")

	unitCode := fmt.Sprintf("OWNUNIT_%d", timestamp)
	unitID := CreateTestUnit(t, testApp, token, entityID, branchID, projectID, unitCode)

	partyCode := fmt.Sprintf("OWNPARTY_%d", timestamp)
	partyID := CreateTestParty(t, testApp, token, partyCode, "Owner Party", "person", "active")

	var ownershipID string

	t.Run("create unit ownership", func(t *testing.T) {
		body := map[string]interface{}{
			"party_id":         partyID,
			"share_percentage": "100.00",
			"effective_from":   "2024-01-01",
			"status":           "active",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/units/"+unitID+"/ownerships", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.NotEmpty(t, result["id"])
		ownershipID = result["id"].(string)
	})

	t.Run("list unit ownerships includes created ownership", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/units/"+unitID+"/ownerships", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
	})

	t.Run("close unit ownership", func(t *testing.T) {
		body := map[string]interface{}{
			"effective_to": "2024-12-31",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/unit-ownerships/"+ownershipID+"/close", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 204, resp.StatusCode)
	})
}

func TestResponsibilityAssignmentCRUD(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("RESPENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Responsibility Test Entity")

	branchCode := fmt.Sprintf("RESPBRANCH_%d", timestamp)
	branchID := CreateTestBranch(t, testApp, token, entityID, branchCode, "Responsibility Test Branch")

	projectCode := fmt.Sprintf("RESPPROJ_%d", timestamp)
	projectID := CreateTestProject(t, testApp, token, entityID, branchID, projectCode, "Responsibility Test Project")

	unitCode := fmt.Sprintf("RESPUNIT_%d", timestamp)
	unitID := CreateTestUnit(t, testApp, token, entityID, branchID, projectID, unitCode)

	partyCode := fmt.Sprintf("RESPPARTY_%d", timestamp)
	partyID := CreateTestParty(t, testApp, token, partyCode, "Responsible Party", "person", "active")

	var assignmentID string

	t.Run("create responsibility assignment", func(t *testing.T) {
		body := map[string]interface{}{
			"party_id":            partyID,
			"responsibility_type": "electricity",
			"effective_from":      "2024-01-01",
			"status":              "active",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/units/"+unitID+"/responsibilities", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.NotEmpty(t, result["id"])
		assignmentID = result["id"].(string)
	})

	t.Run("list responsibility assignments includes created assignment", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/units/"+unitID+"/responsibilities", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
	})

	t.Run("close responsibility assignment", func(t *testing.T) {
		body := map[string]interface{}{
			"effective_to": "2024-12-31",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/responsibility-assignments/"+assignmentID+"/close", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 204, resp.StatusCode)
	})
}

func CreateTestParty(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, code, displayName, partyType, status string) string {
	t.Helper()

	body := map[string]interface{}{
		"party_type":    partyType,
		"party_code":    code,
		"display_name":  displayName,
		"primary_phone": "+1234567890",
		"status":        status,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/parties", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test party: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create test party: status=%d", resp.StatusCode)
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

func CreateTestBranch(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, code, name string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id": entityID,
		"code":               code,
		"name":               name,
		"display_name":       name,
		"country":            "IQ",
		"status":             "active",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/branches", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test branch: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create test branch: status=%d", resp.StatusCode)
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

func CreateTestProject(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, code, name string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id": entityID,
		"primary_branch_id":  branchID,
		"code":               code,
		"name":               name,
		"project_type":       "residential",
		"structure_type":     "tower",
		"country":            "IQ",
		"status":             "active",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/projects", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create test project: status=%d", resp.StatusCode)
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

func CreateTestUnit(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, projectID, code string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id":     entityID,
		"branch_id":              branchID,
		"project_id":             projectID,
		"unit_type":              "apartment",
		"commercial_disposition": "sale_only",
		"unit_code":              code,
		"inventory_status":       "available",
		"sales_status":           "available",
		"occupancy_status":       "vacant",
		"maintenance_status":     "none",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/units", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test unit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create test unit: status=%d", resp.StatusCode)
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
