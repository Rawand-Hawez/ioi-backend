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

// =============================================================================
// Phase 8 harness + permission gating
// =============================================================================

func TestRentalsPhase8Harness(t *testing.T) {
	require.NotNil(t, testApp)
	require.NotEmpty(t, testToken)
}

func TestRentalsSchemaExists(t *testing.T) {
	ctx := context.Background()
	requiredTables := []string{"lease_contracts", "lease_parties", "lease_bills"}
	for _, table := range requiredTables {
		t.Run(table, func(t *testing.T) {
			var exists bool
			err := pool.Get().QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = 'public' AND table_name = $1
				)
			`, table).Scan(&exists)
			require.NoError(t, err)
			require.True(t, exists)
		})
	}
}

func TestRentalsWriteRoutesRequirePermissions(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Restricted Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "draft")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")
	restrictedToken := createRestrictedTestToken(t)

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{"POST", "/api/v1/lease-contracts", map[string]any{"unit_id": unitID, "primary_tenant_id": tenantID}},
		{"PATCH", "/api/v1/lease-contracts/" + leaseID, map[string]any{"notes": "blocked"}},
		{"POST", "/api/v1/lease-contracts/" + leaseID + "/activate", map[string]any{}},
		{"POST", "/api/v1/lease-contracts/" + leaseID + "/terminate", map[string]any{"reason": "blocked"}},
		{"POST", "/api/v1/lease-contracts/" + leaseID + "/renew", map[string]any{}},
		{"POST", "/api/v1/lease-contracts/" + leaseID + "/deposit-refund", map[string]any{"amount": "100.00", "reason": "blocked"}},
		{"POST", "/api/v1/lease-contracts/" + leaseID + "/bills/generate", map[string]any{}},
		{"POST", "/api/v1/lease-bills/" + billID + "/issue", map[string]any{}},
		{"POST", "/api/v1/lease-bills/" + billID + "/void", map[string]any{"reason": "blocked"}},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := rentalRequestWithToken(t, restrictedToken, tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

func TestRentalsWriteRoutesRequireMatchingScope(t *testing.T) {
	sourceUnitID := createRentalTestUnit(t)
	sourceUnit := getSalesTestUnit(t, sourceUnitID)
	targetUnitID := createRentalTestUnit(t)
	targetUnit := getSalesTestUnit(t, targetUnitID)
	tenantID := createRentalTestParty(t, "Scoped Tenant")

	scopedUserID, scopedToken := CreateSecondTestUser(t)
	AssignRoleToUserByID(t, testApp, testToken, scopedUserID, sourceUnit["business_entity_id"].(string), "leasing_officer")

	rr := rentalRequestWithToken(t, scopedToken, http.MethodPost, "/api/v1/lease-contracts", map[string]any{
		"business_entity_id":      targetUnit["business_entity_id"],
		"branch_id":               targetUnit["branch_id"],
		"project_id":              targetUnit["project_id"],
		"unit_id":                 targetUnitID,
		"primary_tenant_id":       tenantID,
		"lease_type":              "residential",
		"start_date":              "2026-05-01",
		"end_date":                "2027-04-30",
		"rent_pricing_basis":      "fixed_amount",
		"contractual_rent_amount": "12000.00",
		"billing_interval_value":  1,
		"billing_interval_unit":   "month",
		"billing_anchor_date":     "2026-05-01",
		"security_deposit_amount": "1000.00",
		"advance_rent_amount":     "0.00",
		"currency_code":           "USD",
	})
	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestRentalsRLSBlocksDirectAuthenticatedWrites(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "RLS Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "draft")

	jsonBody, err := json.Marshal(map[string]any{"status": "active"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPatch, PostgRESTURL+"/lease_contracts?id=eq."+leaseID, bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var status string
	err = pool.Get().QueryRow(context.Background(), `
		SELECT status FROM public.lease_contracts WHERE id = $1
	`, leaseID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "draft", status, "PostgREST authenticated writes must not bypass Fiber approval, permission, and audit paths")
}

// =============================================================================
// CRUD
// =============================================================================

func TestCreateGetListAndPatchDraftLeaseContract(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "CRUD Tenant")
	unit := getSalesTestUnit(t, unitID)

	createRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts", map[string]any{
		"business_entity_id":      unit["business_entity_id"],
		"branch_id":               unit["branch_id"],
		"project_id":              unit["project_id"],
		"unit_id":                 unitID,
		"primary_tenant_id":       tenantID,
		"lease_type":              "residential",
		"start_date":              "2026-05-01",
		"end_date":                "2027-04-30",
		"rent_pricing_basis":      "fixed_amount",
		"contractual_rent_amount": "12000.00",
		"billing_interval_value":  1,
		"billing_interval_unit":   "month",
		"billing_anchor_date":     "2026-05-01",
		"security_deposit_amount": "1000.00",
		"advance_rent_amount":     "0.00",
		"currency_code":           "USD",
	})
	require.Equal(t, http.StatusCreated, createRR.Code, createRR.Body.String())
	created := decodeRentalTestObject(t, createRR.Body)
	require.Equal(t, "draft", created["status"])
	leaseNo, _ := created["lease_no"].(string)
	require.Contains(t, leaseNo, "LSE-")

	leaseID := responseID(t, createRR.Body)
	getRR := rentalRequest(t, http.MethodGet, "/api/v1/lease-contracts/"+leaseID, nil)
	require.Equal(t, http.StatusOK, getRR.Code, getRR.Body.String())

	listRR := rentalRequest(t, http.MethodGet, "/api/v1/lease-contracts?status=draft", nil)
	require.Equal(t, http.StatusOK, listRR.Code, listRR.Body.String())
	listResp := decodeRentalTestObject(t, listRR.Body)
	pagination, ok := listResp["pagination"].(map[string]any)
	require.True(t, ok, "list response missing pagination key")
	require.NotNil(t, pagination["total_count"])
	require.NotNil(t, pagination["total_pages"])

	patchRR := rentalRequest(t, http.MethodPatch, "/api/v1/lease-contracts/"+leaseID, map[string]any{"notes": "updated"})
	require.Equal(t, http.StatusOK, patchRR.Code, patchRR.Body.String())
}

func TestPatchActiveLeaseContractIsBlocked(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Patch Active Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "draft")

	activateRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	patchRR := rentalRequest(t, http.MethodPatch, "/api/v1/lease-contracts/"+leaseID, map[string]any{"notes": "should fail"})
	require.Equal(t, http.StatusConflict, patchRR.Code, patchRR.Body.String())
}

// =============================================================================
// Activation
// =============================================================================

func TestActivateLeaseCreatesPrimaryLeasePartyAndMarksUnitOccupied(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Activation Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "draft")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var leaseStatus, occupancyStatus string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT lc.status, u.occupancy_status
		FROM public.lease_contracts lc
		JOIN public.units u ON u.id = lc.unit_id
		WHERE lc.id = $1
	`, leaseID).Scan(&leaseStatus, &occupancyStatus)
	require.NoError(t, err)
	require.Equal(t, "active", leaseStatus)
	require.Equal(t, "occupied", occupancyStatus)

	var partyCount int
	err = pool.Get().QueryRow(context.Background(), `
		SELECT count(*) FROM public.lease_parties
		WHERE lease_contract_id = $1 AND party_id = $2 AND role = 'primary_tenant' AND is_primary = true
	`, leaseID, tenantID).Scan(&partyCount)
	require.NoError(t, err)
	require.Equal(t, 1, partyCount)
}

func TestActivateLeasePreventsOverlappingActiveLease(t *testing.T) {
	unitID := createRentalTestUnit(t)
	firstTenantID := createRentalTestParty(t, "First Tenant")
	secondTenantID := createRentalTestParty(t, "Second Tenant")
	firstLeaseID := createRentalTestLease(t, unitID, firstTenantID, "draft")
	secondLeaseID := createRentalTestLease(t, unitID, secondTenantID, "draft")

	firstRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+firstLeaseID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, firstRR.Code, firstRR.Body.String())

	secondRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+secondLeaseID+"/activate", map[string]any{})
	require.Equal(t, http.StatusConflict, secondRR.Code, secondRR.Body.String())
}

