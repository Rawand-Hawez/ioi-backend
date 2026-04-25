package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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

func TestSalesWriteRoutesRequirePermissions(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Permission Buyer")
	reservationID := createSalesTestReservation(t, unitID, buyerID, "active")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	lineID := createSalesTestScheduleLine(t, contractID, "2026-05-01", "1000.00", "scheduled")

	restrictedToken := createRestrictedTestToken(t)

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{"POST", "/api/v1/reservations", map[string]any{"unit_id": unitID, "customer_party_id": buyerID}},
		{"POST", "/api/v1/reservations/" + reservationID + "/convert", map[string]any{}},
		{"POST", "/api/v1/reservations/" + reservationID + "/cancel", map[string]any{"reason": "test"}},
		{"POST", "/api/v1/sales-contracts", map[string]any{"unit_id": unitID, "primary_buyer_id": buyerID}},
		{"PATCH", "/api/v1/sales-contracts/" + contractID, map[string]any{"discount_amount": "1.00"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/activate", map[string]any{}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/cancel", map[string]any{"reason": "test"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/complete", map[string]any{}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/terminate", map[string]any{"reason": "test"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/mark-default", map[string]any{"reason": "test"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/schedule/generate", map[string]any{}},
		{"PATCH", "/api/v1/schedule-lines/" + lineID, map[string]any{"due_date": "2026-06-01"}},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := salesRequestWithToken(t, restrictedToken, tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusForbidden, rr.Code)
		})
	}
}

func TestSalesScheduleRoutesMatchPhase7Plan(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Schedule Route Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")

	getRR := salesRequest(t, "GET", "/api/v1/sales-contracts/"+contractID+"/schedule", nil)
	require.Equal(t, http.StatusOK, getRR.Code)

	generateRR := salesRequest(t, "POST", "/api/v1/sales-contracts/"+contractID+"/schedule/generate", map[string]any{})
	require.NotEqual(t, http.StatusNotFound, generateRR.Code)

	oldMutationRR := salesRequest(t, "PATCH", "/api/v1/sales-contracts/"+contractID+"/schedule-lines/not-a-real-id", map[string]any{})
	require.Equal(t, http.StatusNotFound, oldMutationRR.Code)
}

func TestSalesContractRejectsInconsistentAmounts(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Constraint Buyer")
	unit := getSalesTestUnit(t, unitID)

	body := map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"primary_buyer_id":    buyerID,
		"contract_date":       "2026-04-24",
		"effective_date":      "2026-04-24",
		"sale_price_amount":   "100000.00",
		"discount_amount":     "5000.00",
		"net_contract_amount": "99000.00",
		"down_payment_amount": "10000.00",
		"financed_amount":     "85000.00",
		"sale_price_currency": "USD",
	}

	rr := salesRequest(t, "POST", "/api/v1/sales-contracts", body)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateSalesContractDerivesNetAndFinancedAmounts(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Derived Amount Buyer")
	unit := getSalesTestUnit(t, unitID)

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts", map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"primary_buyer_id":    buyerID,
		"contract_date":       "2026-04-24",
		"effective_date":      "2026-04-24",
		"sale_price_amount":   "125000.00",
		"sale_price_currency": "USD",
		"discount_amount":     "5000.00",
		"down_payment_amount": "20000.00",
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	contract := decodeSalesTestObject(t, rr.Body)
	require.Equal(t, "120000.00", normalizeSalesTestJSONAmount(t, contract["net_contract_amount"]))
	require.Equal(t, "20000.00", normalizeSalesTestJSONAmount(t, contract["down_payment_amount"]))
	require.Equal(t, "100000.00", normalizeSalesTestJSONAmount(t, contract["financed_amount"]))
}

func TestCreateSalesContractRejectsProvidedInconsistentAmounts(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Provided Inconsistent Buyer")
	unit := getSalesTestUnit(t, unitID)

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts", map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"primary_buyer_id":    buyerID,
		"contract_date":       "2026-04-24",
		"effective_date":      "2026-04-24",
		"sale_price_amount":   "125000.00",
		"sale_price_currency": "USD",
		"discount_amount":     "5000.00",
		"net_contract_amount": "125000.00",
		"down_payment_amount": "20000.00",
		"financed_amount":     "105000.00",
	})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

func TestConvertReservationCreatesDraftContractWithDerivedAmounts(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Reservation Conversion Buyer")
	unit := getSalesTestUnit(t, unitID)

	reservationRR := salesRequest(t, http.MethodPost, "/api/v1/reservations", map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"customer_id":         buyerID,
		"expires_at":          time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"deposit_amount":      "15000.00",
		"deposit_currency":    "USD",
		"quoted_price_amount": "150000.00",
		"discount_amount":     "10000.00",
	})
	require.Equal(t, http.StatusCreated, reservationRR.Code, reservationRR.Body.String())

	convertRR := salesRequest(t, http.MethodPost, "/api/v1/reservations/"+responseID(t, reservationRR.Body)+"/convert", map[string]any{})
	require.Equal(t, http.StatusCreated, convertRR.Code, convertRR.Body.String())

	response := decodeSalesTestObject(t, convertRR.Body)
	contract, ok := response["contract"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "draft", contract["status"])
	require.Equal(t, "150000.00", normalizeSalesTestJSONAmount(t, contract["sale_price_amount"]))
	require.Equal(t, "10000.00", normalizeSalesTestJSONAmount(t, contract["discount_amount"]))
	require.Equal(t, "140000.00", normalizeSalesTestJSONAmount(t, contract["net_contract_amount"]))
	require.Equal(t, "15000.00", normalizeSalesTestJSONAmount(t, contract["down_payment_amount"]))
	require.Equal(t, "125000.00", normalizeSalesTestJSONAmount(t, contract["financed_amount"]))
}

