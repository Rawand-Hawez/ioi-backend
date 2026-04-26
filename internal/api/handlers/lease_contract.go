package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

// =============================================================================
// Lease contract list / get / create / update (CRUD)
// =============================================================================

func ListLeaseContracts(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	listParams := db.ListLeaseContractsParams{
		Limit:  int32(perPage),
		Offset: int32((page - 1) * perPage),
	}
	if v := c.Query("business_entity_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			listParams.BusinessEntityID = toPgUUID(id)
		}
	}
	if v := c.Query("branch_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			listParams.BranchID = toPgUUID(id)
		}
	}
	if v := c.Query("project_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			listParams.ProjectID = toPgUUID(id)
		}
	}
	if v := c.Query("unit_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			listParams.UnitID = toPgUUID(id)
		}
	}
	if v := c.Query("primary_tenant_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			listParams.PrimaryTenantID = toPgUUID(id)
		}
	}
	if status := c.Query("status"); status != "" {
		listParams.Status = toPgText(&status)
	}

	var leases []db.LeaseContract
	var totalCount int64

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		leases, err = q.ListLeaseContracts(ctx, listParams)
		if err != nil {
			return err
		}
		totalCount, err = q.CountLeaseContracts(ctx, db.CountLeaseContractsParams{
			BusinessEntityID: listParams.BusinessEntityID,
			BranchID:         listParams.BranchID,
			ProjectID:        listParams.ProjectID,
			UnitID:           listParams.UnitID,
			PrimaryTenantID:  listParams.PrimaryTenantID,
			Status:           listParams.Status,
		})
		return err
	})
	if err != nil {
		log.Printf("ListLeaseContracts error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to list lease contracts"})
	}

	totalPages := int64(0)
	if perPage > 0 {
		totalPages = (totalCount + int64(perPage) - 1) / int64(perPage)
	}

	return c.JSON(fiber.Map{
		"data": leases,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total_count": totalCount,
			"total_pages": totalPages,
		},
	})
}

func GetLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var lease db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		var err error
		lease, err = db.New(tx).GetLeaseContract(ctx, toPgUUID(id))
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		log.Printf("GetLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to load lease contract"})
	}
	return c.JSON(lease)
}

type leaseContractCreateRequest struct {
	BusinessEntityID      string  `json:"business_entity_id"`
	BranchID              string  `json:"branch_id"`
	ProjectID             string  `json:"project_id"`
	UnitID                string  `json:"unit_id"`
	PrimaryTenantID       string  `json:"primary_tenant_id"`
	LeaseType             string  `json:"lease_type"`
	StartDate             string  `json:"start_date"`
	EndDate               string  `json:"end_date"`
	RentPricingBasis      string  `json:"rent_pricing_basis"`
	AreaBasisSqm          *string `json:"area_basis_sqm,omitempty"`
	RatePerSqm            *string `json:"rate_per_sqm,omitempty"`
	ContractualRentAmount string  `json:"contractual_rent_amount"`
	BillingIntervalValue  *int16  `json:"billing_interval_value,omitempty"`
	BillingIntervalUnit   *string `json:"billing_interval_unit,omitempty"`
	BillingAnchorDate     *string `json:"billing_anchor_date,omitempty"`
	SecurityDepositAmount *string `json:"security_deposit_amount,omitempty"`
	AdvanceRentAmount     *string `json:"advance_rent_amount,omitempty"`
	CurrencyCode          *string `json:"currency_code,omitempty"`
	NoticePeriodDays      *int32  `json:"notice_period_days,omitempty"`
	PurposeOfUse          *string `json:"purpose_of_use,omitempty"`
	Notes                 *string `json:"notes,omitempty"`
}

var validLeaseTypes = map[string]bool{"residential": true, "commercial": true}
var validRentBases = map[string]bool{"fixed_amount": true, "area_based": true}
var validBillingUnits = map[string]bool{"month": true, "quarter": true, "semi_year": true, "year": true}

func validateLeaseAmounts(req *leaseContractCreateRequest) error {
	rent, err := parseRequiredAmount("contractual_rent_amount", &req.ContractualRentAmount)
	if err != nil {
		return err
	}
	if rent.Sign() < 0 {
		return errors.New("contractual_rent_amount must be non-negative")
	}
	zero := big.NewRat(0, 1)
	if req.SecurityDepositAmount != nil && *req.SecurityDepositAmount != "" {
		v, err := parseRequiredAmount("security_deposit_amount", req.SecurityDepositAmount)
		if err != nil {
			return err
		}
		if v.Cmp(zero) < 0 {
			return errors.New("security_deposit_amount must be non-negative")
		}
	}
	if req.AdvanceRentAmount != nil && *req.AdvanceRentAmount != "" {
		v, err := parseRequiredAmount("advance_rent_amount", req.AdvanceRentAmount)
		if err != nil {
			return err
		}
		if v.Cmp(zero) < 0 {
			return errors.New("advance_rent_amount must be non-negative")
		}
	}
	if req.AreaBasisSqm != nil && *req.AreaBasisSqm != "" {
		v, err := parseRequiredAmount("area_basis_sqm", req.AreaBasisSqm)
		if err != nil {
			return err
		}
		if v.Cmp(zero) < 0 {
			return errors.New("area_basis_sqm must be non-negative")
		}
	}
	if req.RatePerSqm != nil && *req.RatePerSqm != "" {
		v, err := parseRequiredAmount("rate_per_sqm", req.RatePerSqm)
		if err != nil {
			return err
		}
		if v.Cmp(zero) < 0 {
			return errors.New("rate_per_sqm must be non-negative")
		}
	}
	return nil
}

func CreateLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	var req leaseContractCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	beID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}
	branchID, err := uuid.Parse(req.BranchID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid branch_id"})
	}
	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
	}
	unitID, err := uuid.Parse(req.UnitID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid unit_id"})
	}
	tenantID, err := uuid.Parse(req.PrimaryTenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid primary_tenant_id"})
	}

	if !validLeaseTypes[req.LeaseType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease_type, must be residential or commercial"})
	}
	if !validRentBases[req.RentPricingBasis] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid rent_pricing_basis, must be fixed_amount or area_based"})
	}
	if req.RentPricingBasis == "area_based" {
		if req.AreaBasisSqm == nil || *req.AreaBasisSqm == "" || req.RatePerSqm == nil || *req.RatePerSqm == "" {
			return c.Status(400).JSON(fiber.Map{"error": "area_based pricing requires area_basis_sqm and rate_per_sqm"})
		}
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid start_date"})
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid end_date"})
	}
	if endDate.Before(startDate) {
		return c.Status(400).JSON(fiber.Map{"error": "end_date must be on or after start_date"})
	}

	intervalValue := int16(1)
	if req.BillingIntervalValue != nil {
		intervalValue = *req.BillingIntervalValue
	}
	if intervalValue <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "billing_interval_value must be > 0"})
	}
	intervalUnit := "month"
	if req.BillingIntervalUnit != nil && *req.BillingIntervalUnit != "" {
		intervalUnit = *req.BillingIntervalUnit
	}
	if !validBillingUnits[intervalUnit] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid billing_interval_unit"})
	}
	anchorRaw := req.StartDate
	if req.BillingAnchorDate != nil && *req.BillingAnchorDate != "" {
		anchorRaw = *req.BillingAnchorDate
	}
	if _, err := time.Parse("2006-01-02", anchorRaw); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid billing_anchor_date"})
	}

	if err := validateLeaseAmounts(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	currency := "USD"
	if req.CurrencyCode != nil && *req.CurrencyCode != "" {
		currency = *req.CurrencyCode
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	deposit := "0"
	if req.SecurityDepositAmount != nil && *req.SecurityDepositAmount != "" {
		deposit = *req.SecurityDepositAmount
	}
	advance := "0"
	if req.AdvanceRentAmount != nil && *req.AdvanceRentAmount != "" {
		advance = *req.AdvanceRentAmount
	}

	params := db.CreateLeaseContractParams{
		BusinessEntityID:      toPgUUID(beID),
		BranchID:              toPgUUID(branchID),
		ProjectID:             toPgUUID(projectID),
		UnitID:                toPgUUID(unitID),
		PrimaryTenantID:       toPgUUID(tenantID),
		LeaseType:             req.LeaseType,
		StartDate:             pgtype.Date{Time: startDate, Valid: true},
		EndDate:               pgtype.Date{Time: endDate, Valid: true},
		RentPricingBasis:      req.RentPricingBasis,
		AreaBasisSqm:          toPgNumeric(req.AreaBasisSqm),
		RatePerSqm:            toPgNumeric(req.RatePerSqm),
		ContractualRentAmount: toPgNumeric(&req.ContractualRentAmount),
		BillingIntervalValue:  intervalValue,
		BillingIntervalUnit:   intervalUnit,
		BillingAnchorDate:     toPgDate(&anchorRaw),
		SecurityDepositAmount: toPgNumeric(&deposit),
		AdvanceRentAmount:     toPgNumeric(&advance),
		CurrencyCode:          currency,
		NoticePeriodDays:      pgtype.Int4{Int32: deref32(req.NoticePeriodDays), Valid: req.NoticePeriodDays != nil},
		PurposeOfUse:          toPgText(req.PurposeOfUse),
		Notes:                 toPgText(req.Notes),
		CreatedByUserID:       toPgUUID(userID),
	}

	var lease db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.create", params.BusinessEntityID, params.BranchID, params.ProjectID); err != nil {
			return err
		}
		var err error
		lease, err = q.CreateLeaseContract(ctx, params)
		if err != nil {
			return err
		}
		afterSnapshot, _ := json.Marshal(lease)
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "rentals",
			ActionType:        "lease_contract_created",
			EntityType:        "lease_contract",
			EntityID:          lease.ID,
			ScopeType:         "project",
			ScopeID:           lease.ProjectID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Created lease contract %s", lease.LeaseNo),
			AfterSnapshotJson: afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		log.Printf("CreateLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to create lease contract"})
	}

	return c.Status(201).JSON(lease)
}

