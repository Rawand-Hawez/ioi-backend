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
	"github.com/stretchr/testify/require"

	"IOI-real-estate-backend/internal/db/pool"
)

func TestSalesPhase7Harness(t *testing.T) {
	require.NotNil(t, testApp)
	require.NotEmpty(t, testToken)
}

func createSalesTestParty(t *testing.T, name string) string {
	t.Helper()

	code := fmt.Sprintf("SALESPARTY_%d", time.Now().UnixNano())
	return CreateTestParty(t, testApp, testToken, code, name, "person", "active")
}

func createSalesTestUnit(t *testing.T) string {
	t.Helper()

	timestamp := time.Now().UnixNano()
	entityID := CreateTestBusinessEntity(t, testApp, testToken, fmt.Sprintf("SALESENTITY_%d", timestamp), "Sales Test Entity")
	branchID := CreateTestBranch(t, testApp, testToken, entityID, fmt.Sprintf("SALESBRANCH_%d", timestamp), "Sales Test Branch")
	projectID := CreateTestProject(t, testApp, testToken, entityID, branchID, fmt.Sprintf("SALESPROJ_%d", timestamp), "Sales Test Project")

	return CreateTestUnit(t, testApp, testToken, entityID, branchID, projectID, fmt.Sprintf("SALESUNIT_%d", timestamp))
}

func createSalesTestPaymentPlanTemplate(t *testing.T, businessEntityID string, rule map[string]any) string {
	t.Helper()

	if rule == nil {
		rule = map[string]any{
			"type":              "equal_installments",
			"down_payment_rate": 0.10,
		}
	}

	body := map[string]any{
		"business_entity_id":   businessEntityID,
		"code":                 fmt.Sprintf("SALESPLAN_%d", time.Now().UnixNano()),
		"name":                 "Sales Test Payment Plan",
		"status":               "active",
		"frequency_type":       "monthly",
		"installment_count":    12,
		"generation_rule_json": rule,
	}

	rr := salesRequest(t, http.MethodPost, "/api/v1/payment-plan-templates", body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	return responseID(t, rr.Body)
}

func createSalesTestReservation(t *testing.T, unitID string, partyID string, status string) string {
	t.Helper()

	unit := getSalesTestUnit(t, unitID)
	body := map[string]any{
		"business_entity_id": unit["business_entity_id"],
		"branch_id":          unit["branch_id"],
		"project_id":         unit["project_id"],
		"unit_id":            unitID,
		"customer_id":        partyID,
		"expires_at":         time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"deposit_amount":     "0.00",
		"deposit_currency":   "USD",
	}

	rr := salesRequest(t, http.MethodPost, "/api/v1/reservations", body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	id := responseID(t, rr.Body)
	if status != "" && status != "active" {
		setSalesTestStatus(t, "reservations", id, status)
	}
	return id
}

func createSalesTestContract(t *testing.T, unitID string, primaryBuyerID string, status string) string {
	t.Helper()

	unit := getSalesTestUnit(t, unitID)
	body := map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"primary_buyer_id":    primaryBuyerID,
		"contract_date":       "2026-04-24",
		"effective_date":      "2026-04-24",
		"sale_price_amount":   "100000.00",
		"sale_price_currency": "USD",
		"discount_amount":     "0.00",
		"net_contract_amount": "100000.00",
		"down_payment_amount": "10000.00",
		"financed_amount":     "90000.00",
	}

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts", body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	id := responseID(t, rr.Body)
	if status != "" && status != "draft" {
		setSalesTestStatus(t, "sales_contracts", id, status)
	}
	return id
}

func createSalesTestScheduleLine(t *testing.T, contractID string, dueDate string, amount string, status string) string {
	t.Helper()

	contractUUID := uuid.MustParse(contractID)
	lineID := uuid.New()

	p := pool.Get()
	require.NotNil(t, p)

	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var lineNo int16
		if err := tx.QueryRow(context.Background(), `
			SELECT COALESCE(MAX(line_no), 0)::smallint + 1
			FROM installment_schedule_lines
			WHERE sales_contract_id = $1
		`, contractUUID).Scan(&lineNo); err != nil {
			return err
		}

		_, err := tx.Exec(context.Background(), `
			INSERT INTO installment_schedule_lines (
				id, sales_contract_id, line_no, due_date, line_type,
				description, principal_amount, status
			) VALUES ($1, $2, $3, $4, 'installment', 'Sales test schedule line', $5, $6)
		`, lineID, contractUUID, lineNo, dueDate, amount, status)
		return err
	})
	require.NoError(t, err)

	return lineID.String()
}

func salesRequest(t *testing.T, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return salesRequestWithToken(t, testToken, method, path, body)
}

func salesRequestWithToken(t *testing.T, token string, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testApp.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	rr := httptest.NewRecorder()
	rr.Code = resp.StatusCode
	for key, values := range resp.Header {
		for _, value := range values {
			rr.Header().Add(key, value)
		}
	}
	_, err = io.Copy(rr.Body, resp.Body)
	require.NoError(t, err)

	return rr
}

func getSalesTestUnit(t *testing.T, unitID string) map[string]any {
	t.Helper()

	rr := salesRequest(t, http.MethodGet, "/api/v1/units/"+unitID, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var unit map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&unit))

	return unit
}

func responseID(t *testing.T, body *bytes.Buffer) string {
	t.Helper()

	var result map[string]any
	require.NoError(t, json.Unmarshal(body.Bytes(), &result))

	id, ok := result["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func setSalesTestStatus(t *testing.T, table string, id string, status string) {
	t.Helper()

	recordID := uuid.MustParse(id)
	p := pool.Get()
	require.NotNil(t, p)

	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), fmt.Sprintf("UPDATE %s SET status = $1, updated_at = timezone('utc', now()) WHERE id = $2", table), status, recordID)
		return err
	})
	require.NoError(t, err)
}