// =============================================================================
// Billing schedule + bill generation
// =============================================================================

func TestGenerateLeaseBillsMonthlyOneYearProduces12Bills(t *testing.T) {
	// createRentalTestLease helper sets advance_rent_amount = 0, so the 12-month lease
	// produces exactly 12 monthly bills (no extra advance bill is appended).
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Billing Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/bills/generate", map[string]any{})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var billCount int
	var firstAmount string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT count(*), min(billed_amount)::text
		FROM public.lease_bills
		WHERE lease_contract_id = $1
	`, leaseID).Scan(&billCount, &firstAmount)
	require.NoError(t, err)
	require.Equal(t, 12, billCount)
	require.Equal(t, "1000.00", normalizeRentalTestJSONAmount(t, firstAmount))
}

func TestGenerateLeaseBillsIsIdempotent(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Idempotent Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	first := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/bills/generate", map[string]any{})
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/bills/generate", map[string]any{})
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())

	var billCount int
	err := pool.Get().QueryRow(context.Background(), `
		SELECT count(*) FROM public.lease_bills WHERE lease_contract_id = $1
	`, leaseID).Scan(&billCount)
	require.NoError(t, err)
	require.Equal(t, 12, billCount)
}

func TestGenerateLeaseBillsWithAdvanceRentKeepsFirstRentBill(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Advance Billing Tenant")
	unit := getSalesTestUnit(t, unitID)

	createRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts", map[string]any{
		"business_entity_id":      unit["business_entity_id"],
		"branch_id":               unit["branch_id"],
		"project_id":              unit["project_id"],
		"unit_id":                 unitID,
		"primary_tenant_id":       tenantID,
		"lease_type":              "residential",
		"start_date":              "2026-05-01",
		"end_date":                "2027-04-30",
		"rent_pricing_basis":      "fixed_amount",
		"contractual_rent_amount": "12000.00",
		"billing_interval_value":  1,
		"billing_interval_unit":   "month",
		"billing_anchor_date":     "2026-05-01",
		"security_deposit_amount": "1000.00",
		"advance_rent_amount":     "500.00",
		"currency_code":           "USD",
	})
	require.Equal(t, http.StatusCreated, createRR.Code, createRR.Body.String())
	leaseID := responseID(t, createRR.Body)

	activateRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/activate", map[string]any{})
	require.Equal(t, http.StatusOK, activateRR.Code, activateRR.Body.String())

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/bills/generate", map[string]any{})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var totalBills, advanceBills, rentBills int
	err := pool.Get().QueryRow(context.Background(), `
		SELECT count(*),
		       count(*) FILTER (WHERE is_advance),
		       count(*) FILTER (WHERE NOT is_advance)
		FROM public.lease_bills
		WHERE lease_contract_id = $1
	`, leaseID).Scan(&totalBills, &advanceBills, &rentBills)
	require.NoError(t, err)
	require.Equal(t, 13, totalBills)
	require.Equal(t, 1, advanceBills)
	require.Equal(t, 12, rentBills)
}

func TestGetLeaseBillingSchedulePreviewMatches12Periods(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Preview Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodGet, "/api/v1/lease-contracts/"+leaseID+"/billing-schedule", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	resp := decodeRentalTestObject(t, rr.Body)
	data, ok := resp["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 12)
}

// =============================================================================
// Issue + Void
// =============================================================================

func TestIssueLeaseBillCreatesReceivableAndLinksIt(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Issue Bill Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/issue", map[string]any{})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var billStatus, sourceModule, sourceType string
	var receivableID string
	var receivableAmount string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT lb.status, lb.receivable_id::text, r.source_module, r.source_record_type, r.original_amount::text
		FROM public.lease_bills lb
		JOIN public.receivables r ON r.id = lb.receivable_id
		WHERE lb.id = $1
	`, billID).Scan(&billStatus, &receivableID, &sourceModule, &sourceType, &receivableAmount)
	require.NoError(t, err)
	require.Equal(t, "issued", billStatus)
	require.NotEmpty(t, receivableID)
	require.Equal(t, "rentals", sourceModule)
	require.Equal(t, "lease_bill", sourceType)
	require.Equal(t, "1000.00", normalizeRentalTestJSONAmount(t, receivableAmount))
}

