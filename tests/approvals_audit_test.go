package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupApprovalsTestData(t *testing.T) (entityID, branchID string) {
	t.Helper()
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("APPENTITY_%d", timestamp)
	entityID = CreateTestBusinessEntity(t, testApp, testToken, entityCode, "Approvals Test Entity")

	branchCode := fmt.Sprintf("APPBRANCH_%d", timestamp)
	branchID = CreateTestBranch(t, testApp, testToken, entityID, branchCode, "Approvals Test Branch")

	return
}

func AssignRoleToUser(t *testing.T, app *fiber.App, token, entityID, roleCode string) {
	t.Helper()

	userID := GetTestUserID(t, token)
	roleID := GetRoleIDByCode(t, app, token, roleCode)

	body := map[string]interface{}{
		"role_id":    roleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, app, "POST", "/api/v1/users/"+userID+"/role-assignments", token, body)
	// 201 = created, 200 = OK (some implementations), 409 = already exists (acceptable)
	if resp.StatusCode != 201 && resp.StatusCode != 200 && resp.StatusCode != 409 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to assign role %s to user: status=%d body=%s", roleCode, resp.StatusCode, string(respBody))
	}
	resp.Body.Close()
}

func AssignRoleToUserByID(t *testing.T, app *fiber.App, adminToken, userID, entityID, roleCode string) {
	t.Helper()

	roleID := GetRoleIDByCode(t, app, adminToken, roleCode)

	body := map[string]interface{}{
		"role_id":    roleID,
		"scope_type": "business_entity",
		"scope_id":   entityID,
	}

	resp := MakeAuthenticatedRequest(t, app, "POST", "/api/v1/users/"+userID+"/role-assignments", adminToken, body)
	if resp.StatusCode != 201 && resp.StatusCode != 200 && resp.StatusCode != 409 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to assign role %s to user %s: status=%d body=%s", roleCode, userID, resp.StatusCode, string(respBody))
	}
	resp.Body.Close()
}

func CreateTestApprovalPolicy(t *testing.T, app *fiber.App, token, entityID, code, name, module, requestType string, minApprovers int16, preventSelfApproval bool) string {
	t.Helper()

	AssignRoleToUser(t, app, token, entityID, "system_admin")

	body := map[string]interface{}{
		"business_entity_id":    entityID,
		"code":                  code,
		"name":                  name,
		"module":                module,
		"request_type":          requestType,
		"min_approvers":         minApprovers,
		"prevent_self_approval": preventSelfApproval,
		"is_active":             true,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/approval-policies", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test approval policy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test approval policy: status=%d body=%s", resp.StatusCode, string(respBody))
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

func CreateTestApprovalRequest(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, policyID, module, requestType, sourceRecordType, sourceRecordID string, assignedToUserID *string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id": entityID,
		"module":             module,
		"request_type":       requestType,
		"source_record_type": sourceRecordType,
		"source_record_id":   sourceRecordID,
		"payload_snapshot": map[string]interface{}{
			"test": "data",
		},
	}
	if branchID != "" {
		body["branch_id"] = branchID
	}
	if policyID != "" {
		body["approval_policy_id"] = policyID
	}
	if assignedToUserID != nil {
		body["assigned_to_user_id"] = *assignedToUserID
	}

	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/approval-requests", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test approval request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test approval request: status=%d body=%s", resp.StatusCode, string(respBody))
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

func CreateSecondTestUser(t *testing.T) (userID, token string) {
	t.Helper()
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("approver_%d@ioi.dev", timestamp)
	password := "testing12345!"

	authPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	resp, err := http.Post(GoTrueURL+"/signup", "application/json", bytes.NewBuffer(authPayload))
	if err != nil {
		t.Fatalf("Failed to signup second test user: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to signup second test user: status=%d body=%s", resp.StatusCode, string(body))
	}

	var signupResult struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signupResult); err != nil {
		t.Fatalf("Failed to decode signup response: %v", err)
	}
	userID = signupResult.User.ID

	loginResp, err := http.Post(GoTrueURL+"/token?grant_type=password", "application/json", bytes.NewBuffer(authPayload))
	if err != nil {
		t.Fatalf("Failed to login second test user: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != 200 {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("Failed to login second test user: status=%d body=%s", loginResp.StatusCode, string(body))
	}

	var authResult struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&authResult); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	token = authResult.AccessToken

	return userID, token
}