func deref32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

type leaseContractUpdateRequest struct {
	LeaseType             *string `json:"lease_type,omitempty"`
	StartDate             *string `json:"start_date,omitempty"`
	EndDate               *string `json:"end_date,omitempty"`
	RentPricingBasis      *string `json:"rent_pricing_basis,omitempty"`
	AreaBasisSqm          *string `json:"area_basis_sqm,omitempty"`
	RatePerSqm            *string `json:"rate_per_sqm,omitempty"`
	ContractualRentAmount *string `json:"contractual_rent_amount,omitempty"`
	BillingIntervalValue  *int16  `json:"billing_interval_value,omitempty"`
	BillingIntervalUnit   *string `json:"billing_interval_unit,omitempty"`
	BillingAnchorDate     *string `json:"billing_anchor_date,omitempty"`
	SecurityDepositAmount *string `json:"security_deposit_amount,omitempty"`
	AdvanceRentAmount     *string `json:"advance_rent_amount,omitempty"`
	NoticePeriodDays      *int32  `json:"notice_period_days,omitempty"`
	PurposeOfUse          *string `json:"purpose_of_use,omitempty"`
	Notes                 *string `json:"notes,omitempty"`
}

func UpdateLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	var req leaseContractUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.LeaseType != nil && !validLeaseTypes[*req.LeaseType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease_type"})
	}
	if req.RentPricingBasis != nil && !validRentBases[*req.RentPricingBasis] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid rent_pricing_basis"})
	}
	if req.BillingIntervalUnit != nil && !validBillingUnits[*req.BillingIntervalUnit] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid billing_interval_unit"})
	}
	if req.BillingIntervalValue != nil && *req.BillingIntervalValue <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "billing_interval_value must be > 0"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	params := db.UpdateLeaseContractParams{
		ID:                    toPgUUID(id),
		LeaseType:             toPgText(req.LeaseType),
		StartDate:             toPgDate(req.StartDate),
		EndDate:               toPgDate(req.EndDate),
		RentPricingBasis:      toPgText(req.RentPricingBasis),
		AreaBasisSqm:          toPgNumeric(req.AreaBasisSqm),
		RatePerSqm:            toPgNumeric(req.RatePerSqm),
		ContractualRentAmount: toPgNumeric(req.ContractualRentAmount),
		BillingIntervalValue:  pgtype.Int2{Int16: derefInt16(req.BillingIntervalValue), Valid: req.BillingIntervalValue != nil},
		BillingIntervalUnit:   toPgText(req.BillingIntervalUnit),
		BillingAnchorDate:     toPgDate(req.BillingAnchorDate),
		SecurityDepositAmount: toPgNumeric(req.SecurityDepositAmount),
		AdvanceRentAmount:     toPgNumeric(req.AdvanceRentAmount),
		NoticePeriodDays:      pgtype.Int4{Int32: deref32(req.NoticePeriodDays), Valid: req.NoticePeriodDays != nil},
		PurposeOfUse:          toPgText(req.PurposeOfUse),
		Notes:                 toPgText(req.Notes),
	}

	var lease db.LeaseContract
	var existing db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		existing, err = q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.edit", existing.BusinessEntityID, existing.BranchID, existing.ProjectID); err != nil {
			return err
		}
		if existing.Status != "draft" {
			return ErrLeaseNotDraft
		}
		lease, err = q.UpdateLeaseContract(ctx, params)
		if err != nil {
			return err
		}
		beforeSnapshot, _ := json.Marshal(existing)
		afterSnapshot, _ := json.Marshal(lease)
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:          pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:             toPgUUID(userID),
			Module:             "rentals",
			ActionType:         "lease_contract_updated",
			EntityType:         "lease_contract",
			EntityID:           lease.ID,
			ScopeType:          "project",
			ScopeID:            lease.ProjectID,
			ResultStatus:       "success",
			SummaryText:        "Updated draft lease contract",
			BeforeSnapshotJson: beforeSnapshot,
			AfterSnapshotJson:  afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("UpdateLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to update lease contract"})
	}
	return c.JSON(lease)
}

func derefInt16(v *int16) int16 {
	if v == nil {
		return 0
	}
	return *v
}

// =============================================================================
// Lease activation
// =============================================================================

func ActivateLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var lease db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.activate", existing.BusinessEntityID, existing.BranchID, existing.ProjectID); err != nil {
			return err
		}
		if existing.Status != "draft" {
			return ErrLeaseActivateNotDraft
		}

		// Reject if another active lease already exists for the same unit.
		if _, err := q.GetActiveLeaseContractForUnit(ctx, existing.UnitID); err == nil {
			return ErrLeaseUnitHasActive
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		updated, err := q.UpdateLeaseContractStatus(ctx, db.UpdateLeaseContractStatusParams{
			ID:       toPgUUID(id),
			Status:   "active",
			Status_2: "draft",
		})
		if err != nil {
			return err
		}

		if _, err := q.CreateLeaseParty(ctx, db.CreateLeasePartyParams{
			LeaseContractID: updated.ID,
			PartyID:         updated.PrimaryTenantID,
			Role:            "primary_tenant",
			IsPrimary:       true,
			EffectiveFrom:   updated.StartDate,
			EffectiveTo:     pgtype.Date{Valid: false},
			Status:          "active",
		}); err != nil {
			return err
		}

		if err := q.UpdateUnitOccupancyStatus(ctx, db.UpdateUnitOccupancyStatusParams{
			ID:              updated.UnitID,
			OccupancyStatus: "occupied",
		}); err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(updated)
		if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "rentals",
			ActionType:        "lease_activated",
			EntityType:        "lease_contract",
			EntityID:          updated.ID,
			ScopeType:         "project",
			ScopeID:           updated.ProjectID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Activated lease contract %s", updated.LeaseNo),
			AfterSnapshotJson: afterSnapshot,
		}); err != nil {
			return err
		}

		lease = updated
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("ActivateLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to activate lease contract"})
	}
	return c.JSON(lease)
}

// =============================================================================
// Bill generation (schedule preview + persist as drafts)
// =============================================================================

type leaseBillDraft struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	DueDate     time.Time
	Amount      string
	IsAdvance   bool
}

func leaseIntervalMonths(value int16, unit string) (int, error) {
	if value <= 0 {
		return 0, errors.New("billing_interval_value must be > 0")
	}
	switch unit {
	case "month":
		return int(value), nil
	case "quarter":
		return int(value) * 3, nil
	case "semi_year":
		return int(value) * 6, nil
	case "year":
		return int(value) * 12, nil
	default:
		return 0, fmt.Errorf("invalid billing_interval_unit %q", unit)
	}
}

func buildLeaseBillDrafts(lease db.LeaseContract) ([]leaseBillDraft, error) {
	if !lease.StartDate.Valid || !lease.EndDate.Valid {
		return nil, errors.New("lease contract has invalid start/end date")
	}
	intervalMonths, err := leaseIntervalMonths(lease.BillingIntervalValue, lease.BillingIntervalUnit)
	if err != nil {
		return nil, err
	}

	rent, err := numericToBigRat(lease.ContractualRentAmount)
	if err != nil {
		return nil, err
	}
	advance, err := numericToBigRat(lease.AdvanceRentAmount)
	if err != nil {
		return nil, err
	}

	startDate := lease.StartDate.Time
	endDate := lease.EndDate.Time
	anchorDate := startDate
	if lease.BillingAnchorDate.Valid {
		anchorDate = lease.BillingAnchorDate.Time
	}

	totalMonths := monthsBetween(startDate, endDate) + 1
	if totalMonths <= 0 {
		return nil, errors.New("lease end_date must be on or after start_date")
	}
	periods := totalMonths / intervalMonths
	if totalMonths%intervalMonths > 0 {
		periods++
	}
	if periods < 1 {
		periods = 1
	}

	periodRent := new(big.Rat).Mul(rent, big.NewRat(int64(intervalMonths), 12))

	drafts := make([]leaseBillDraft, 0, periods+1)

	if advance.Sign() > 0 {
		periodEnd := addMonthsClamped(startDate, intervalMonths).AddDate(0, 0, -1)
		if periodEnd.After(endDate) {
			periodEnd = endDate
		}
		dueDate := startDate
		drafts = append(drafts, leaseBillDraft{
			PeriodStart: startDate,
			PeriodEnd:   periodEnd,
			DueDate:     dueDate,
			Amount:      formatMoney2(advance),
			IsAdvance:   true,
		})
	}

	cumulative := big.NewRat(0, 1)
	for i := 0; i < periods; i++ {
		periodStart := addMonthsClamped(startDate, i*intervalMonths)
		periodEnd := addMonthsClamped(startDate, (i+1)*intervalMonths).AddDate(0, 0, -1)
		if periodEnd.After(endDate) {
			periodEnd = endDate
		}

		var amount *big.Rat
		if i == periods-1 {
			amount = new(big.Rat).Sub(rent, cumulative)
			if amount.Sign() < 0 {
				amount = big.NewRat(0, 1)
			}
		} else {
			amount = new(big.Rat).Set(periodRent)
			cumulative = new(big.Rat).Add(cumulative, periodRent)
		}

		dueDate := addMonthsClamped(anchorDate, i*intervalMonths)
		if dueDate.Before(periodStart) {
			dueDate = periodStart
		}

		drafts = append(drafts, leaseBillDraft{
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			DueDate:     dueDate,
			Amount:      formatMoney2(amount),
			IsAdvance:   false,
		})
	}

	return drafts, nil
}