func TestIssueLeaseBillTwiceFailsOnSecondCall(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Double Issue Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	first := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/issue", map[string]any{})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/issue", map[string]any{})
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())
}

func TestVoidDraftLeaseBillDoesNotCreateReceivable(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Void Draft Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/void", map[string]any{"reason": "duplicate"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var status string
	var receivableID *string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT status, receivable_id::text FROM public.lease_bills WHERE id = $1
	`, billID).Scan(&status, &receivableID)
	require.NoError(t, err)
	require.Equal(t, "voided", status)
	require.Nil(t, receivableID)
}

func TestVoidIssuedLeaseBillVoidsOpenReceivable(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Void Issued Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	issueRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/issue", map[string]any{})
	require.Equal(t, http.StatusOK, issueRR.Code, issueRR.Body.String())

	voidRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/void", map[string]any{"reason": "mistake"})
	require.Equal(t, http.StatusOK, voidRR.Code, voidRR.Body.String())

	var billStatus, receivableStatus string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT lb.status, r.status
		FROM public.lease_bills lb
		JOIN public.receivables r ON r.id = lb.receivable_id
		WHERE lb.id = $1
	`, billID).Scan(&billStatus, &receivableStatus)
	require.NoError(t, err)
	require.Equal(t, "voided", billStatus)
	require.Equal(t, "voided", receivableStatus)
}

