package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"

	"IOI-real-estate-backend/internal/db/pool"
)

func setupFinanceTestData(t *testing.T) (entityID, branchID, projectID, unitID, partyID string) {
	t.Helper()
	timestamp := time.Now().UnixNano()

	entityCode := fmt.Sprintf("FINENTITY_%d", timestamp)
	entityID = CreateTestBusinessEntity(t, testApp, testToken, entityCode, "Finance Test Entity")

	branchCode := fmt.Sprintf("FINBRANCH_%d", timestamp)
	branchID = CreateTestBranch(t, testApp, testToken, entityID, branchCode, "Finance Test Branch")

	projectCode := fmt.Sprintf("FINPROJ_%d", timestamp)
	projectID = CreateTestProject(t, testApp, testToken, entityID, branchID, projectCode, "Finance Test Project")

	unitCode := fmt.Sprintf("FINUNIT_%d", timestamp)
	unitID = CreateTestUnit(t, testApp, testToken, entityID, branchID, projectID, unitCode)

	partyCode := fmt.Sprintf("FINPARTY_%d", timestamp)
	partyID = CreateTestParty(t, testApp, testToken, partyCode, "Finance Test Party", "person", "active")

	return
}

func CreateTestReceivable(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, partyID string, amount string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id": entityID,
		"branch_id":          branchID,
		"party_id":           partyID,
		"receivable_date":    "2024-01-15",
		"due_date":           "2024-02-15",
		"original_amount":    amount,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/receivables", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test receivable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test receivable: status=%d body=%s", resp.StatusCode, string(respBody))
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

