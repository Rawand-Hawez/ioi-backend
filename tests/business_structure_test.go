// backend/tests/business_structure_test.go

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBusinessEntityCRUD(t *testing.T) {
	token := testToken
	uniqueCode := fmt.Sprintf("ACME_%d", time.Now().UnixNano())

	t.Run("create business entity", func(t *testing.T) {
		body := map[string]interface{}{
			"code":             uniqueCode,
			"name":             "Acme Corporation",
			"display_name":     "Acme Corp",
			"default_currency": "USD",
			"country":          "IQ",
			"registration_no":  "REG123",
			"tax_no":           "TAX456",
			"status":           "active",
			"notes":            "Test entity",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/business-entities", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)
	})

	t.Run("list business entities with pagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/business-entities?page=1&per_page=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("duplicate code returns 409", func(t *testing.T) {
		body := map[string]interface{}{
			"code": uniqueCode,
			"name": "Another Acme",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/business-entities", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/business-entities", nil)
		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/business-entities/invalid-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("non-existent id returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/business-entities/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})
}

func TestBranchCRUD(t *testing.T) {
	token := testToken
	uniqueEntityCode := fmt.Sprintf("BRANCHTEST_%d", time.Now().UnixNano())
	uniqueBranchCode := fmt.Sprintf("HQ_%d", time.Now().UnixNano())

	entityID := CreateTestBusinessEntity(t, testApp, token, uniqueEntityCode, "Branch Test Entity")

	t.Run("create branch", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"code":               uniqueBranchCode,
			"name":               "Headquarters",
			"display_name":       "HQ Office",
			"country":            "IQ",
			"city":               "Erbil",
			"address_text":       "123 Main Street",
			"phone":              "+1234567890",
			"email":              "hq@example.com",
			"status":             "active",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/branches", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)
	})

	t.Run("duplicate branch code per entity returns 409", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"code":               uniqueBranchCode,
			"name":               "Another HQ",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/branches", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("list branches for business entity", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/business-entities/"+entityID+"/branches", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("branch with non-existent business entity returns 400", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": "00000000-0000-0000-0000-000000000000",
			"code":               "TEST",
			"name":               "Test Branch",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/branches", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}