// =============================================================================
// Termination (Phase 7 pattern: lease stays 'active' during pending approval)
// =============================================================================

func TestTerminateActiveLeaseCreatesApprovalRequestLeaseStaysActive(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Termination Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{
		"reason": "tenant requested early termination",
	})
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())

	response := decodeRentalTestObject(t, rr.Body)
	require.Equal(t, "approval_requested", response["action"])
	require.NotEmpty(t, response["approval_request_id"])

	var leaseStatus, approvalStatus, requestType string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT lc.status, ar.status, ar.request_type
		FROM public.lease_contracts lc
		JOIN public.approval_requests ar ON ar.source_record_id = lc.id
		WHERE lc.id = $1 AND ar.module = 'rentals' AND ar.request_type = 'lease_termination'
		ORDER BY ar.created_at DESC LIMIT 1
	`, leaseID).Scan(&leaseStatus, &approvalStatus, &requestType)
	require.NoError(t, err)
	require.Equal(t, "active", leaseStatus, "lease must stay active during pending approval (Phase 7 pattern)")
	require.Equal(t, "pending", approvalStatus)
	require.Equal(t, "lease_termination", requestType)
}

func TestTerminateLeaseReusesPendingApprovalRequest(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Reuse Termination Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	first := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "first"})
	require.Equal(t, http.StatusAccepted, first.Code, first.Body.String())
	firstResp := decodeRentalTestObject(t, first.Body)
	firstID := firstResp["approval_request_id"]

	second := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "second"})
	require.Equal(t, http.StatusAccepted, second.Code, second.Body.String())
	secondResp := decodeRentalTestObject(t, second.Body)
	require.Equal(t, firstID, secondResp["approval_request_id"], "second call must reuse the same pending approval")

	var count int
	err := pool.Get().QueryRow(context.Background(), `
		SELECT count(*) FROM public.approval_requests
		WHERE module='rentals' AND request_type='lease_termination' AND source_record_id = $1
	`, leaseID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestTerminateLeaseAfterApprovalAppliesAndClosesAllParties(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Apply Termination Tenant")
	guarantorID := createRentalTestParty(t, "Apply Termination Guarantor")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	addRentalTestLeaseParty(t, leaseID, guarantorID, "guarantor", false)

	requestRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "apply"})
	require.Equal(t, http.StatusAccepted, requestRR.Code, requestRR.Body.String())
	requestResp := decodeRentalTestObject(t, requestRR.Body)
	approvalID := requestResp["approval_request_id"].(string)

	// Manually mark approval approved (Phase 6 decide flow not exercised here).
	_, err := pool.Get().Exec(context.Background(), `
		UPDATE public.approval_requests SET status='approved', decided_at=timezone('utc', now()) WHERE id = $1
	`, approvalID)
	require.NoError(t, err)

	applyRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "apply"})
	require.Equal(t, http.StatusOK, applyRR.Code, applyRR.Body.String())

	var leaseStatus, occupancy string
	err = pool.Get().QueryRow(context.Background(), `
		SELECT lc.status, u.occupancy_status
		FROM public.lease_contracts lc JOIN public.units u ON u.id = lc.unit_id
		WHERE lc.id = $1
	`, leaseID).Scan(&leaseStatus, &occupancy)
	require.NoError(t, err)
	require.Equal(t, "terminated", leaseStatus)
	require.Equal(t, "vacant", occupancy)

	var openCount int
	err = pool.Get().QueryRow(context.Background(), `
		SELECT count(*) FROM public.lease_parties WHERE lease_contract_id = $1 AND status = 'active'
	`, leaseID).Scan(&openCount)
	require.NoError(t, err)
	require.Equal(t, 0, openCount, "all lease_parties (tenant + guarantor) must be closed on termination")
}

func TestTerminateLeaseAfterRejectionReturnsConflict(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Rejection Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	first := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "first"})
	require.Equal(t, http.StatusAccepted, first.Code, first.Body.String())
	approvalID := decodeRentalTestObject(t, first.Body)["approval_request_id"].(string)

	_, err := pool.Get().Exec(context.Background(), `
		UPDATE public.approval_requests SET status='rejected', decided_at=timezone('utc', now()) WHERE id = $1
	`, approvalID)
	require.NoError(t, err)

	second := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "second"})
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())
}

// =============================================================================
// Renewal
// =============================================================================

func TestRenewLeaseMarksOldLeaseRenewedAndCreatesDraftRenewal(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Renewal Tenant")
	oldLeaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+oldLeaseID+"/renew", map[string]any{
		"start_date":              "2027-05-01",
		"end_date":                "2028-04-30",
		"contractual_rent_amount": "13200.00",
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	response := decodeRentalTestObject(t, rr.Body)
	newLease, ok := response["lease"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "draft", newLease["status"])
	require.Equal(t, oldLeaseID, newLease["renewed_from_lease_contract_id"])

	var oldStatus string
	err := pool.Get().QueryRow(context.Background(), `
		SELECT status FROM public.lease_contracts WHERE id = $1
	`, oldLeaseID).Scan(&oldStatus)
	require.NoError(t, err)
	require.Equal(t, "renewed", oldStatus)
}

// =============================================================================
// Deposit refund
// =============================================================================

func TestLeaseDepositRefundCreatesApprovalRequest(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Deposit Refund Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/deposit-refund", map[string]any{
		"amount": "500.00",
		"reason": "partial deposit release",
	})
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
	response := decodeRentalTestObject(t, rr.Body)
	require.Equal(t, "approval_requested", response["action"])
	require.NotEmpty(t, response["approval_request_id"])

	var requestType string
	var payload []byte
	err := pool.Get().QueryRow(context.Background(), `
		SELECT request_type, payload_snapshot_json
		FROM public.approval_requests
		WHERE source_record_id = $1 AND module = 'rentals' AND request_type = 'deposit_refund'
		ORDER BY created_at DESC LIMIT 1
	`, leaseID).Scan(&requestType, &payload)
	require.NoError(t, err)
	require.Equal(t, "deposit_refund", requestType)
	require.Contains(t, string(payload), "500.00")
}

func TestLeaseDepositRefundRejectsExcessAmount(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Excess Refund Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/deposit-refund", map[string]any{
		"amount": "999999.00",
		"reason": "excessive",
	})
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

// =============================================================================
// Route + audit coverage
// =============================================================================

func TestRentalsRoutesMatchPhase8Plan(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Route Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/lease-contracts", nil},
		{http.MethodGet, "/api/v1/lease-contracts/" + leaseID, nil},
		{http.MethodGet, "/api/v1/lease-contracts/" + leaseID + "/billing-schedule", nil},
		{http.MethodGet, "/api/v1/lease-contracts/" + leaseID + "/bills", nil},
		{http.MethodPost, "/api/v1/lease-bills/" + billID + "/issue", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := rentalRequest(t, tc.method, tc.path, tc.body)
			require.NotEqual(t, http.StatusNotFound, rr.Code, rr.Body.String())
		})
	}
}

func TestRentalAuditLogsCoverMaterialActions(t *testing.T) {
	unitID := createRentalTestUnit(t)
	tenantID := createRentalTestParty(t, "Audit Tenant")
	leaseID := createRentalTestLease(t, unitID, tenantID, "active")
	billID := createRentalTestBill(t, leaseID, "draft", "1000.00")

	require.Equal(t, http.StatusOK, rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/issue", map[string]any{}).Code)
	require.Equal(t, http.StatusOK, rentalRequest(t, http.MethodPost, "/api/v1/lease-bills/"+billID+"/void", map[string]any{"reason": "audit"}).Code)
	require.Equal(t, http.StatusAccepted, rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/terminate", map[string]any{"reason": "audit"}).Code)
	require.Equal(t, http.StatusAccepted, rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/deposit-refund", map[string]any{"amount": "100.00"}).Code)

	requireRentalAuditExists(t, "lease_contract", leaseID, "lease_activated")
	requireRentalAuditExists(t, "lease_bill", billID, "lease_bill_issued")
	requireRentalAuditExists(t, "lease_bill", billID, "lease_bill_voided")
	requireRentalAuditExists(t, "lease_contract", leaseID, "lease_termination_requested")
	requireRentalAuditExists(t, "lease_contract", leaseID, "lease_deposit_refund_requested")
}

func requireRentalAuditExists(t *testing.T, entityType string, entityID string, actionType string) {
	t.Helper()
	var count int
	err := pool.Get().QueryRow(context.Background(), `
		SELECT count(*) FROM public.audit_logs
		WHERE module = 'rentals'
		  AND entity_type = $1
		  AND entity_id = $2
		  AND action_type = $3
	`, entityType, entityID, actionType).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "no audit log for %s/%s/%s", entityType, entityID, actionType)
}

// =============================================================================
// Test helpers
// =============================================================================

func createRentalTestParty(t *testing.T, name string) string {
	t.Helper()
	code := fmt.Sprintf("RENPARTY_%d", time.Now().UnixNano())
	return CreateTestParty(t, testApp, testToken, code, name, "person", "active")
}

func createRentalTestUnit(t *testing.T) string {
	t.Helper()
	timestamp := time.Now().UnixNano()
	entityID := CreateTestBusinessEntity(t, testApp, testToken, fmt.Sprintf("RENENT_%d", timestamp), "Rentals Test Entity")
	branchID := CreateTestBranch(t, testApp, testToken, entityID, fmt.Sprintf("RENBRANCH_%d", timestamp), "Rentals Test Branch")
	projectID := CreateTestProject(t, testApp, testToken, entityID, branchID, fmt.Sprintf("RENPROJ_%d", timestamp), "Rentals Test Project")
	return CreateTestUnit(t, testApp, testToken, entityID, branchID, projectID, fmt.Sprintf("RENUNIT_%d", timestamp))
}

// createRentalTestLease creates a 12-month residential lease at $12,000/year ($1,000/month),
// monthly billing interval, $1,000 security deposit, ZERO advance rent.
// The zero advance is intentional — TestGenerateLeaseBillsMonthlyOneYearProduces12Bills
// expects exactly 12 bills with no extra advance bill prepended.
func createRentalTestLease(t *testing.T, unitID string, tenantID string, status string) string {
	t.Helper()

	unit := getSalesTestUnit(t, unitID)
	body := map[string]any{
		"business_entity_id":      unit["business_entity_id"],
		"branch_id":               unit["branch_id"],
		"project_id":              unit["project_id"],
		"unit_id":                 unitID,
		"primary_tenant_id":       tenantID,
		"lease_type":              "residential",
		"start_date":              "2026-05-01",
		"end_date":                "2027-04-30",
		"rent_pricing_basis":      "fixed_amount",
		"contractual_rent_amount": "12000.00",
		"billing_interval_value":  1,
		"billing_interval_unit":   "month",
		"billing_anchor_date":     "2026-05-01",
		"security_deposit_amount": "1000.00",
		"advance_rent_amount":     "0.00",
		"currency_code":           "USD",
	}

	rr := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts", body)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	leaseID := responseID(t, rr.Body)

	if status == "active" {
		actRR := rentalRequest(t, http.MethodPost, "/api/v1/lease-contracts/"+leaseID+"/activate", map[string]any{})
		require.Equal(t, http.StatusOK, actRR.Code, actRR.Body.String())
	}
	return leaseID
}

// createRentalTestBill inserts a draft lease_bill row directly via SQL — used for
// permission and route tests where the full generate flow isn't needed.
func createRentalTestBill(t *testing.T, leaseID string, status string, amount string) string {
	t.Helper()

	leaseUUID := uuid.MustParse(leaseID)
	billID := uuid.New()

	p := pool.Get()
	require.NotNil(t, p)
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		var beID, branchID, unitID, tenantID string
		var intervalValue int16
		var intervalUnit, currency string
		if err := tx.QueryRow(context.Background(), `
			SELECT business_entity_id::text, branch_id::text, unit_id::text, primary_tenant_id::text,
			       billing_interval_value, billing_interval_unit, currency_code
			FROM public.lease_contracts WHERE id = $1
		`, leaseUUID).Scan(&beID, &branchID, &unitID, &tenantID, &intervalValue, &intervalUnit, &currency); err != nil {
			return err
		}

		// Use a far-future window that will not collide with bills generated by
		// the regular billing flow (the helper lease covers 2026-05 → 2027-04).
		periodStart := fmt.Sprintf("2099-01-%02d", (time.Now().UnixNano()%28)+1)
		periodEnd := "2099-12-31"
		_, err := tx.Exec(context.Background(), `
			INSERT INTO public.lease_bills (
				id, business_entity_id, branch_id, lease_contract_id, unit_id, responsible_party_id,
				billing_period_start, billing_period_end, due_date,
				billing_interval_value, billing_interval_unit,
				billed_amount, currency_code, is_advance, status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $7, $9, $10, $11, $12, false, $13)
		`, billID, beID, branchID, leaseUUID, unitID, tenantID,
			periodStart, periodEnd, intervalValue, intervalUnit,
			amount, currency, status)
		return err
	})
	require.NoError(t, err)
	return billID.String()
}

// addRentalTestLeaseParty adds an extra lease_parties row (e.g. guarantor) so the
// termination test can verify CloseAllActiveLeaseParties closes guarantors too.
func addRentalTestLeaseParty(t *testing.T, leaseID string, partyID string, role string, isPrimary bool) string {
	t.Helper()

	leaseUUID := uuid.MustParse(leaseID)
	partyUUID := uuid.MustParse(partyID)
	rowID := uuid.New()

	p := pool.Get()
	require.NotNil(t, p)
	err := p.WithTx(context.Background(), map[string]any{"sub": testToken}, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO public.lease_parties (id, lease_contract_id, party_id, role, is_primary, effective_from, status)
			VALUES ($1, $2, $3, $4, $5, '2026-05-01', 'active')
		`, rowID, leaseUUID, partyUUID, role, isPrimary)
		return err
	})
	require.NoError(t, err)
	return rowID.String()
}