func TestApprovalPolicies(t *testing.T) {
	token := testToken
	entityID, _ := setupApprovalsTestData(t)

	t.Run("list approval policies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/approval-policies", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("create approval policy", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		code := fmt.Sprintf("POLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, code, "Test Policy", "finance", "payment_void", 1, true)
		assert.NotEmpty(t, policyID)
	})

	t.Run("create approval policy with duplicate code fails", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		code := fmt.Sprintf("DUPE_POLICY_%d", timestamp)

		body := map[string]interface{}{
			"business_entity_id":    entityID,
			"code":                  code,
			"name":                  "First Policy",
			"module":                "finance",
			"request_type":          "payment_void",
			"min_approvers":         1,
			"prevent_self_approval": true,
			"is_active":             true,
		}

		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-policies", token, body)
		require.Equal(t, 201, resp.StatusCode)

		body["name"] = "Second Policy"
		resp2 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-policies", token, body)
		assert.Equal(t, 409, resp2.StatusCode)
	})

	t.Run("get approval policy by id", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		code := fmt.Sprintf("GETPOLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, code, "Get Test Policy", "finance", "financial_adjustment", 2, false)

		req := httptest.NewRequest("GET", "/api/v1/approval-policies/"+policyID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Equal(t, policyID, result["id"])
		assert.Equal(t, "Get Test Policy", result["name"])
		assert.Equal(t, "finance", result["module"])
		assert.Equal(t, "financial_adjustment", result["request_type"])
	})

	t.Run("get nonexistent approval policy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/approval-policies/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("update approval policy", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		code := fmt.Sprintf("UPDATEPOLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, code, "Original Name", "finance", "deposit_refund", 1, true)

		updateBody := map[string]interface{}{
			"name":          "Updated Name",
			"min_approvers": 2,
			"is_active":     false,
		}

		resp := MakeAuthenticatedRequest(t, testApp, "PATCH", "/api/v1/approval-policies/"+policyID, token, updateBody)
		require.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Equal(t, "Updated Name", result["name"])
		assert.Equal(t, float64(2), result["min_approvers"])
		assert.Equal(t, false, result["is_active"])
	})

	t.Run("update nonexistent approval policy", func(t *testing.T) {
		updateBody := map[string]interface{}{
			"name": "Should Fail",
		}

		resp := MakeAuthenticatedRequest(t, testApp, "PATCH", "/api/v1/approval-policies/00000000-0000-0000-0000-000000000000", token, updateBody)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("list approval policies with filters", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		code := fmt.Sprintf("FILTERPOLICY_%d", timestamp)
		_ = CreateTestApprovalPolicy(t, testApp, token, entityID, code, "Filter Test", "sales", "ownership_transfer", 1, true)

		req := httptest.NewRequest("GET", "/api/v1/approval-policies?module=sales&business_entity_id="+entityID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
	})
}

func TestApprovalRequests(t *testing.T) {
	token := testToken
	entityID, branchID := setupApprovalsTestData(t)
	requesterUserID := GetTestUserID(t, token)

	t.Run("list approval requests", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/approval-requests", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("create approval request", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("REQPOLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Request Test Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000001"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)
		assert.NotEmpty(t, requestID)
	})

	t.Run("get approval request by id", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("GETREQPOLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Get Request Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000002"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)

		req := httptest.NewRequest("GET", "/api/v1/approval-requests/"+requestID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "request")
		requestData := result["request"].(map[string]interface{})
		assert.Equal(t, requestID, requestData["id"])
		assert.Equal(t, "pending", requestData["status"])
	})

	t.Run("get nonexistent approval request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/approval-requests/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("self-approval blocking", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("SELFAVOID_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Self Approval Block Policy", "finance", "payment_void", 1, true)

		sourceRecordID := "00000000-0000-0000-0000-000000000003"
		assignedToUserID := requesterUserID
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &assignedToUserID)

		decideBody := map[string]interface{}{
			"decision": "approved",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", token, decideBody)
		assert.Equal(t, 400, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result["error"], "self-approval is not allowed")
	})

	t.Run("non-approver rejection", func(t *testing.T) {
		approverUserID, approverToken := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approverUserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("NONAPPROVER_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Non Approver Test Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000004"
		assignedToUserID := requesterUserID
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &assignedToUserID)

		decideBody := map[string]interface{}{
			"decision": "approved",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody)
		assert.Equal(t, 400, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result["error"], "not an eligible pending approver")
	})

	t.Run("duplicate voting prevention", func(t *testing.T) {
		approverUserID, approverToken := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approverUserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("DUPEVOTE_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Duplicate Vote Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000005"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &approverUserID)

		decideBody := map[string]interface{}{
			"decision": "approved",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody)
		require.Equal(t, 200, resp.StatusCode)

		decideBody2 := map[string]interface{}{
			"decision": "approved",
		}
		resp2 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody2)
		assert.Equal(t, 400, resp2.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp2, &result)
		assert.Contains(t, result["error"], "not in pending status")
	})

	t.Run("status transition pending to approved when min_approvers met", func(t *testing.T) {
		approverUserID, approverToken := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approverUserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("APPROVE_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Approval Flow Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000006"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &approverUserID)

		decideBody := map[string]interface{}{
			"decision": "approved",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody)
		require.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Equal(t, "approved", result["status"])
	})

	t.Run("status transition pending to rejected on first rejection", func(t *testing.T) {
		approverUserID, approverToken := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approverUserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("REJECT_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Rejection Flow Policy", "finance", "payment_void", 2, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000007"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &approverUserID)

		decideBody := map[string]interface{}{
			"decision": "rejected",
			"reason":   "Does not meet criteria",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody)
		require.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Equal(t, "rejected", result["status"])
	})

	t.Run("cancel approval request by requester", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("CANCEL_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Cancel Test Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000008"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)

		cancelBody := map[string]interface{}{
			"reason": "No longer needed",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/cancel", token, cancelBody)
		require.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Equal(t, "cancelled", result["status"])
	})

	t.Run("cancel approval request by non-requester fails", func(t *testing.T) {
		_, otherToken := CreateSecondTestUser(t)

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("CANCELDENY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Cancel Deny Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000009"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)

		cancelBody := map[string]interface{}{
			"reason": "Unauthorized cancel attempt",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/cancel", otherToken, cancelBody)
		assert.Equal(t, 403, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result["error"], "only the requester can cancel")
	})

	t.Run("decide on non-pending request fails", func(t *testing.T) {
		approverUserID, approverToken := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approverUserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("NONPENDING_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Non Pending Test Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000010"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &approverUserID)

		decideBody := map[string]interface{}{
			"decision": "approved",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody)
		require.Equal(t, 200, resp.StatusCode)

		decideBody2 := map[string]interface{}{
			"decision": "approved",
		}
		resp2 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approverToken, decideBody2)
		assert.Equal(t, 400, resp2.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp2, &result)
		assert.Contains(t, result["error"], "not in pending status")
	})

	t.Run("list approval requests with filters", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("FILTERREQ_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Filter Request Policy", "sales", "ownership_transfer", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000011"
		_ = CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "sales", "ownership_transfer", "contract", sourceRecordID, nil)

		req := httptest.NewRequest("GET", "/api/v1/approval-requests?module=sales&status=pending&business_entity_id="+entityID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
	})
}