// monthsBetween moved to time_helpers.go

func GetLeaseBillingSchedule(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var lease db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		var err error
		lease, err = db.New(tx).GetLeaseContract(ctx, toPgUUID(id))
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		log.Printf("GetLeaseBillingSchedule error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to load lease contract"})
	}

	drafts, err := buildLeaseBillDrafts(lease)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]fiber.Map, 0, len(drafts))
	for _, d := range drafts {
		out = append(out, fiber.Map{
			"billing_period_start": d.PeriodStart.Format("2006-01-02"),
			"billing_period_end":   d.PeriodEnd.Format("2006-01-02"),
			"due_date":             d.DueDate.Format("2006-01-02"),
			"billed_amount":        d.Amount,
			"is_advance":           d.IsAdvance,
		})
	}
	return c.JSON(fiber.Map{"data": out})
}

func GenerateLeaseBills(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var bills []db.LeaseBill
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		lease, err := q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.bill.generate", lease.BusinessEntityID, lease.BranchID, lease.ProjectID); err != nil {
			return err
		}
		if lease.Status != "active" {
			return ErrLeaseGenerateNotActive
		}

		drafts, err := buildLeaseBillDrafts(lease)
		if err != nil {
			return err
		}

		notesAdvance := "Advance rent"
		for _, d := range drafts {
			// Skip if a bill for this period already exists (idempotent generation).
			existing, lookupErr := q.GetLeaseBillForPeriod(ctx, db.GetLeaseBillForPeriodParams{
				LeaseContractID:    lease.ID,
				BillingPeriodStart: pgtype.Date{Time: d.PeriodStart, Valid: true},
				BillingPeriodEnd:   pgtype.Date{Time: d.PeriodEnd, Valid: true},
				IsAdvance:          d.IsAdvance,
			})
			if lookupErr == nil {
				bills = append(bills, existing)
				continue
			}
			if !errors.Is(lookupErr, pgx.ErrNoRows) {
				return lookupErr
			}

			amount := d.Amount
			params := db.CreateLeaseBillParams{
				BusinessEntityID:     lease.BusinessEntityID,
				BranchID:             lease.BranchID,
				LeaseContractID:      lease.ID,
				UnitID:               lease.UnitID,
				ResponsiblePartyID:   lease.PrimaryTenantID,
				BillingPeriodStart:   pgtype.Date{Time: d.PeriodStart, Valid: true},
				BillingPeriodEnd:     pgtype.Date{Time: d.PeriodEnd, Valid: true},
				DueDate:              pgtype.Date{Time: d.DueDate, Valid: true},
				BillingIntervalValue: lease.BillingIntervalValue,
				BillingIntervalUnit:  lease.BillingIntervalUnit,
				BilledAmount:         toPgNumeric(&amount),
				CurrencyCode:         lease.CurrencyCode,
				IsAdvance:            d.IsAdvance,
				Status:               "draft",
			}
			if d.IsAdvance {
				params.Notes = pgtype.Text{String: notesAdvance, Valid: true}
			}
			bill, err := q.CreateLeaseBill(ctx, params)
			if err != nil {
				return err
			}
			bills = append(bills, bill)
		}

		summary := fmt.Sprintf("Generated %d lease bills", len(bills))
		afterSnapshot, _ := json.Marshal(map[string]any{"bills_count": len(bills)})
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "rentals",
			ActionType:        "lease_bills_generated",
			EntityType:        "lease_contract",
			EntityID:          lease.ID,
			ScopeType:         "project",
			ScopeID:           lease.ProjectID,
			ResultStatus:      "success",
			SummaryText:       summary,
			AfterSnapshotJson: afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("GenerateLeaseBills error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate lease bills"})
	}
	return c.Status(201).JSON(fiber.Map{"data": bills})
}

func ListLeaseBillsForContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	listParams := db.ListLeaseBillsParams{
		LeaseContractID: toPgUUID(id),
		Limit:           int32(perPage),
		Offset:          int32((page - 1) * perPage),
	}
	if status := c.Query("status"); status != "" {
		listParams.Status = toPgText(&status)
	}

	var bills []db.LeaseBill
	var totalCount int64
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		bills, err = q.ListLeaseBills(ctx, listParams)
		if err != nil {
			return err
		}
		totalCount, err = q.CountLeaseBills(ctx, db.CountLeaseBillsParams{
			LeaseContractID:    listParams.LeaseContractID,
			UnitID:             listParams.UnitID,
			ResponsiblePartyID: listParams.ResponsiblePartyID,
			Status:             listParams.Status,
		})
		return err
	})
	if err != nil {
		log.Printf("ListLeaseBillsForContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to list lease bills"})
	}

	totalPages := int64(0)
	if perPage > 0 {
		totalPages = (totalCount + int64(perPage) - 1) / int64(perPage)
	}
	return c.JSON(fiber.Map{
		"data": bills,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total_count": totalCount,
			"total_pages": totalPages,
		},
	})
}

// =============================================================================
// Approval-gated termination (Phase 7 pattern: lease stays active during pending)
// =============================================================================

type leaseTerminationRequest struct {
	Reason          *string `json:"reason,omitempty"`
	TerminationDate *string `json:"termination_date,omitempty"`
}

type leaseApprovalOutcome struct {
	Action            string
	ApprovalRequestID pgtype.UUID
	Lease             db.LeaseContract
}

func handleLeaseTerminationGate(
	ctx context.Context,
	q *db.Queries,
	lease db.LeaseContract,
	userID uuid.UUID,
	reason *string,
	terminationDate *string,
) (leaseApprovalOutcome, error) {
	out := leaseApprovalOutcome{Lease: lease}

	latest, lerr := q.GetLatestRentalsApprovalRequest(ctx, db.GetLatestRentalsApprovalRequestParams{
		SourceRecordType: "lease_contract",
		SourceRecordID:   lease.ID,
		RequestType:      "lease_termination",
	})
	if lerr != nil && !errors.Is(lerr, pgx.ErrNoRows) {
		return out, lerr
	}

	if lerr == nil {
		switch latest.Status {
		case "approved":
			updated, err := q.UpdateLeaseContractStatus(ctx, db.UpdateLeaseContractStatusParams{
				ID:       lease.ID,
				Status:   "terminated",
				Status_2: "active",
			})
			if err != nil {
				return out, err
			}

			closedAt := updated.EndDate
			if terminationDate != nil && *terminationDate != "" {
				if t, err := time.Parse("2006-01-02", *terminationDate); err == nil {
					closedAt = pgtype.Date{Time: t, Valid: true}
				}
			}
			if !closedAt.Valid {
				closedAt = pgtype.Date{Time: time.Now(), Valid: true}
			}
			if _, err := q.CloseAllActiveLeaseParties(ctx, db.CloseAllActiveLeasePartiesParams{
				LeaseContractID: updated.ID,
				EffectiveTo:     closedAt,
			}); err != nil {
				return out, err
			}

			if err := q.UpdateUnitOccupancyStatus(ctx, db.UpdateUnitOccupancyStatusParams{
				ID:              updated.UnitID,
				OccupancyStatus: "vacant",
			}); err != nil {
				return out, err
			}

			beforeSnapshot, _ := json.Marshal(lease)
			afterSnapshot, _ := json.Marshal(updated)
			if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
				EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UserID:                   toPgUUID(userID),
				Module:                   "rentals",
				ActionType:               "lease_terminated",
				EntityType:               "lease_contract",
				EntityID:                 lease.ID,
				ScopeType:                "project",
				ScopeID:                  lease.ProjectID,
				ResultStatus:             "success",
				SummaryText:              fmt.Sprintf("Applied approved lease termination for %s", lease.LeaseNo),
				BeforeSnapshotJson:       beforeSnapshot,
				AfterSnapshotJson:        afterSnapshot,
				RelatedApprovalRequestID: latest.ID,
			}); err != nil {
				return out, err
			}
			out.Action = "applied"
			out.ApprovalRequestID = latest.ID
			out.Lease = updated
			return out, nil
		case "pending":
			out.Action = "pending_existing"
			out.ApprovalRequestID = latest.ID
			return out, nil
		case "rejected":
			out.Action = "rejected"
			out.ApprovalRequestID = latest.ID
			return out, nil
		}
	}

	payload := map[string]any{}
	if reason != nil {
		payload["reason"] = *reason
	}
	if terminationDate != nil {
		payload["termination_date"] = *terminationDate
	}
	payload["lease_id"] = uuid.UUID(lease.ID.Bytes).String()
	payloadBytes, _ := json.Marshal(payload)

	created, err := q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
		BusinessEntityID:    lease.BusinessEntityID,
		BranchID:            lease.BranchID,
		Module:              "rentals",
		RequestType:         "lease_termination",
		SourceRecordType:    "lease_contract",
		SourceRecordID:      lease.ID,
		RequestedByUserID:   toPgUUID(userID),
		Status:              "pending",
		PayloadSnapshotJson: payloadBytes,
	})
	if err != nil {
		return out, err
	}

	afterSnapshot, _ := json.Marshal(lease)
	if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UserID:                   toPgUUID(userID),
		Module:                   "rentals",
		ActionType:               "lease_termination_requested",
		EntityType:               "lease_contract",
		EntityID:                 lease.ID,
		ScopeType:                "project",
		ScopeID:                  lease.ProjectID,
		ResultStatus:             "pending",
		SummaryText:              fmt.Sprintf("Requested termination approval for %s", lease.LeaseNo),
		AfterSnapshotJson:        afterSnapshot,
		RelatedApprovalRequestID: created.ID,
	}); err != nil {
		return out, err
	}

	out.Action = "pending_new"
	out.ApprovalRequestID = created.ID
	return out, nil
}

func TerminateLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	var req leaseTerminationRequest
	_ = c.BodyParser(&req)

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var outcome leaseApprovalOutcome
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.terminate", existing.BusinessEntityID, existing.BranchID, existing.ProjectID); err != nil {
			return err
		}
		if existing.Status != "active" {
			return ErrLeaseTerminateNotActive
		}
		o, err := handleLeaseTerminationGate(ctx, q, existing, userID, req.Reason, req.TerminationDate)
		if err != nil {
			return err
		}
		outcome = o
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("TerminateLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to terminate lease contract"})
	}

	switch outcome.Action {
	case "applied":
		return c.JSON(fiber.Map{
			"message":             "lease contract terminated",
			"lease":               outcome.Lease,
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	case "pending_new", "pending_existing":
		return c.Status(202).JSON(fiber.Map{
			"action":              "approval_requested",
			"message":             "termination request submitted for approval",
			"lease_id":            id,
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	case "rejected":
		return c.Status(409).JSON(fiber.Map{
			"error":               "latest termination request was rejected",
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	default:
		log.Printf("unexpected lease termination outcome: %q", outcome.Action)
		return c.Status(500).JSON(fiber.Map{"error": "failed to terminate lease contract"})
	}
}

// =============================================================================
// Renewal — creates a draft successor lease, marks the old one renewed
// =============================================================================

type leaseRenewRequest struct {
	StartDate             string  `json:"start_date"`
	EndDate               string  `json:"end_date"`
	ContractualRentAmount *string `json:"contractual_rent_amount,omitempty"`
	BillingAnchorDate     *string `json:"billing_anchor_date,omitempty"`
	SecurityDepositAmount *string `json:"security_deposit_amount,omitempty"`
	AdvanceRentAmount     *string `json:"advance_rent_amount,omitempty"`
	Notes                 *string `json:"notes,omitempty"`
}

func RenewLeaseContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	var req leaseRenewRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid start_date"})
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid end_date"})
	}
	if endDate.Before(startDate) {
		return c.Status(400).JSON(fiber.Map{"error": "end_date must be on or after start_date"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var newLease db.LeaseContract
	var oldLease db.LeaseContract
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.renew", existing.BusinessEntityID, existing.BranchID, existing.ProjectID); err != nil {
			return err
		}
		if existing.Status != "active" {
			return ErrLeaseRenewNotActive
		}

		anchor := req.StartDate
		if req.BillingAnchorDate != nil && *req.BillingAnchorDate != "" {
			anchor = *req.BillingAnchorDate
		}

		rent := existing.ContractualRentAmount
		if req.ContractualRentAmount != nil && *req.ContractualRentAmount != "" {
			rent = toPgNumeric(req.ContractualRentAmount)
		}
		deposit := existing.SecurityDepositAmount
		if req.SecurityDepositAmount != nil && *req.SecurityDepositAmount != "" {
			deposit = toPgNumeric(req.SecurityDepositAmount)
		}
		advance := existing.AdvanceRentAmount
		if req.AdvanceRentAmount != nil && *req.AdvanceRentAmount != "" {
			advance = toPgNumeric(req.AdvanceRentAmount)
		}

		// Mark old lease renewed BEFORE creating the new one so the unique active-lease
		// constraint doesn't reject the new draft (active stays unique on activation).
		updatedOld, err := q.UpdateLeaseContractStatus(ctx, db.UpdateLeaseContractStatusParams{
			ID:       existing.ID,
			Status:   "renewed",
			Status_2: "active",
		})
		if err != nil {
			return err
		}
		oldLease = updatedOld

		newParams := db.CreateLeaseContractParams{
			BusinessEntityID:           existing.BusinessEntityID,
			BranchID:                   existing.BranchID,
			ProjectID:                  existing.ProjectID,
			UnitID:                     existing.UnitID,
			PrimaryTenantID:            existing.PrimaryTenantID,
			RenewedFromLeaseContractID: existing.ID,
			LeaseType:                  existing.LeaseType,
			StartDate:                  pgtype.Date{Time: startDate, Valid: true},
			EndDate:                    pgtype.Date{Time: endDate, Valid: true},
			RentPricingBasis:           existing.RentPricingBasis,
			AreaBasisSqm:               existing.AreaBasisSqm,
			RatePerSqm:                 existing.RatePerSqm,
			ContractualRentAmount:      rent,
			BillingIntervalValue:       existing.BillingIntervalValue,
			BillingIntervalUnit:        existing.BillingIntervalUnit,
			BillingAnchorDate:          toPgDate(&anchor),
			SecurityDepositAmount:      deposit,
			AdvanceRentAmount:          advance,
			CurrencyCode:               existing.CurrencyCode,
			NoticePeriodDays:           existing.NoticePeriodDays,
			PurposeOfUse:               existing.PurposeOfUse,
			Notes:                      toPgText(req.Notes),
			CreatedByUserID:            toPgUUID(userID),
		}
		newLease, err = q.CreateLeaseContract(ctx, newParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(newLease)
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "rentals",
			ActionType:        "lease_renewed",
			EntityType:        "lease_contract",
			EntityID:          existing.ID,
			ScopeType:         "project",
			ScopeID:           existing.ProjectID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Renewed lease %s into draft %s", existing.LeaseNo, newLease.LeaseNo),
			AfterSnapshotJson: afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("RenewLeaseContract error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to renew lease contract"})
	}

	return c.Status(201).JSON(fiber.Map{
		"lease":        newLease,
		"renewed_from": oldLease,
	})
}

// =============================================================================
// Deposit refund — approval request only (no outbound payment in Phase 8)
// =============================================================================

type leaseDepositRefundRequest struct {
	Amount string  `json:"amount"`
	Reason *string `json:"reason,omitempty"`
}

func RequestLeaseDepositRefund(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease contract id"})
	}

	var req leaseDepositRefundRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	amount, err := parseRequiredAmount("amount", &req.Amount)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if amount.Sign() <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "amount must be positive"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var approvalRequestID pgtype.UUID
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		lease, err := q.GetLeaseContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.lease.deposit_refund", lease.BusinessEntityID, lease.BranchID, lease.ProjectID); err != nil {
			return err
		}
		switch lease.Status {
		case "active", "expired", "terminated":
			// allowed
		default:
			return errors.New("deposit refund only allowed for active, expired or terminated leases")
		}

		deposit, err := numericToBigRat(lease.SecurityDepositAmount)
		if err != nil {
			return err
		}
		if amount.Cmp(deposit) > 0 {
			return errors.New("amount exceeds security_deposit_amount")
		}

		payload := map[string]any{
			"lease_id":      uuid.UUID(lease.ID.Bytes).String(),
			"tenant_id":     uuid.UUID(lease.PrimaryTenantID.Bytes).String(),
			"amount":        req.Amount,
			"currency_code": lease.CurrencyCode,
			"start_date":    lease.StartDate.Time.Format("2006-01-02"),
			"end_date":      lease.EndDate.Time.Format("2006-01-02"),
		}
		if req.Reason != nil {
			payload["reason"] = *req.Reason
		}
		payloadBytes, _ := json.Marshal(payload)

		created, err := q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
			BusinessEntityID:    lease.BusinessEntityID,
			BranchID:            lease.BranchID,
			Module:              "rentals",
			RequestType:         "deposit_refund",
			SourceRecordType:    "lease_contract",
			SourceRecordID:      lease.ID,
			RequestedByUserID:   toPgUUID(userID),
			Status:              "pending",
			PayloadSnapshotJson: payloadBytes,
		})
		if err != nil {
			return err
		}
		approvalRequestID = created.ID

		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:                   toPgUUID(userID),
			Module:                   "rentals",
			ActionType:               "lease_deposit_refund_requested",
			EntityType:               "lease_contract",
			EntityID:                 lease.ID,
			ScopeType:                "project",
			ScopeID:                  lease.ProjectID,
			ResultStatus:             "pending",
			SummaryText:              fmt.Sprintf("Requested deposit refund of %s %s for lease %s", req.Amount, lease.CurrencyCode, lease.LeaseNo),
			AfterSnapshotJson:        payloadBytes,
			RelatedApprovalRequestID: created.ID,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease contract not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		switch err.Error() {
		case "deposit refund only allowed for active, expired or terminated leases":
			return c.Status(409).JSON(fiber.Map{"error": err.Error()})
		case "amount exceeds security_deposit_amount":
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("RequestLeaseDepositRefund error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to request deposit refund"})
	}

	return c.Status(202).JSON(fiber.Map{
		"action":              "approval_requested",
		"message":             "deposit refund request submitted for approval",
		"lease_id":            id,
		"approval_request_id": formatApprovalRequestUUID(approvalRequestID),
	})
}