func rentalRequest(t *testing.T, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return rentalRequestWithToken(t, testToken, method, path, body)
}

func rentalRequestWithToken(t *testing.T, token string, method string, path string, body any) *httptest.ResponseRecorder {
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

func decodeRentalTestObject(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var response map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body.Bytes()))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&response))
	return response
}

func normalizeRentalTestJSONAmount(t *testing.T, value any) string {
	t.Helper()
	switch amount := value.(type) {
	case string:
		// Pad strings like "1000" or "1000.0" to two decimal places.
		rat, ok := new(big.Rat).SetString(amount)
		require.True(t, ok, "invalid amount %q", amount)
		return formatRentalCents(rat)
	case json.Number:
		rat, ok := new(big.Rat).SetString(amount.String())
		require.True(t, ok)
		return formatRentalCents(rat)
	case float64:
		return fmt.Sprintf("%.2f", amount)
	default:
		t.Fatalf("unexpected amount value %T: %v", value, value)
		return ""
	}
}

func formatRentalCents(rat *big.Rat) string {
	cents := new(big.Rat).Mul(rat, big.NewRat(100, 1))
	num := new(big.Int).Set(cents.Num())
	den := cents.Denom()
	q := new(big.Int).Quo(num, den)
	value := q.Int64()
	whole := value / 100
	frac := value % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}