func TestPaymentPlanTemplateValidatesGenerationRule(t *testing.T) {
	businessEntityID := createSalesTestBusinessEntity(t)

	cases := []struct {
		name string
		rule map[string]any
	}{
		{
			name: "percentages do not total 100",
			rule: map[string]any{
				"down_payment": map[string]any{"percentage": 10, "anchor": "reservation"},
				"tranches": []map[string]any{
					{"percentage": 80, "anchor": "contract_date", "installment_count": 8, "frequency": "monthly"},
				},
			},
		},
		{
			name: "invalid anchor",
			rule: map[string]any{
				"down_payment": map[string]any{"percentage": 10, "anchor": "invalid"},
				"tranches": []map[string]any{
					{"percentage": 90, "anchor": "contract_date", "installment_count": 9, "frequency": "monthly"},
				},
			},
		},
		{
			name: "installment count mismatch",
			rule: map[string]any{
				"down_payment": map[string]any{"percentage": 10, "anchor": "reservation"},
				"tranches": []map[string]any{
					{"percentage": 90, "anchor": "contract_date", "installment_count": 8, "frequency": "monthly"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := salesRequest(t, "POST", "/api/v1/payment-plan-templates", map[string]any{
				"business_entity_id":   businessEntityID,
				"code":                 fmt.Sprintf("INVALID_PLAN_%d", time.Now().UnixNano()),
				"name":                 "Invalid " + tc.name,
				"status":               "active",
				"frequency_type":       "monthly",
				"installment_count":    9,
				"generation_rule_json": tc.rule,
			})
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestPaymentPlanTemplateAcceptsValidGenerationRule(t *testing.T) {
	businessEntityID := createSalesTestBusinessEntity(t)

	rr := salesRequest(t, "POST", "/api/v1/payment-plan-templates", map[string]any{
		"business_entity_id": businessEntityID,
		"code":               fmt.Sprintf("VALID_PLAN_%d", time.Now().UnixNano()),
		"name":               "Valid payment plan",
		"status":             "active",
		"frequency_type":     "monthly",
		"installment_count":  9,
		"generation_rule_json": map[string]any{
			"down_payment": map[string]any{"percentage": 10, "anchor": "reservation"},
			"tranches": []map[string]any{
				{"percentage": 90, "anchor": "contract_date", "installment_count": 9, "frequency": "monthly"},
			},
		},
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

func TestGenerateSalesContractScheduleUsesPaymentPlanTemplate(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Template Schedule Buyer")
	businessEntityID := createSalesTestBusinessEntity(t)
	templateID := createSalesTestPaymentPlanTemplate(t, businessEntityID, map[string]any{
		"down_payment": map[string]any{"percentage": 10, "anchor": "contract_date", "offset_days": 0},
		"tranches": []map[string]any{
			{"percentage": 90, "anchor": "contract_date", "offset_days": 30, "installment_count": 3, "frequency": "monthly", "line_type": "installment"},
		},
	})
	contractID := createSalesTestContractWithTemplate(t, unitID, buyerID, templateID, "100000.00", "0.00", "10000.00")

	rr := salesRequest(t, "POST", "/api/v1/sales-contracts/"+contractID+"/schedule/generate", map[string]any{
		"contract_date": "2026-05-01",
	})

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	lines := decodeScheduleLines(t, rr.Body)
	require.Len(t, lines, 4)
	require.Equal(t, "down_payment", lines[0].LineType)
	require.Equal(t, "10000.00", lines[0].PrincipalAmount)
	require.Equal(t, "installment", lines[1].LineType)
	require.Equal(t, "30000.00", lines[1].PrincipalAmount)
	require.Equal(t, "2026-05-31", lines[1].DueDate)
}

func TestGenerateSalesContractScheduleRequiresHandoverDateWhenAnchored(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Handover Schedule Buyer")
	businessEntityID := createSalesTestBusinessEntity(t)
	templateID := createSalesTestPaymentPlanTemplate(t, businessEntityID, map[string]any{
		"down_payment": map[string]any{"percentage": 10, "anchor": "contract_date", "offset_days": 0},
		"tranches": []map[string]any{
			{"percentage": 90, "anchor": "handover_date", "offset_days": 0, "installment_count": 3, "frequency": "monthly", "line_type": "installment"},
		},
	})
	contractID := createSalesTestContractWithTemplate(t, unitID, buyerID, templateID, "100000.00", "0.00", "10000.00")

	rr := salesRequest(t, "POST", "/api/v1/sales-contracts/"+contractID+"/schedule/generate", map[string]any{
		"contract_date": "2026-05-01",
	})

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "handover_date")
}

func TestCreateReservationExpiresStaleReservationsBeforeConflictCheck(t *testing.T) {
	unitID := createSalesTestUnit(t)
	customerID := createSalesTestParty(t, "Expired Reservation Buyer")
	staleReservationID := createSalesTestReservation(t, unitID, customerID, "active")
	expireSalesTestReservation(t, staleReservationID)

	unit := getSalesTestUnit(t, unitID)
	rr := salesRequest(t, http.MethodPost, "/api/v1/reservations", map[string]any{
		"business_entity_id": unit["business_entity_id"],
		"branch_id":          unit["branch_id"],
		"project_id":         unit["project_id"],
		"unit_id":            unitID,
		"customer_id":        customerID,
		"expires_at":         time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"deposit_amount":     "0.00",
		"deposit_currency":   "USD",
		"discount_amount":    "0.00",
	})

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

func TestCreateReservationRejectsInvalidDepositPayment(t *testing.T) {
	unitID := createSalesTestUnit(t)
	customerID := createSalesTestParty(t, "Invalid Deposit Buyer")
	unit := getSalesTestUnit(t, unitID)
	paymentID := createSalesTestPayment(t, unit["business_entity_id"].(string), unit["branch_id"].(string), customerID, "500.00")

	rr := salesRequest(t, http.MethodPost, "/api/v1/reservations", map[string]any{
		"business_entity_id": unit["business_entity_id"],
		"branch_id":          unit["branch_id"],
		"project_id":         unit["project_id"],
		"unit_id":            unitID,
		"customer_id":        customerID,
		"expires_at":         time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"deposit_amount":     "500.00",
		"deposit_currency":   "USD",
		"deposit_payment_id": paymentID,
		"discount_amount":    "0.00",
	})

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

func TestReservationCancelReleasesUnitWhenNoActiveContract(t *testing.T) {
	unitID := createSalesTestUnit(t)
	customerID := createSalesTestParty(t, "Cancel Reservation Buyer")
	reservationID := createSalesTestReservation(t, unitID, customerID, "active")

	rr := salesRequest(t, http.MethodPost, "/api/v1/reservations/"+reservationID+"/cancel", map[string]any{"reason": "test"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	unit := getSalesTestUnit(t, unitID)
	require.Equal(t, "available", unit["sales_status"])
}

func TestActivateSalesContractGeneratesScheduleFromTemplateWhenNoLinesExist(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Activation Template Buyer")
	unit := getSalesTestUnit(t, unitID)
	templateID := createSalesTestPaymentPlanTemplate(t, unit["business_entity_id"].(string), map[string]any{
		"down_payment": map[string]any{"percentage": 20, "anchor": "contract_date"},
		"tranches": []map[string]any{
			{"percentage": 80, "anchor": "contract_date", "installment_count": 4, "frequency": "monthly"},
		},
	})
	contractID := createSalesTestContractWithTemplate(t, unitID, buyerID, templateID, "100000.00", "0.00", "20000.00")

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	response := decodeSalesTestObject(t, rr.Body)
	contract := response["contract"].(map[string]any)
	require.Equal(t, "active", contract["status"])
	require.Equal(t, "sold", getSalesTestUnit(t, unitID)["sales_status"])
	require.Len(t, getSalesTestScheduleLineReceivables(t, contractID), 5)
}

func TestActivateSalesContractCreatesReceivablesForEveryScheduleLine(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Activation Receivable Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	lineID1 := createSalesTestScheduleLine(t, contractID, "2026-05-01", "40000.00", "scheduled")
	lineID2 := createSalesTestScheduleLine(t, contractID, "2026-06-01", "60000.00", "scheduled")

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 2)
	expectedAmounts := map[string]string{
		lineID1: "40000.00",
		lineID2: "60000.00",
	}
	for _, line := range lines {
		require.NotEmpty(t, line["receivable_id"])
		receivable := getSalesTestReceivable(t, line["receivable_id"])
		require.Equal(t, "sales", receivable["source_module"])
		require.Equal(t, "installment_schedule_line", receivable["source_record_type"])
		require.Equal(t, line["id"], receivable["source_record_id"])
		require.Equal(t, expectedAmounts[line["id"]], normalizeSalesTestAmountString(t, receivable["original_amount"]))
	}
}

func TestActivateSalesContractRejectsSecondActiveContractForUnit(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID1 := createSalesTestParty(t, "First Active Buyer")
	buyerID2 := createSalesTestParty(t, "Second Active Buyer")
	firstContractID := createSalesTestContract(t, unitID, buyerID1, "draft")
	secondContractID := createSalesTestContract(t, unitID, buyerID2, "draft")
	createSalesTestScheduleLine(t, firstContractID, "2026-05-01", "100000.00", "scheduled")
	createSalesTestScheduleLine(t, secondContractID, "2026-05-01", "100000.00", "scheduled")

	firstRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+firstContractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, firstRR.Code, firstRR.Body.String())

	secondRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+secondContractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusConflict, secondRR.Code, secondRR.Body.String())
}

func TestCancelActiveSalesContractCreatesApprovalAndDoesNotMutateImmediately(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Active Cancel Request Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/cancel", map[string]any{"reason": "request"})
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())

	body := decodeSalesTestObject(t, rr.Body)
	approvalID, _ := body["approval_request_id"].(string)
	require.NotEmpty(t, approvalID, "response must include approval_request_id")

	contract := getSalesTestContract(t, contractID)
	require.Equal(t, "active", contract["status"], "contract must not mutate before approval")

	require.Equal(t, "pending", getSalesTestApprovalRequestStatus(t, approvalID))
	require.Equal(t, 1, countSalesTestApprovalsForContract(t, contractID, "contract_cancellation"))
}

func TestCancelActiveSalesContractAppliesAfterApproval(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Active Cancel Apply Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	firstRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/cancel", map[string]any{"reason": "request"})
	require.Equal(t, http.StatusAccepted, firstRR.Code, firstRR.Body.String())
	firstBody := decodeSalesTestObject(t, firstRR.Body)
	approvalID, _ := firstBody["approval_request_id"].(string)
	require.NotEmpty(t, approvalID)

	approveSalesTestApprovalRequest(t, approvalID)

	applyRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/cancel", map[string]any{})
	require.Equal(t, http.StatusOK, applyRR.Code, applyRR.Body.String())

	applyBody := decodeSalesTestObject(t, applyRR.Body)
	contract, _ := applyBody["contract"].(map[string]any)
	require.Equal(t, "cancelled", contract["status"])

	stored := getSalesTestContract(t, contractID)
	require.Equal(t, "cancelled", stored["status"])
}

func TestTerminateSalesContractCreatesApprovalAndAppliesAfterApproval(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Terminate Apply Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	firstRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/terminate", map[string]any{"reason": "request"})
	require.Equal(t, http.StatusAccepted, firstRR.Code, firstRR.Body.String())
	firstBody := decodeSalesTestObject(t, firstRR.Body)
	approvalID, _ := firstBody["approval_request_id"].(string)
	require.NotEmpty(t, approvalID, "response must include approval_request_id")

	require.Equal(t, "active", getSalesTestContract(t, contractID)["status"], "contract must not mutate before approval")

	approveSalesTestApprovalRequest(t, approvalID)

	applyRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/terminate", map[string]any{})
	require.Equal(t, http.StatusOK, applyRR.Code, applyRR.Body.String())

	applyBody := decodeSalesTestObject(t, applyRR.Body)
	contract, _ := applyBody["contract"].(map[string]any)
	require.Equal(t, "terminated", contract["status"])

	require.Equal(t, "terminated", getSalesTestContract(t, contractID)["status"])
}

func TestCompleteSalesContractRejectsOutstandingReceivables(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Outstanding Receivables Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	createSalesTestScheduleLine(t, contractID, "2026-05-01", "40000.00", "scheduled")
	createSalesTestScheduleLine(t, contractID, "2026-06-01", "60000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 2)
	markSalesTestReceivablePaid(t, lines[0]["receivable_id"])

	completeRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/complete", map[string]any{})
	require.Equal(t, http.StatusBadRequest, completeRR.Code, completeRR.Body.String())
	require.Equal(t, "active", getSalesTestContract(t, contractID)["status"])
}

func TestCompleteSalesContractRequiresLinkedReceivablesSettled(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Settled Receivables Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	createSalesTestScheduleLine(t, contractID, "2026-05-01", "40000.00", "scheduled")
	createSalesTestScheduleLine(t, contractID, "2026-06-01", "60000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 2)
	for _, line := range lines {
		markSalesTestReceivablePaid(t, line["receivable_id"])
	}

	completeRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/complete", map[string]any{})
	require.Equal(t, http.StatusOK, completeRR.Code, completeRR.Body.String())
	require.Equal(t, "completed", getSalesTestContract(t, contractID)["status"])
}

func TestMarkSalesContractDefaultedOnlyChangesContractState(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Defaulted Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	createSalesTestScheduleLine(t, contractID, "2026-05-01", "100000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 1)
	receivableID := lines[0]["receivable_id"]
	beforeStatus := getSalesTestReceivableStatus(t, receivableID)
	beforeOutstanding := getSalesTestReceivableOutstanding(t, receivableID)

	markRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/mark-default", map[string]any{"reason": "stopped paying"})
	require.Equal(t, http.StatusOK, markRR.Code, markRR.Body.String())

	require.Equal(t, "defaulted", getSalesTestContract(t, contractID)["status"])
	require.Equal(t, beforeStatus, getSalesTestReceivableStatus(t, receivableID))
	require.Equal(t, beforeOutstanding, getSalesTestReceivableOutstanding(t, receivableID))
}

func TestUpdateDraftScheduleLineMutatesImmediately(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Draft Schedule Edit Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	lineID := createSalesTestScheduleLine(t, contractID, "2026-05-01", "10000.00", "scheduled")

	rr := salesRequest(t, http.MethodPatch, "/api/v1/schedule-lines/"+lineID, map[string]any{
		"due_date":         "2026-07-01",
		"principal_amount": "11000.00",
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	body := decodeSalesTestObject(t, rr.Body)
	require.Equal(t, "2026-07-01", normalizeSalesTestJSONDate(t, body["due_date"]))
	require.Equal(t, "11000.00", normalizeSalesTestJSONAmount(t, body["principal_amount"]))
}

func TestUpdateActiveFutureScheduleLineCreatesApprovalRequest(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Active Future Schedule Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	futureDate := time.Now().Add(60 * 24 * time.Hour).Format("2006-01-02")
	createSalesTestScheduleLine(t, contractID, futureDate, "100000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 1)
	lineID := lines[0]["id"]

	newDate := time.Now().Add(90 * 24 * time.Hour).Format("2006-01-02")
	rr := salesRequest(t, http.MethodPatch, "/api/v1/schedule-lines/"+lineID, map[string]any{
		"due_date": newDate,
	})
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
	body := decodeSalesTestObject(t, rr.Body)
	approvalID, _ := body["approval_request_id"].(string)
	require.NotEmpty(t, approvalID)

	require.Equal(t, "pending", getSalesTestApprovalRequestStatus(t, approvalID))
}

func TestUpdatePostedScheduleLineIsRejected(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Posted Schedule Edit Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "draft")
	futureDate := time.Now().Add(60 * 24 * time.Hour).Format("2006-01-02")
	createSalesTestScheduleLine(t, contractID, futureDate, "100000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 1)
	lineID := lines[0]["id"]
	receivableID := lines[0]["receivable_id"]
	markSalesTestReceivablePaid(t, receivableID)

	rr := salesRequest(t, http.MethodPatch, "/api/v1/schedule-lines/"+lineID, map[string]any{
		"due_date": time.Now().Add(120 * 24 * time.Hour).Format("2006-01-02"),
	})
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
}

func TestSalesReservationToActivatedContractWorkflow(t *testing.T) {
	unitID := createSalesTestUnit(t)
	customerID := createSalesTestParty(t, "Workflow E2E Buyer")
	unit := getSalesTestUnit(t, unitID)

	reservationRR := salesRequest(t, http.MethodPost, "/api/v1/reservations", map[string]any{
		"business_entity_id":  unit["business_entity_id"],
		"branch_id":           unit["branch_id"],
		"project_id":          unit["project_id"],
		"unit_id":             unitID,
		"customer_id":         customerID,
		"expires_at":          time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"deposit_amount":      "10000.00",
		"deposit_currency":    "USD",
		"quoted_price_amount": "100000.00",
		"discount_amount":     "0.00",
	})
	require.Equal(t, http.StatusCreated, reservationRR.Code, reservationRR.Body.String())
	reservationID := responseID(t, reservationRR.Body)

	convertRR := salesRequest(t, http.MethodPost, "/api/v1/reservations/"+reservationID+"/convert", map[string]any{})
	require.Equal(t, http.StatusCreated, convertRR.Code, convertRR.Body.String())
	contract := decodeSalesTestObject(t, convertRR.Body)["contract"].(map[string]any)
	contractID := contract["id"].(string)

	createSalesTestScheduleLine(t, contractID, "2026-05-01", "100000.00", "scheduled")

	activateRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	require.Equal(t, "active", getSalesTestContract(t, contractID)["status"])
	lines := getSalesTestScheduleLineReceivables(t, contractID)
	require.Len(t, lines, 1)
	require.NotEmpty(t, lines[0]["receivable_id"])
}

func TestSalesApprovalGatedCancelWorkflow(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "E2E Cancel Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	requestRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/cancel", map[string]any{"reason": "buyer pulled out"})
	require.Equal(t, http.StatusAccepted, requestRR.Code, requestRR.Body.String())
	approvalID, _ := decodeSalesTestObject(t, requestRR.Body)["approval_request_id"].(string)
	require.NotEmpty(t, approvalID)

	require.Equal(t, "active", getSalesTestContract(t, contractID)["status"])

	approveSalesTestApprovalRequest(t, approvalID)
	applyRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/cancel", map[string]any{})
	require.Equal(t, http.StatusOK, applyRR.Code, applyRR.Body.String())
	require.Equal(t, "cancelled", getSalesTestContract(t, contractID)["status"])
}

func TestSalesDefaultAndCompletionWorkflow(t *testing.T) {
	t.Run("default path", func(t *testing.T) {
		unitID := createSalesTestUnit(t)
		buyerID := createSalesTestParty(t, "E2E Default Buyer")
		contractID := createSalesTestContract(t, unitID, buyerID, "draft")
		createSalesTestScheduleLine(t, contractID, "2026-05-01", "100000.00", "scheduled")
		require.Equal(t, http.StatusOK, salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{}).Code)

		lines := getSalesTestScheduleLineReceivables(t, contractID)
		require.Len(t, lines, 1)
		receivableID := lines[0]["receivable_id"]
		beforeStatus := getSalesTestReceivableStatus(t, receivableID)

		require.Equal(t, http.StatusOK, salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/mark-default", map[string]any{"reason": "stopped paying"}).Code)
		require.Equal(t, "defaulted", getSalesTestContract(t, contractID)["status"])
		require.Equal(t, beforeStatus, getSalesTestReceivableStatus(t, receivableID), "default must not mutate receivable")
	})

	t.Run("completion path", func(t *testing.T) {
		unitID := createSalesTestUnit(t)
		buyerID := createSalesTestParty(t, "E2E Completion Buyer")
		contractID := createSalesTestContract(t, unitID, buyerID, "draft")
		createSalesTestScheduleLine(t, contractID, "2026-05-01", "100000.00", "scheduled")
		require.Equal(t, http.StatusOK, salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/activate", map[string]any{}).Code)

		for _, line := range getSalesTestScheduleLineReceivables(t, contractID) {
			markSalesTestReceivablePaid(t, line["receivable_id"])
		}
		require.Equal(t, http.StatusOK, salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/complete", map[string]any{}).Code)
		require.Equal(t, "completed", getSalesTestContract(t, contractID)["status"])
	})
}

func TestSalesOwnershipTransferWorkflow(t *testing.T) {
	unitID := createSalesTestUnit(t)
	fromBuyerID := createSalesTestParty(t, "E2E Transfer From")
	contractID := createSalesTestContract(t, unitID, fromBuyerID, "active")
	toBuyerID := createSalesTestParty(t, "E2E Transfer To")
	createSalesTestUnitOwnership(t, unitID, fromBuyerID, "100.00")

	requestRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/transfer-request", map[string]any{
		"transfer_type":       "buyer_replacement",
		"from_party_id":       fromBuyerID,
		"to_party_id":         toBuyerID,
		"effective_date":      "2026-05-01",
		"financial_treatment": "no_change",
	})
	require.Equal(t, http.StatusCreated, requestRR.Code, requestRR.Body.String())
	body := decodeSalesTestObject(t, requestRR.Body)
	transferID, _ := body["id"].(string)
	approvalID, _ := body["approval_request_id"].(string)
	require.NotEmpty(t, transferID)
	require.NotEmpty(t, approvalID)

	approveSalesTestApprovalRequest(t, approvalID)
	require.Equal(t, http.StatusOK, salesRequest(t, http.MethodPost, "/api/v1/ownership-transfers/"+transferID+"/complete", map[string]any{}).Code)

	require.Equal(t, toBuyerID, getSalesTestContract(t, contractID)["primary_buyer_id"])
	require.Equal(t, "inactive", getSalesTestSalesContractParty(t, contractID, fromBuyerID)["status"])
	require.Equal(t, "active", getSalesTestSalesContractParty(t, contractID, toBuyerID)["status"])
	require.Equal(t, "inactive", getSalesTestUnitOwnership(t, unitID, fromBuyerID)["status"])
	require.Equal(t, "active", getSalesTestUnitOwnership(t, unitID, toBuyerID)["status"])
}

func TestSalesPermissionFailuresWorkflow(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "E2E Permission Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	restrictedToken := createRestrictedTestToken(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{"POST", "/api/v1/sales-contracts/" + contractID + "/cancel", map[string]any{"reason": "x"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/terminate", map[string]any{"reason": "x"}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/complete", map[string]any{}},
		{"POST", "/api/v1/sales-contracts/" + contractID + "/mark-default", map[string]any{"reason": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := salesRequestWithToken(t, restrictedToken, tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

func TestRequestOwnershipTransferRequiresActiveFromParty(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Original Owner Active")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	randomPartyID := createSalesTestParty(t, "Random Not Buyer")
	newBuyerID := createSalesTestParty(t, "Transfer Target")

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/transfer-request", map[string]any{
		"transfer_type":       "buyer_replacement",
		"from_party_id":       randomPartyID,
		"to_party_id":         newBuyerID,
		"effective_date":      "2026-05-01",
		"financial_treatment": "no_change",
	})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

func TestRequestOwnershipTransferCreatesApprovalLinkedToTransfer(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Linked Original Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")

	newBuyerID := createSalesTestParty(t, "Linked Target Buyer")

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/transfer-request", map[string]any{
		"transfer_type":       "buyer_replacement",
		"from_party_id":       buyerID,
		"to_party_id":         newBuyerID,
		"effective_date":      "2026-05-01",
		"financial_treatment": "no_change",
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	body := decodeSalesTestObject(t, rr.Body)
	transferID, _ := body["id"].(string)
	approvalID, _ := body["approval_request_id"].(string)
	require.NotEmpty(t, transferID)
	require.NotEmpty(t, approvalID)

	approval := getSalesTestApprovalRequest(t, approvalID)
	require.Equal(t, "ownership_transfer", approval["source_record_type"])
	require.Equal(t, transferID, approval["source_record_id"])
	require.Equal(t, "pending", approval["status"])
}

func TestCompleteOwnershipTransferRejectsUnapprovedTransfer(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "Unapproved From Buyer")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")
	newBuyerID := createSalesTestParty(t, "Unapproved To Buyer")

	requestRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/transfer-request", map[string]any{
		"transfer_type":       "buyer_replacement",
		"from_party_id":       buyerID,
		"to_party_id":         newBuyerID,
		"effective_date":      "2026-05-01",
		"financial_treatment": "no_change",
	})
	require.Equal(t, http.StatusCreated, requestRR.Code, requestRR.Body.String())
	transferID, _ := decodeSalesTestObject(t, requestRR.Body)["id"].(string)

	completeRR := salesRequest(t, http.MethodPost, "/api/v1/ownership-transfers/"+transferID+"/complete", map[string]any{})
	require.Equal(t, http.StatusBadRequest, completeRR.Code, completeRR.Body.String())
}

func TestCompleteOwnershipTransferUpdatesBuyerHistoryAndUnitOwnership(t *testing.T) {
	unitID := createSalesTestUnit(t)
	buyerID := createSalesTestParty(t, "From Buyer Side Effects")
	contractID := createSalesTestContract(t, unitID, buyerID, "active")
	newBuyerID := createSalesTestParty(t, "To Buyer Side Effects")
	createSalesTestUnitOwnership(t, unitID, buyerID, "100.00")

	requestRR := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+contractID+"/transfer-request", map[string]any{
		"transfer_type":       "buyer_replacement",
		"from_party_id":       buyerID,
		"to_party_id":         newBuyerID,
		"effective_date":      "2026-05-01",
		"financial_treatment": "no_change",
	})
	require.Equal(t, http.StatusCreated, requestRR.Code, requestRR.Body.String())
	body := decodeSalesTestObject(t, requestRR.Body)
	transferID, _ := body["id"].(string)
	approvalID, _ := body["approval_request_id"].(string)
	require.NotEmpty(t, transferID)
	require.NotEmpty(t, approvalID)

	approveSalesTestApprovalRequest(t, approvalID)

	completeRR := salesRequest(t, http.MethodPost, "/api/v1/ownership-transfers/"+transferID+"/complete", map[string]any{})
	require.Equal(t, http.StatusOK, completeRR.Code, completeRR.Body.String())

	require.Equal(t, "completed", getSalesTestOwnershipTransfer(t, transferID)["status"])

	contract := getSalesTestContract(t, contractID)
	require.Equal(t, newBuyerID, contract["primary_buyer_id"])

	oldParty := getSalesTestSalesContractParty(t, contractID, buyerID)
	require.Equal(t, "inactive", oldParty["status"])
	require.NotEmpty(t, oldParty["effective_to"])

	newParty := getSalesTestSalesContractParty(t, contractID, newBuyerID)
	require.Equal(t, "active", newParty["status"])
	require.Empty(t, newParty["effective_to"])

	oldOwnership := getSalesTestUnitOwnership(t, unitID, buyerID)
	require.Equal(t, "inactive", oldOwnership["status"])
	require.NotEmpty(t, oldOwnership["effective_to"])

	newOwnership := getSalesTestUnitOwnership(t, unitID, newBuyerID)
	require.Equal(t, "active", newOwnership["status"])
	require.Empty(t, newOwnership["effective_to"])
}

func TestCancelOrTerminateDoesNotCreateDuplicatePendingApprovals(t *testing.T) {
	unitID := createSalesTestUnit(t)
	cancelBuyerID := createSalesTestParty(t, "Cancel Dup Buyer")
	cancelContractID := createSalesTestContract(t, unitID, cancelBuyerID, "active")

	firstCancel := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+cancelContractID+"/cancel", map[string]any{"reason": "first"})
	require.Equal(t, http.StatusAccepted, firstCancel.Code, firstCancel.Body.String())
	firstID, _ := decodeSalesTestObject(t, firstCancel.Body)["approval_request_id"].(string)
	require.NotEmpty(t, firstID)

	secondCancel := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+cancelContractID+"/cancel", map[string]any{"reason": "second"})
	require.Equal(t, http.StatusAccepted, secondCancel.Code, secondCancel.Body.String())
	secondID, _ := decodeSalesTestObject(t, secondCancel.Body)["approval_request_id"].(string)
	require.Equal(t, firstID, secondID, "duplicate cancel must reuse pending approval")

	require.Equal(t, 1, countSalesTestApprovalsForContract(t, cancelContractID, "contract_cancellation"))

	unitID2 := createSalesTestUnit(t)
	terminateBuyerID := createSalesTestParty(t, "Terminate Dup Buyer")
	terminateContractID := createSalesTestContract(t, unitID2, terminateBuyerID, "active")

	firstTerm := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+terminateContractID+"/terminate", map[string]any{"reason": "first"})
	require.Equal(t, http.StatusAccepted, firstTerm.Code, firstTerm.Body.String())
	firstTermID, _ := decodeSalesTestObject(t, firstTerm.Body)["approval_request_id"].(string)
	require.NotEmpty(t, firstTermID)

	secondTerm := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts/"+terminateContractID+"/terminate", map[string]any{"reason": "second"})
	require.Equal(t, http.StatusAccepted, secondTerm.Code, secondTerm.Body.String())
	secondTermID, _ := decodeSalesTestObject(t, secondTerm.Body)["approval_request_id"].(string)
	require.Equal(t, firstTermID, secondTermID, "duplicate terminate must reuse pending approval")

	require.Equal(t, 1, countSalesTestApprovalsForContract(t, terminateContractID, "contract_termination"))
}

func createRestrictedTestToken(t *testing.T) string {
	t.Helper()

	_, token := CreateSecondTestUser(t)
	return token
}

func createSalesTestBusinessEntity(t *testing.T) string {
	t.Helper()

	timestamp := time.Now().UnixNano()
	return CreateTestBusinessEntity(t, testApp, testToken, fmt.Sprintf("SALESENTITY_%d", timestamp), "Sales Test Entity")
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
			"down_payment": map[string]any{"percentage": 10, "anchor": "reservation"},
			"tranches": []map[string]any{
				{"percentage": 90, "anchor": "contract_date", "installment_count": 12, "frequency": "monthly"},
			},
		}
	}

	body := map[string]any{
		"business_entity_id":   businessEntityID,
		"code":                 fmt.Sprintf("SALESPLAN_%d", time.Now().UnixNano()),
		"name":                 "Sales Test Payment Plan",
		"status":               "active",
		"frequency_type":       "monthly",
		"installment_count":    salesTestRuleInstallmentCount(t, rule),
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
		"discount_amount":    "0.00",
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

func createSalesTestContractWithTemplate(t *testing.T, unitID string, primaryBuyerID string, templateID string, salePrice string, discount string, downPayment string) string {
	t.Helper()

	saleRat := parseSalesTestAmount(t, salePrice)
	discountRat := parseSalesTestAmount(t, discount)
	downRat := parseSalesTestAmount(t, downPayment)
	netRat := new(big.Rat).Sub(saleRat, discountRat)
	financedRat := new(big.Rat).Sub(netRat, downRat)

	unit := getSalesTestUnit(t, unitID)
	body := map[string]any{
		"business_entity_id":       unit["business_entity_id"],
		"branch_id":                unit["branch_id"],
		"project_id":               unit["project_id"],
		"unit_id":                  unitID,
		"primary_buyer_id":         primaryBuyerID,
		"contract_date":            "2026-04-24",
		"effective_date":           "2026-04-24",
		"sale_price_amount":        salePrice,
		"sale_price_currency":      "USD",
		"discount_amount":          discount,
		"net_contract_amount":      formatSalesTestAmount(t, netRat),
		"down_payment_amount":      downPayment,
		"financed_amount":          formatSalesTestAmount(t, financedRat),
		"payment_plan_template_id": templateID,
	}

	rr := salesRequest(t, http.MethodPost, "/api/v1/sales-contracts", body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	return responseID(t, rr.Body)
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

func createSalesTestPayment(t *testing.T, businessEntityID string, branchID string, partyID string, amount string) string {
	t.Helper()

	rr := salesRequest(t, http.MethodPost, "/api/v1/payments", map[string]any{
		"business_entity_id": businessEntityID,
		"branch_id":          branchID,
		"party_id":           partyID,
		"payment_date":       "2026-04-24",
		"payment_method":     "cash",
		"amount_received":    amount,
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	return responseID(t, rr.Body)
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

type decodedScheduleLine struct {
	LineType        string
	PrincipalAmount string
	DueDate         string
}

func decodeScheduleLines(t *testing.T, body *bytes.Buffer) []decodedScheduleLine {
	t.Helper()

	var response struct {
		Data []map[string]any `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body.Bytes()))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&response))

	lines := make([]decodedScheduleLine, 0, len(response.Data))
	for _, raw := range response.Data {
		lines = append(lines, decodedScheduleLine{
			LineType:        raw["line_type"].(string),
			PrincipalAmount: normalizeSalesTestJSONAmount(t, raw["principal_amount"]),
			DueDate:         normalizeSalesTestJSONDate(t, raw["due_date"]),
		})
	}
	return lines
}

func decodeSalesTestObject(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()

	var response map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body.Bytes()))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&response))
	return response
}

func getSalesTestScheduleLineReceivables(t *testing.T, contractID string) []map[string]string {
	t.Helper()

	contractUUID := uuid.MustParse(contractID)
	p := pool.Get()
	require.NotNil(t, p)

	var lines []map[string]string
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT id::text, COALESCE(receivable_id::text, ''), principal_amount::text
			FROM installment_schedule_lines
			WHERE sales_contract_id = $1
			ORDER BY line_no
		`, contractUUID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, receivableID, principalAmount string
			if err := rows.Scan(&id, &receivableID, &principalAmount); err != nil {
				return err
			}
			lines = append(lines, map[string]string{
				"id":               id,
				"receivable_id":    receivableID,
				"principal_amount": principalAmount,
			})
		}
		return rows.Err()
	})
	require.NoError(t, err)
	return lines
}

func getSalesTestReceivable(t *testing.T, receivableID string) map[string]string {
	t.Helper()

	receivableUUID := uuid.MustParse(receivableID)
	p := pool.Get()
	require.NotNil(t, p)

	receivable := map[string]string{}
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var sourceModule, sourceRecordType, sourceRecordID, originalAmount string
		if err := tx.QueryRow(context.Background(), `
			SELECT source_module, source_record_type, source_record_id::text, original_amount::text
			FROM receivables
			WHERE id = $1
		`, receivableUUID).Scan(&sourceModule, &sourceRecordType, &sourceRecordID, &originalAmount); err != nil {
			return err
		}
		receivable = map[string]string{
			"source_module":      sourceModule,
			"source_record_type": sourceRecordType,
			"source_record_id":   sourceRecordID,
			"original_amount":    originalAmount,
		}
		return nil
	})
	require.NoError(t, err)
	return receivable
}

func parseSalesTestAmount(t *testing.T, value string) *big.Rat {
	t.Helper()

	amount, ok := new(big.Rat).SetString(value)
	require.True(t, ok, "invalid amount %q", value)
	return amount
}

func formatSalesTestAmount(t *testing.T, amount *big.Rat) string {
	t.Helper()

	cents := new(big.Rat).Mul(amount, big.NewRat(100, 1))
	require.Equal(t, int64(1), cents.Denom().Int64())
	return formatSalesTestCents(cents.Num().Int64())
}

func normalizeSalesTestAmountString(t *testing.T, value string) string {
	t.Helper()

	amount, ok := new(big.Rat).SetString(value)
	require.True(t, ok)
	return formatSalesTestAmount(t, amount)
}

func normalizeSalesTestJSONAmount(t *testing.T, value any) string {
	t.Helper()

	switch amount := value.(type) {
	case string:
		return amount
	case json.Number:
		rat, ok := new(big.Rat).SetString(amount.String())
		require.True(t, ok)
		return formatSalesTestAmount(t, rat)
	case float64:
		return fmt.Sprintf("%.2f", amount)
	default:
		t.Fatalf("unexpected amount value %T: %v", value, value)
		return ""
	}
}

func normalizeSalesTestJSONDate(t *testing.T, value any) string {
	t.Helper()

	date, ok := value.(string)
	require.True(t, ok, "unexpected date value %T: %v", value, value)
	if len(date) >= len("2006-01-02") {
		return date[:len("2006-01-02")]
	}
	return date
}

func formatSalesTestCents(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func salesTestRuleInstallmentCount(t *testing.T, rule map[string]any) int32 {
	t.Helper()

	var total int32
	rawTranches, ok := rule["tranches"]
	require.True(t, ok)

	switch tranches := rawTranches.(type) {
	case []map[string]any:
		for _, tranche := range tranches {
			total += salesTestAnyToInt32(t, tranche["installment_count"])
		}
	case []any:
		for _, raw := range tranches {
			tranche, ok := raw.(map[string]any)
			require.True(t, ok)
			total += salesTestAnyToInt32(t, tranche["installment_count"])
		}
	default:
		t.Fatalf("unexpected tranches type %T", rawTranches)
	}
	return total
}

func salesTestAnyToInt32(t *testing.T, value any) int32 {
	t.Helper()

	switch v := value.(type) {
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	case float64:
		return int32(v)
	case json.Number:
		i, err := v.Int64()
		require.NoError(t, err)
		return int32(i)
	default:
		t.Fatalf("unexpected int value %T: %v", value, value)
		return 0
	}
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

func getSalesTestContract(t *testing.T, contractID string) map[string]any {
	t.Helper()

	rr := salesRequest(t, http.MethodGet, "/api/v1/sales-contracts/"+contractID, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var contract map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&contract))
	return contract
}

func approveSalesTestApprovalRequest(t *testing.T, approvalRequestID string) {
	t.Helper()

	approvalUUID := uuid.MustParse(approvalRequestID)
	p := pool.Get()
	require.NotNil(t, p)

	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE approval_requests
			SET status = 'approved',
			    decided_at = timezone('utc', now()),
			    updated_at = timezone('utc', now())
			WHERE id = $1
		`, approvalUUID)
		return err
	})
	require.NoError(t, err)
}

func getSalesTestApprovalRequestStatus(t *testing.T, approvalRequestID string) string {
	t.Helper()

	approvalUUID := uuid.MustParse(approvalRequestID)
	p := pool.Get()
	require.NotNil(t, p)

	var status string
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status FROM approval_requests WHERE id = $1`, approvalUUID).Scan(&status)
	})
	require.NoError(t, err)
	return status
}

func countSalesTestApprovalsForContract(t *testing.T, contractID string, requestType string) int {
	t.Helper()

	contractUUID := uuid.MustParse(contractID)
	p := pool.Get()
	require.NotNil(t, p)

	var count int
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM approval_requests
			WHERE module = 'sales'
			  AND source_record_type = 'sales_contract'
			  AND source_record_id = $1
			  AND request_type = $2
		`, contractUUID, requestType).Scan(&count)
	})
	require.NoError(t, err)
	return count
}

func markSalesTestReceivablePaid(t *testing.T, receivableID string) {
	t.Helper()

	receivableUUID := uuid.MustParse(receivableID)
	p := pool.Get()
	require.NotNil(t, p)

	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE receivables
			SET paid_amount = original_amount,
			    status = 'paid',
			    updated_at = timezone('utc', now())
			WHERE id = $1
		`, receivableUUID)
		return err
	})
	require.NoError(t, err)
}

func getSalesTestReceivableStatus(t *testing.T, receivableID string) string {
	t.Helper()

	receivableUUID := uuid.MustParse(receivableID)
	p := pool.Get()
	require.NotNil(t, p)

	var status string
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status FROM receivables WHERE id = $1`, receivableUUID).Scan(&status)
	})
	require.NoError(t, err)
	return status
}

func getSalesTestReceivableOutstanding(t *testing.T, receivableID string) string {
	t.Helper()

	receivableUUID := uuid.MustParse(receivableID)
	p := pool.Get()
	require.NotNil(t, p)

	var outstanding string
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT outstanding_amount::text FROM receivables WHERE id = $1`, receivableUUID).Scan(&outstanding)
	})
	require.NoError(t, err)
	return outstanding
}

func getSalesTestApprovalRequest(t *testing.T, approvalRequestID string) map[string]string {
	t.Helper()

	approvalUUID := uuid.MustParse(approvalRequestID)
	p := pool.Get()
	require.NotNil(t, p)

	row := map[string]string{}
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var module, requestType, sourceRecordType, sourceRecordID, status string
		if err := tx.QueryRow(context.Background(), `
			SELECT module, request_type, source_record_type, source_record_id::text, status
			FROM approval_requests
			WHERE id = $1
		`, approvalUUID).Scan(&module, &requestType, &sourceRecordType, &sourceRecordID, &status); err != nil {
			return err
		}
		row = map[string]string{
			"module":             module,
			"request_type":       requestType,
			"source_record_type": sourceRecordType,
			"source_record_id":   sourceRecordID,
			"status":             status,
		}
		return nil
	})
	require.NoError(t, err)
	return row
}

