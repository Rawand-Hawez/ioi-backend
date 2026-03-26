package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestListRoles(t *testing.T) {
	token := testToken

	req := httptest.NewRequest("GET", "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result, "data")
	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 12)

	for _, item := range data {
		role := item.(map[string]interface{})
		assert.Contains(t, role, "id")
		assert.Contains(t, role, "code")
		assert.Contains(t, role, "name")
	}
}

func TestListPermissionsWithFilter(t *testing.T) {
	token := testToken

	req := httptest.NewRequest("GET", "/api/v1/permissions?module=inventory", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result, "data")
	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)

	for _, item := range data {
		perm := item.(map[string]interface{})
		assert.Equal(t, "inventory", perm["module"])
	}
}

func TestListRolePermissions(t *testing.T) {
	token := testToken

	systemAdminRoleID := GetRoleIDByCode(t, testApp, token, "system_admin")

	req := httptest.NewRequest("GET", "/api/v1/roles/"+systemAdminRoleID+"/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result, "data")
	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 29)

	for _, item := range data {
		perm := item.(map[string]interface{})
		assert.Contains(t, perm, "id")
		assert.Contains(t, perm, "key")
	}
}

func TestListRolePermissionsNotFound(t *testing.T) {
	token := testToken

	req := httptest.NewRequest("GET", "/api/v1/roles/00000000-0000-0000-0000-000000000000/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAssignRoleToUser(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("ROLENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Role Test Entity")

	testUserID := GetTestUserID(t, token)

	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	DecodeResponse(t, resp, &result)
	assert.Contains(t, result, "id")
	assert.Equal(t, testUserID, result["user_id"])
	assert.Equal(t, reportingUserRoleID, result["role_id"])
	assert.Equal(t, "business_entity", result["scope_type"])
}

func TestAssignRoleToUserDuplicate(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("DUPEENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Duplicate Test Entity")

	testUserID := GetTestUserID(t, token)
	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp1 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 201, resp1.StatusCode)

	resp2 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 409, resp2.StatusCode)
}

func TestAssignRoleToUserInvalidScopeType(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("INVALIDENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Invalid Scope Test Entity")

	testUserID := GetTestUserID(t, token)
	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "invalid_scope_type",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestListUserRoleAssignments(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("LISTENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "List Assignments Entity")

	testUserID := GetTestUserID(t, token)
	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 201, resp.StatusCode)

	req := httptest.NewRequest("GET", "/api/v1/users/"+testUserID+"/role-assignments", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp2, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	assert.Contains(t, result, "data")
	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)

	foundAssignment := false
	for _, item := range data {
		assignment := item.(map[string]interface{})
		if assignment["role_id"] == reportingUserRoleID {
			foundAssignment = true
			assert.Equal(t, "business_entity", assignment["scope_type"])
		}
	}
	assert.True(t, foundAssignment, "Should find the assigned role in the list")
}

func TestRemoveRoleFromUser(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("REMOVEENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "Remove Test Entity")

	testUserID := GetTestUserID(t, token)
	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 201, resp.StatusCode)

	var assignment map[string]interface{}
	DecodeResponse(t, resp, &assignment)
	assignmentID := assignment["id"].(string)

	req := httptest.NewRequest("DELETE", "/api/v1/user-role-assignments/"+assignmentID, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp2, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 204, resp2.StatusCode)

	req2 := httptest.NewRequest("GET", "/api/v1/user-role-assignments/"+assignmentID, nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp3, err := testApp.Test(req2, -1)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp3.StatusCode)
}

func TestRemoveRoleFromUserNotFound(t *testing.T) {
	token := testToken

	req := httptest.NewRequest("DELETE", "/api/v1/user-role-assignments/00000000-0000-0000-0000-000000000000", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetMyPermissions(t *testing.T) {
	token := testToken
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("MYPERMENTITY_%d", timestamp)
	entityID := CreateTestBusinessEntity(t, testApp, token, entityCode, "My Permissions Entity")

	testUserID := GetTestUserID(t, token)
	reportingUserRoleID := GetRoleIDByCode(t, testApp, token, "reporting_user")

	body := map[string]interface{}{
		"role_id":    reportingUserRoleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/users/"+testUserID+"/role-assignments", token, body)
	assert.Equal(t, 201, resp.StatusCode)

	req := httptest.NewRequest("GET", "/api/v1/me/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp2, err := testApp.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	assert.Contains(t, result, "data")
	data := result["data"].([]interface{})

	hasInventoryView := false
	for _, item := range data {
		perm := item.(map[string]interface{})
		if perm["key"] == "inventory.unit.view" {
			hasInventoryView = true
			assert.Contains(t, perm, "scope_type")
			assert.Contains(t, perm, "scope_id")
		}
	}
	assert.True(t, hasInventoryView, "Should have inventory.unit.view permission from reporting_user role")
}

func GetRoleIDByCode(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, code string) string {
	t.Helper()

	req := httptest.NewRequest("GET", "/api/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to get roles: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Failed to get roles: status=%d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data := result["data"].([]interface{})
	for _, item := range data {
		role := item.(map[string]interface{})
		if role["code"] == code {
			return role["id"].(string)
		}
	}

	t.Fatalf("Role with code %s not found", code)
	return ""
}

func GetTestUserID(t *testing.T, token string) string {
	t.Helper()

	req := httptest.NewRequest("GET", "/api/v1/me/permissions", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to get current user: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if userID, ok := result["user_id"].(string); ok {
		return userID
	}

	return ""
}