func CreateTestPayment(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, partyID, amount string) string {
	t.Helper()

	body := map[string]interface{}{
		"business_entity_id": entityID,
		"branch_id":          branchID,
		"party_id":           partyID,
		"payment_date":       "2024-01-20",
		"payment_method":     "cash",
		"amount_received":    amount,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/payments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test payment: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test payment: status=%d body=%s", resp.StatusCode, string(respBody))
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

func CreateTestCreditBalanceDirect(t *testing.T, entityID, branchID, partyID, amount string) string {
	t.Helper()

	p := pool.Get()
	ctx := context.Background()

	creditID := uuid.New()

	err := p.WithTx(ctx, map[string]interface{}{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO credit_balances (
				id, business_entity_id, branch_id, party_id,
				source_module, source_record_type, source_record_id,
				currency_code, amount_total, reason
			) VALUES ($1, $2, $3, $4, 'test', 'manual', $5, 'USD', $6, 'Test credit balance')
		`, creditID, entityID, branchID, partyID, creditID, amount)
		return err
	})

	if err != nil {
		t.Fatalf("Failed to create test credit balance: %v", err)
	}

	return creditID.String()
}

func CreateTestFinancialAdjustment(t *testing.T, app interface {
	Test(req *http.Request, timeout ...int) (*http.Response, error)
}, token, entityID, branchID, partyID, adjustmentType, amount string) string {
	t.Helper()

	sourceRecordID := uuid.New().String()
	body := map[string]interface{}{
		"business_entity_id": entityID,
		"branch_id":          branchID,
		"party_id":           partyID,
		"source_module":      "manual",
		"source_record_type": "manual",
		"source_record_id":   sourceRecordID,
		"adjustment_type":    adjustmentType,
		"amount":             amount,
		"effective_date":     "2024-01-15",
		"reason":             "Test adjustment",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/financial-adjustments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to create test financial adjustment: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create test financial adjustment: status=%d body=%s", resp.StatusCode, string(respBody))
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

func TestReceivables(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("create manual receivable", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"branch_id":          branchID,
			"party_id":           partyID,
			"receivable_date":    "2024-01-15",
			"due_date":           "2024-02-15",
			"original_amount":    "1000.00",
		}

		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/receivables", token, body)
		assert.Equal(t, 201, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.NotEmpty(t, result["id"])
		assert.Equal(t, "open", result["status"])
		assert.InDelta(t, 1000.00, result["original_amount"], 0.01)
	})

	t.Run("list receivables with filters", func(t *testing.T) {
		CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("GET", "/api/v1/receivables?party_id="+partyID+"&status=open", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	t.Run("get receivable by id", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "750.00")

		req := httptest.NewRequest("GET", "/api/v1/receivables/"+receivableID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, receivableID, result["id"])
		assert.InDelta(t, 750.00, result["original_amount"], 0.01)
	})

	t.Run("get nonexistent receivable", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/receivables/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("create receivable with invalid amount", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"branch_id":          branchID,
			"party_id":           partyID,
			"receivable_date":    "2024-01-15",
			"due_date":           "2024-02-15",
			"original_amount":    "-100.00",
		}

		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/receivables", token, body)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestPayments(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("create draft payment", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"branch_id":          branchID,
			"party_id":           partyID,
			"payment_date":       "2024-01-20",
			"payment_method":     "cash",
			"amount_received":    "500.00",
		}

		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments", token, body)
		assert.Equal(t, 201, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.NotEmpty(t, result["id"])
		assert.Equal(t, "draft", result["status"])
		assert.InDelta(t, 500.00, result["amount_received"], 0.01)
	})

	t.Run("post draft payment", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "300.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "posted", result["status"])
	})

	t.Run("post already posted payment", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "200.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req2, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("post voided payment fails", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "200.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/void", nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req2, -1)

		req3 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req3.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req3, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("void posted payment", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "150.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/void", nil)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req2, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "voided", result["status"])
	})

	t.Run("void draft payment fails", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "100.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/void", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestAllocations(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("allocate to single receivable", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "1000.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "500.00"},
			},
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "allocations")
	})

	t.Run("allocate to multiple receivables", func(t *testing.T) {
		receivableID1 := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "300.00")
		receivableID2 := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "200.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID1, "amount": "300.00"},
				{"receivable_id": receivableID2, "amount": "200.00"},
			},
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("over-allocate fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "100.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "200.00"},
			},
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("allocate to fully paid receivable fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "100.00")
		paymentID1 := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "100.00")
		paymentID2 := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "50.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID1+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "100.00"},
			},
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID1+"/allocate", token, allocBody)

		req2 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID2+"/post", nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req2, -1)

		allocBody2 := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "50.00"},
			},
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID2+"/allocate", token, allocBody2)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("allocate with non-posted payment fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "100.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "50.00")

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "50.00"},
			},
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestVoidPaymentWithAllocations(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("void payment reverses receivable amounts", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "1000.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "500.00"},
			},
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)

		req2 := httptest.NewRequest("GET", "/api/v1/receivables/"+receivableID, nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		resp2, _ := testApp.Test(req2, -1)
		var beforeVoid map[string]interface{}
		json.NewDecoder(resp2.Body).Decode(&beforeVoid)
		assert.Equal(t, "partially_paid", beforeVoid["status"])

		req3 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/void", nil)
		req3.Header.Set("Authorization", "Bearer "+token)
		resp3, _ := testApp.Test(req3, -1)
		assert.Equal(t, 200, resp3.StatusCode)

		req4 := httptest.NewRequest("GET", "/api/v1/receivables/"+receivableID, nil)
		req4.Header.Set("Authorization", "Bearer "+token)
		resp4, _ := testApp.Test(req4, -1)
		var afterVoid map[string]interface{}
		json.NewDecoder(resp4.Body).Decode(&afterVoid)
		assert.Equal(t, "open", afterVoid["status"])
	})
}

func TestCreditBalances(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("apply credit balance to receivable", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "500.00")
		creditID := CreateTestCreditBalanceDirect(t, entityID, branchID, partyID, "200.00")

		applyBody := map[string]interface{}{
			"receivable_id": receivableID,
			"amount":        "200.00",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/credit-balances/"+creditID+"/apply", token, applyBody)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		DecodeResponse(t, resp, &result)
		assert.Contains(t, result, "amount_used")
	})

	t.Run("apply more than available fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "1000.00")
		creditID := CreateTestCreditBalanceDirect(t, entityID, branchID, partyID, "100.00")

		applyBody := map[string]interface{}{
			"receivable_id": receivableID,
			"amount":        "500.00",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/credit-balances/"+creditID+"/apply", token, applyBody)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("apply to fully paid receivable fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "100.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "100.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "100.00"},
			},
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)

		creditID := CreateTestCreditBalanceDirect(t, entityID, branchID, partyID, "50.00")
		applyBody := map[string]interface{}{
			"receivable_id": receivableID,
			"amount":        "50.00",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/credit-balances/"+creditID+"/apply", token, applyBody)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("apply exhausted credit fails", func(t *testing.T) {
		receivableID1 := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "500.00")
		receivableID2 := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "500.00")
		creditID := CreateTestCreditBalanceDirect(t, entityID, branchID, partyID, "100.00")

		applyBody1 := map[string]interface{}{
			"receivable_id": receivableID1,
			"amount":        "100.00",
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/credit-balances/"+creditID+"/apply", token, applyBody1)

		applyBody2 := map[string]interface{}{
			"receivable_id": receivableID2,
			"amount":        "50.00",
		}
		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/credit-balances/"+creditID+"/apply", token, applyBody2)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

func TestFinancialAdjustments(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("create financial adjustment", func(t *testing.T) {
		adjustmentID := CreateTestFinancialAdjustment(t, testApp, token, entityID, branchID, partyID, "credit", "250.00")
		assert.NotEmpty(t, adjustmentID)

		req := httptest.NewRequest("GET", "/api/v1/financial-adjustments/"+adjustmentID, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "pending", result["status"])
	})

	t.Run("approve financial adjustment", func(t *testing.T) {
		adjustmentID := CreateTestFinancialAdjustment(t, testApp, token, entityID, branchID, partyID, "credit", "100.00")

		req := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/approve", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "approved", result["status"])
	})

	t.Run("reject financial adjustment", func(t *testing.T) {
		adjustmentID := CreateTestFinancialAdjustment(t, testApp, token, entityID, branchID, partyID, "debit", "75.00")

		req := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/reject", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "rejected", result["status"])
	})

	t.Run("approve already approved adjustment fails", func(t *testing.T) {
		adjustmentID := CreateTestFinancialAdjustment(t, testApp, token, entityID, branchID, partyID, "waiver", "50.00")

		req := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/approve", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/approve", nil)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req2, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("reject already approved adjustment fails", func(t *testing.T) {
		adjustmentID := CreateTestFinancialAdjustment(t, testApp, token, entityID, branchID, partyID, "penalty", "25.00")

		req := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/approve", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/financial-adjustments/"+adjustmentID+"/reject", nil)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req2, -1)
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})
}

func TestStatements(t *testing.T) {
	token := testToken
	entityID, branchID, _, unitID, partyID := setupFinanceTestData(t)

	t.Run("party statement", func(t *testing.T) {
		CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "300.00")

		req := httptest.NewRequest("GET", "/api/v1/parties/"+partyID+"/statement", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
	})

	t.Run("unit statement", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"branch_id":          branchID,
			"party_id":           partyID,
			"unit_id":            unitID,
			"receivable_date":    "2024-01-15",
			"due_date":           "2024-02-15",
			"original_amount":    "400.00",
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/receivables", token, body)

		req := httptest.NewRequest("GET", "/api/v1/units/"+unitID+"/statement", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Contains(t, result, "data")
	})
}

func TestImmutabilityTriggers(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("modify original_amount on partially_paid receivable fails", func(t *testing.T) {
		receivableID := CreateTestReceivable(t, testApp, token, entityID, branchID, partyID, "1000.00")
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		allocBody := map[string]interface{}{
			"allocations": []map[string]string{
				{"receivable_id": receivableID, "amount": "500.00"},
			},
		}
		MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments/"+paymentID+"/allocate", token, allocBody)

		p := pool.Get()
		ctx := context.Background()

		receivableUUID, _ := uuid.Parse(receivableID)
		err := p.WithTx(ctx, map[string]interface{}{"sub": token}, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE receivables SET original_amount = 2000.00 WHERE id = $1", receivableUUID)
			return err
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot modify original_amount")
	})

	t.Run("modify amount_received on posted payment fails", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "500.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		p := pool.Get()
		ctx := context.Background()

		paymentUUID, _ := uuid.Parse(paymentID)
		err := p.WithTx(ctx, map[string]interface{}{"sub": token}, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE payments SET amount_received = 1000.00 WHERE id = $1", paymentUUID)
			return err
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot modify amount_received")
	})
}

func TestPermissionChecks(t *testing.T) {
	token := testToken
	entityID, branchID, _, _, partyID := setupFinanceTestData(t)

	t.Run("create payment without permission returns 403 for restricted user", func(t *testing.T) {
		body := map[string]interface{}{
			"business_entity_id": entityID,
			"branch_id":          branchID,
			"party_id":           partyID,
			"payment_date":       "2024-01-20",
			"payment_method":     "cash",
			"amount_received":    "100.00",
		}

		resp := MakeAuthenticatedRequest(t, testApp, "POST", "/api/v1/payments", token, body)
		assert.Equal(t, 201, resp.StatusCode)
	})

	t.Run("post payment without permission returns 403 for restricted user", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "100.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("void payment without permission returns 403 for restricted user", func(t *testing.T) {
		paymentID := CreateTestPayment(t, testApp, token, entityID, branchID, partyID, "100.00")

		req := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/post", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		testApp.Test(req, -1)

		req2 := httptest.NewRequest("POST", "/api/v1/payments/"+paymentID+"/void", nil)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp, err := testApp.Test(req2, -1)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}