func getSalesTestOwnershipTransfer(t *testing.T, transferID string) map[string]string {
	t.Helper()

	transferUUID := uuid.MustParse(transferID)
	p := pool.Get()
	require.NotNil(t, p)

	row := map[string]string{}
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var status, fromPartyID, toPartyID, salesContractID string
		var approvalRequestID *string
		if err := tx.QueryRow(context.Background(), `
			SELECT status, from_party_id::text, to_party_id::text, sales_contract_id::text, approval_request_id::text
			FROM ownership_transfers
			WHERE id = $1
		`, transferUUID).Scan(&status, &fromPartyID, &toPartyID, &salesContractID, &approvalRequestID); err != nil {
			return err
		}
		row = map[string]string{
			"status":            status,
			"from_party_id":     fromPartyID,
			"to_party_id":       toPartyID,
			"sales_contract_id": salesContractID,
		}
		if approvalRequestID != nil {
			row["approval_request_id"] = *approvalRequestID
		}
		return nil
	})
	require.NoError(t, err)
	return row
}

func getSalesTestSalesContractParty(t *testing.T, contractID string, partyID string) map[string]string {
	t.Helper()

	contractUUID := uuid.MustParse(contractID)
	partyUUID := uuid.MustParse(partyID)
	p := pool.Get()
	require.NotNil(t, p)

	row := map[string]string{}
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var status, role string
		var effectiveTo *string
		if err := tx.QueryRow(context.Background(), `
			SELECT status, role, effective_to::text
			FROM sales_contract_parties
			WHERE sales_contract_id = $1 AND party_id = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, contractUUID, partyUUID).Scan(&status, &role, &effectiveTo); err != nil {
			return err
		}
		row = map[string]string{"status": status, "role": role}
		if effectiveTo != nil {
			row["effective_to"] = *effectiveTo
		} else {
			row["effective_to"] = ""
		}
		return nil
	})
	require.NoError(t, err)
	return row
}

func createSalesTestUnitOwnership(t *testing.T, unitID string, partyID string, sharePercentage string) string {
	t.Helper()

	unitUUID := uuid.MustParse(unitID)
	partyUUID := uuid.MustParse(partyID)
	p := pool.Get()
	require.NotNil(t, p)

	var ownershipID string
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			INSERT INTO unit_ownerships (unit_id, party_id, share_percentage, effective_from, status)
			VALUES ($1, $2, $3::numeric, now()::date, 'active')
			RETURNING id::text
		`, unitUUID, partyUUID, sharePercentage).Scan(&ownershipID)
	})
	require.NoError(t, err)
	return ownershipID
}