func TestAuditLogs(t *testing.T) {
	token := testToken
	entityID, branchID := setupApprovalsTestData(t)

	t.Run("list audit logs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/audit-logs", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("list audit logs with filters", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("AUDITPOLICY_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Audit Test Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000012"
		_ = CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)

		req := httptest.NewRequest("GET", "/api/v1/audit-logs?module=finance&action_type=approval_request_created", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)
	})

	t.Run("get audit logs for entity", func(t *testing.T) {
		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("ENTITYAUDIT_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Entity Audit Policy", "finance", "payment_void", 1, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000013"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, nil)

		req := httptest.NewRequest("GET", "/api/v1/audit-logs/approval_request/"+requestID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "data")
		data := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(data), 1)

		firstLog := data[0].(map[string]interface{})
		assert.Equal(t, "approval_request", firstLog["entity_type"])
		assert.Equal(t, requestID, firstLog["entity_id"])
	})

	t.Run("get audit logs for nonexistent entity returns empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/audit-logs/approval_request/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		data := result["data"].([]interface{})
		assert.Equal(t, 0, len(data))
	})

	t.Run("list audit logs with date filters", func(t *testing.T) {
		dateFrom := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
		dateTo := time.Now().Add(24 * time.Hour).Format("2006-01-02")

		req := httptest.NewRequest("GET", "/api/v1/audit-logs?date_from="+dateFrom+"&date_to="+dateTo, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "data")
	})

	t.Run("list audit logs with pagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/audit-logs?page=1&per_page=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		pagination := result["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(10), pagination["per_page"])
	})
}

func TestApprovalRequestsWithMultipleApprovers(t *testing.T) {
	token := testToken
	entityID, branchID := setupApprovalsTestData(t)

	t.Run("requires all approvers when min_approvers equals total approvers", func(t *testing.T) {
		approver1UserID, approver1Token := CreateSecondTestUser(t)
		approver2UserID, approver2Token := CreateSecondTestUser(t)
		AssignRoleToUserByID(t, testApp, testToken, approver1UserID, entityID, "finance_approver")
		AssignRoleToUserByID(t, testApp, testToken, approver2UserID, entityID, "finance_approver")

		timestamp := time.Now().UnixNano()
		policyCode := fmt.Sprintf("MULTI_%d", timestamp)
		policyID := CreateTestApprovalPolicy(t, testApp, token, entityID, policyCode, "Multi Approver Policy", "finance", "payment_void", 2, false)

		sourceRecordID := "00000000-0000-0000-0000-000000000014"
		requestID := CreateTestApprovalRequest(t, testApp, token, entityID, branchID, policyID, "finance", "payment_void", "payment", sourceRecordID, &approver1UserID)

		addApproverBody := map[string]interface{}{
			"approval_request_id": requestID,
			"user_id":             approver2UserID,
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-request-approvers", token, addApproverBody)
		_ = resp

		decideBody1 := map[string]interface{}{
			"decision": "approved",
		}
		resp1 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approver1Token, decideBody1)
		require.Equal(t, 200, resp1.StatusCode)

		var result1 map[string]interface{}
		DecodeResponse(t, resp1, &result1)
		assert.Equal(t, "pending", result1["status"])

		decideBody2 := map[string]interface{}{
			"decision": "approved",
		}
		resp2 := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/approval-requests/"+requestID+"/decide", approver2Token, decideBody2)
		require.Equal(t, 200, resp2.StatusCode)

		var result2 map[string]interface{}
		DecodeResponse(t, resp2, &result2)
		assert.Equal(t, "approved", result2["status"])
	})
}