func getSalesTestUnitOwnership(t *testing.T, unitID string, partyID string) map[string]string {
	t.Helper()

	unitUUID := uuid.MustParse(unitID)
	partyUUID := uuid.MustParse(partyID)
	p := pool.Get()
	require.NotNil(t, p)

	row := map[string]string{}
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var status, sharePercentage string
		var effectiveTo *string
		if err := tx.QueryRow(context.Background(), `
			SELECT status, share_percentage::text, effective_to::text
			FROM unit_ownerships
			WHERE unit_id = $1 AND party_id = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, unitUUID, partyUUID).Scan(&status, &sharePercentage, &effectiveTo); err != nil {
			return err
		}
		row = map[string]string{"status": status, "share_percentage": sharePercentage}
		if effectiveTo != nil {
			row["effective_to"] = *effectiveTo
		} else {
			row["effective_to"] = ""
		}
		return nil
	})
	require.NoError(t, err)
	return row
}

func expireSalesTestReservation(t *testing.T, reservationID string) {
	t.Helper()

	recordID := uuid.MustParse(reservationID)
	p := pool.Get()
	require.NotNil(t, p)

	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE reservations
			SET reserved_at = timezone('utc', now()) - interval '2 hours',
				expires_at = timezone('utc', now()) - interval '1 hour',
				updated_at = timezone('utc', now())
			WHERE id = $1
		`, recordID)
		return err
	})
	require.NoError(t, err)
}
