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

var validContractTransitions = map[string][]string{
	"draft":      {"active", "cancelled"},
	"active":     {"completed", "cancelled", "terminated", "defaulted"},
	"completed":  {},
	"cancelled":  {},
	"terminated": {},
	"defaulted":  {},
}

func isValidContractTransition(from, to string) bool {
	allowed, ok := validContractTransitions[from]
	if !ok {
		return false
	}
	for _, t := range allowed {
		if t == to {
			return true
		}
	}
	return false
}

func validateContractAmounts(salePrice string, discount, net, down, financed *string) error {
	salePriceAmount, err := parseRequiredAmount("sale_price_amount", &salePrice)
	if err != nil {
		return err
	}
	discountAmount, err := parseRequiredAmount("discount_amount", discount)
	if err != nil {
		return err
	}
	netAmount, err := parseRequiredAmount("net_contract_amount", net)
	if err != nil {
		return err
	}
	downAmount, err := parseRequiredAmount("down_payment_amount", down)
	if err != nil {
		return err
	}
	financedAmount, err := parseRequiredAmount("financed_amount", financed)
	if err != nil {
		return err
	}

	zero := big.NewRat(0, 1)
	if salePriceAmount.Cmp(zero) < 0 || discountAmount.Cmp(zero) < 0 || netAmount.Cmp(zero) < 0 || downAmount.Cmp(zero) < 0 || financedAmount.Cmp(zero) < 0 {
		return fmt.Errorf("amounts must be non-negative")
	}
	if netAmount.Cmp(new(big.Rat).Sub(salePriceAmount, discountAmount)) != 0 {
		return fmt.Errorf("net_contract_amount must equal sale_price_amount minus discount_amount")
	}
	if financedAmount.Cmp(new(big.Rat).Sub(netAmount, downAmount)) != 0 {
		return fmt.Errorf("financed_amount must equal net_contract_amount minus down_payment_amount")
	}
	return nil
}

func parseRequiredAmount(field string, value *string) (*big.Rat, error) {
	if value == nil || *value == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	amount, ok := new(big.Rat).SetString(*value)
	if !ok {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return amount, nil
}

func ListSalesContracts(c *fiber.Ctx) error {
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

	listParams := db.ListSalesContractsParams{
		Limit:  int32(perPage),
		Offset: int32((page - 1) * perPage),
	}

	if beID := c.Query("business_entity_id"); beID != "" {
		if id, err := uuid.Parse(beID); err == nil {
			listParams.BusinessEntityID = toPgUUID(id)
		}
	}
	if branchID := c.Query("branch_id"); branchID != "" {
		if id, err := uuid.Parse(branchID); err == nil {
			listParams.BranchID = toPgUUID(id)
		}
	}
	if projectID := c.Query("project_id"); projectID != "" {
		if id, err := uuid.Parse(projectID); err == nil {
			listParams.ProjectID = toPgUUID(id)
		}
	}
	if unitID := c.Query("unit_id"); unitID != "" {
		if id, err := uuid.Parse(unitID); err == nil {
			listParams.UnitID = toPgUUID(id)
		}
	}
	if buyerID := c.Query("primary_buyer_id"); buyerID != "" {
		if id, err := uuid.Parse(buyerID); err == nil {
			listParams.PrimaryBuyerID = toPgUUID(id)
		}
	}
	if status := c.Query("status"); status != "" {
		listParams.Status = toPgText(&status)
	}

	var contracts []db.SalesContract
	var totalCount int64

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		contracts, err = q.ListSalesContracts(ctx, listParams)
		if err != nil {
			return err
		}

		countParams := db.CountSalesContractsParams{
			BusinessEntityID: listParams.BusinessEntityID,
			BranchID:         listParams.BranchID,
			ProjectID:        listParams.ProjectID,
			UnitID:           listParams.UnitID,
			PrimaryBuyerID:   listParams.PrimaryBuyerID,
			Status:           listParams.Status,
		}
		totalCount, err = q.CountSalesContracts(ctx, countParams)
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to list sales contracts"})
	}

	totalPages := int64(0)
	if perPage > 0 {
		totalPages = (totalCount + int64(perPage) - 1) / int64(perPage)
	}

	response := fiber.Map{
		"data": contracts,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total_count": totalCount,
			"total_pages": totalPages,
		},
	}

	return c.JSON(response)
}

func GetSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		contract, err = q.GetSalesContract(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to get sales contract"})
	}

	return c.JSON(contract)
}

func CreateSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	var req struct {
		BusinessEntityID      string  `json:"business_entity_id"`
		BranchID              string  `json:"branch_id"`
		ProjectID             string  `json:"project_id"`
		UnitID                string  `json:"unit_id"`
		PrimaryBuyerID        string  `json:"primary_buyer_id"`
		SourceReservationID   *string `json:"source_reservation_id,omitempty"`
		ContractDate          string  `json:"contract_date"`
		EffectiveDate         string  `json:"effective_date"`
		SalePriceAmount       string  `json:"sale_price_amount"`
		SalePriceCurrency     string  `json:"sale_price_currency"`
		DiscountAmount        *string `json:"discount_amount,omitempty"`
		NetContractAmount     *string `json:"net_contract_amount,omitempty"`
		DownPaymentAmount     *string `json:"down_payment_amount,omitempty"`
		FinancedAmount        *string `json:"financed_amount,omitempty"`
		PaymentPlanTemplateID *string `json:"payment_plan_template_id,omitempty"`
		HandoverDatePlanned   *string `json:"handover_date_planned,omitempty"`
		Notes                 *string `json:"notes,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
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
	primaryBuyerID, err := uuid.Parse(req.PrimaryBuyerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid primary_buyer_id"})
	}

	contractDate, err := time.Parse("2006-01-02", req.ContractDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid contract_date format, use YYYY-MM-DD"})
	}
	effectiveDate, err := time.Parse("2006-01-02", req.EffectiveDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid effective_date format, use YYYY-MM-DD"})
	}

	if err := validateContractAmounts(req.SalePriceAmount, req.DiscountAmount, req.NetContractAmount, req.DownPaymentAmount, req.FinancedAmount); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		activeContract, err := q.GetActiveSalesContractForUnit(ctx, toPgUUID(unitID))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if activeContract.ID.Valid {
			return errors.New("unit already has an active sales contract")
		}

		createParams := db.CreateSalesContractParams{
			BusinessEntityID:      toPgUUID(businessEntityID),
			BranchID:              toPgUUID(branchID),
			ProjectID:             toPgUUID(projectID),
			UnitID:                toPgUUID(unitID),
			PrimaryBuyerID:        toPgUUID(primaryBuyerID),
			SourceReservationID:   toPgUUIDFromString(req.SourceReservationID),
			ContractDate:          pgtype.Date{Time: contractDate, Valid: true},
			EffectiveDate:         pgtype.Date{Time: effectiveDate, Valid: true},
			SalePriceAmount:       toPgNumeric(&req.SalePriceAmount),
			SalePriceCurrency:     req.SalePriceCurrency,
			DiscountAmount:        toPgNumeric(req.DiscountAmount),
			NetContractAmount:     toPgNumeric(req.NetContractAmount),
			DownPaymentAmount:     toPgNumeric(req.DownPaymentAmount),
			FinancedAmount:        toPgNumeric(req.FinancedAmount),
			PaymentPlanTemplateID: toPgUUIDFromString(req.PaymentPlanTemplateID),
			HandoverDatePlanned:   toPgDate(req.HandoverDatePlanned),
			Notes:                 toPgText(req.Notes),
			CreatedByUserID:       toPgUUID(userID),
		}

		contract, err = q.CreateSalesContract(ctx, createParams)
		if err != nil {
			return err
		}

		partyParams := db.CreateSalesContractPartyParams{
			SalesContractID: toPgUUID(contract.ID.Bytes),
			PartyID:         toPgUUID(primaryBuyerID),
			Role:            "primary_buyer",
			IsPrimary:       true,
			EffectiveFrom:   pgtype.Date{Time: time.Now(), Valid: true},
			Status:          "active",
		}
		_, err = q.CreateSalesContractParty(ctx, partyParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "create_contract",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(contract.ID.Bytes),
			ScopeType:         "project",
			ScopeID:           toPgUUID(projectID),
			ResultStatus:      "success",
			SummaryText:       "Created sales contract",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if err.Error() == "unit already has an active sales contract" {
			return c.Status(409).JSON(fiber.Map{"error": "unit already has an active sales contract"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "sales contract already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to create sales contract"})
	}

	return c.Status(201).JSON(contract)
}

func UpdateSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	var req struct {
		SalePriceAmount       *string `json:"sale_price_amount,omitempty"`
		DiscountAmount        *string `json:"discount_amount,omitempty"`
		NetContractAmount     *string `json:"net_contract_amount,omitempty"`
		DownPaymentAmount     *string `json:"down_payment_amount,omitempty"`
		FinancedAmount        *string `json:"financed_amount,omitempty"`
		PaymentPlanTemplateID *string `json:"payment_plan_template_id,omitempty"`
		HandoverDatePlanned   *string `json:"handover_date_planned,omitempty"`
		HandoverDateActual    *string `json:"handover_date_actual,omitempty"`
		Notes                 *string `json:"notes,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "draft" {
			return errors.New("can only update draft contracts")
		}

		updateParams := db.UpdateSalesContractParams{
			ID:                    toPgUUID(id),
			SalePriceAmount:       toPgNumeric(req.SalePriceAmount),
			DiscountAmount:        toPgNumeric(req.DiscountAmount),
			NetContractAmount:     toPgNumeric(req.NetContractAmount),
			DownPaymentAmount:     toPgNumeric(req.DownPaymentAmount),
			FinancedAmount:        toPgNumeric(req.FinancedAmount),
			PaymentPlanTemplateID: toPgUUIDFromString(req.PaymentPlanTemplateID),
			HandoverDatePlanned:   toPgDate(req.HandoverDatePlanned),
			HandoverDateActual:    toPgDate(req.HandoverDateActual),
			Notes:                 toPgText(req.Notes),
		}

		contract, err = q.UpdateSalesContract(ctx, updateParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "update_contract",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Updated sales contract",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only update draft contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only update draft contracts"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to update sales contract"})
	}

	return c.JSON(contract)
}

func ActivateSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract
	var scheduleLines []db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "draft" {
			return errors.New("can only activate draft contracts")
		}

		scheduleLines, err = q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if len(scheduleLines) == 0 {
			return errors.New("cannot activate contract with no schedule lines")
		}

		for i, line := range scheduleLines {
			receivableNo := fmt.Sprintf("REC-%s-%d", existing.ContractNo, line.LineNo)
			receivableParams := db.CreateReceivableParams{
				BusinessEntityID: existing.BusinessEntityID,
				BranchID:         existing.BranchID,
				PartyID:          existing.PrimaryBuyerID,
				UnitID:           existing.UnitID,
				SourceModule:     "sales",
				SourceRecordType: "installment_schedule_line",
				SourceRecordID:   toPgUUID(line.ID.Bytes),
				ReceivableNo:     toPgText(&receivableNo),
				ReceivableDate:   pgtype.Date{Time: time.Now(), Valid: true},
				DueDate:          line.DueDate,
				CurrencyCode:     existing.SalePriceCurrency,
				OriginalAmount:   line.PrincipalAmount,
				Notes:            toPgText(strPtr(fmt.Sprintf("Installment %d: %s", line.LineNo, line.Description.String))),
			}

			receivable, err := q.CreateReceivable(ctx, receivableParams)
			if err != nil {
				return fmt.Errorf("failed to create receivable for line %d: %w", line.LineNo, err)
			}

			linkParams := db.LinkScheduleLineReceivableParams{
				ID:           toPgUUID(line.ID.Bytes),
				ReceivableID: toPgUUID(receivable.ID.Bytes),
			}
			_, err = q.LinkScheduleLineReceivable(ctx, linkParams)
			if err != nil {
				return fmt.Errorf("failed to link receivable for line %d: %w", line.LineNo, err)
			}

			scheduleLines[i].ReceivableID = toPgUUID(receivable.ID.Bytes)
		}

		statusParams := db.UpdateSalesContractStatusParams{
			ID:       toPgUUID(id),
			Status:   "active",
			Status_2: "draft",
		}
		contract, err = q.UpdateSalesContractStatus(ctx, statusParams)
		if err != nil {
			return err
		}

		updateUnitParams := db.UpdateUnitParams{
			ID:          existing.UnitID,
			SalesStatus: toPgText(strPtr("sold")),
		}
		_, err = q.UpdateUnit(ctx, updateUnitParams)
		if err != nil {
			return fmt.Errorf("failed to update unit sales status: %w", err)
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "activate_contract",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Activated sales contract with %d receivables created", len(scheduleLines)),
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only activate draft contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only activate draft contracts"})
		}
		if err.Error() == "cannot activate contract with no schedule lines" {
			return c.Status(400).JSON(fiber.Map{"error": "cannot activate contract with no schedule lines"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to activate sales contract"})
	}

	return c.JSON(fiber.Map{
		"message":             "sales contract activated",
		"contract":            contract,
		"receivables_created": len(scheduleLines),
	})
}

func CancelSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	var req struct {
		Reason *string `json:"reason,omitempty"`
	}

	c.BodyParser(&req)

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status == "draft" {
			statusParams := db.UpdateSalesContractStatusParams{
				ID:       toPgUUID(id),
				Status:   "cancelled",
				Status_2: "draft",
			}
			contract, err = q.UpdateSalesContractStatus(ctx, statusParams)
			if err != nil {
				return err
			}

			updateUnitParams := db.UpdateUnitParams{
				ID:          existing.UnitID,
				SalesStatus: toPgText(strPtr("available")),
			}
			_, err = q.UpdateUnit(ctx, updateUnitParams)
			if err != nil {
				return err
			}

			afterSnapshot, _ := json.Marshal(contract)
			auditParams := db.InsertAuditLogParams{
				EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UserID:            toPgUUID(userID),
				Module:            "sales",
				ActionType:        "cancel_contract",
				EntityType:        "sales_contract",
				EntityID:          toPgUUID(id),
				ScopeType:         "project",
				ScopeID:           existing.ProjectID,
				ResultStatus:      "success",
				SummaryText:       "Cancelled draft sales contract",
				AfterSnapshotJson: afterSnapshot,
			}
			_, err = q.InsertAuditLog(ctx, auditParams)
			return err
		}

		if existing.Status == "active" {
			approvalReqParams := db.CreateApprovalRequestParams{
				BusinessEntityID:    existing.BusinessEntityID,
				BranchID:            existing.BranchID,
				Module:              "sales",
				RequestType:         "contract_cancellation",
				SourceRecordType:    "sales_contract",
				SourceRecordID:      toPgUUID(id),
				RequestedByUserID:   toPgUUID(userID),
				Status:              "pending",
				PayloadSnapshotJson: []byte(fmt.Sprintf(`{"reason": "%s"}`, ptrToString(req.Reason))),
			}
			_, err = q.CreateApprovalRequest(ctx, approvalReqParams)
			if err != nil {
				return err
			}

			afterSnapshot, _ := json.Marshal(existing)
			auditParams := db.InsertAuditLogParams{
				EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UserID:            toPgUUID(userID),
				Module:            "sales",
				ActionType:        "request_contract_cancellation",
				EntityType:        "sales_contract",
				EntityID:          toPgUUID(id),
				ScopeType:         "project",
				ScopeID:           existing.ProjectID,
				ResultStatus:      "success",
				SummaryText:       "Requested approval for contract cancellation",
				AfterSnapshotJson: afterSnapshot,
			}
			_, err = q.InsertAuditLog(ctx, auditParams)
			if err != nil {
				return err
			}

			return errors.New("approval_required")
		}

		return errors.New("invalid contract status for cancellation")
	})

	if err != nil {
		if err.Error() == "approval_required" {
			return c.Status(202).JSON(fiber.Map{
				"message":     "cancellation request submitted for approval",
				"contract_id": id,
			})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "invalid contract status for cancellation" {
			return c.Status(409).JSON(fiber.Map{"error": "invalid contract status for cancellation"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to cancel sales contract"})
	}

	return c.JSON(fiber.Map{
		"message":  "sales contract cancelled",
		"contract": contract,
	})
}

func TerminateSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	var req struct {
		Reason *string `json:"reason,omitempty"`
	}

	c.BodyParser(&req)

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var existing db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err = q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "active" {
			return errors.New("can only terminate active contracts")
		}

		approvalReqParams := db.CreateApprovalRequestParams{
			BusinessEntityID:    existing.BusinessEntityID,
			BranchID:            existing.BranchID,
			Module:              "sales",
			RequestType:         "contract_termination",
			SourceRecordType:    "sales_contract",
			SourceRecordID:      toPgUUID(id),
			RequestedByUserID:   toPgUUID(userID),
			Status:              "pending",
			PayloadSnapshotJson: []byte(fmt.Sprintf(`{"reason": "%s"}`, ptrToString(req.Reason))),
		}
		_, err = q.CreateApprovalRequest(ctx, approvalReqParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(existing)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "request_contract_termination",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           existing.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Requested approval for contract termination",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only terminate active contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only terminate active contracts"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to request contract termination"})
	}

	return c.Status(202).JSON(fiber.Map{
		"message":     "termination request submitted for approval",
		"contract_id": id,
	})
}

func CompleteSalesContract(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "active" {
			return errors.New("can only complete active contracts")
		}

		scheduleLines, err := q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		for _, line := range scheduleLines {
			if line.Status != "paid" {
				return errors.New("all schedule lines must be paid before completing contract")
			}
		}

		statusParams := db.UpdateSalesContractStatusParams{
			ID:       toPgUUID(id),
			Status:   "completed",
			Status_2: "active",
		}
		contract, err = q.UpdateSalesContractStatus(ctx, statusParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "complete_contract",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Completed sales contract - all receivables settled",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only complete active contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only complete active contracts"})
		}
		if err.Error() == "all schedule lines must be paid before completing contract" {
			return c.Status(400).JSON(fiber.Map{"error": "all schedule lines must be paid before completing contract"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to complete sales contract"})
	}

	return c.JSON(fiber.Map{
		"message":  "sales contract completed",
		"contract": contract,
	})
}

func MarkSalesContractDefaulted(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var contract db.SalesContract

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "active" {
			return errors.New("can only mark active contracts as defaulted")
		}

		statusParams := db.UpdateSalesContractStatusParams{
			ID:       toPgUUID(id),
			Status:   "defaulted",
			Status_2: "active",
		}
		contract, err = q.UpdateSalesContractStatus(ctx, statusParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "mark_contract_defaulted",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Marked sales contract as defaulted",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only mark active contracts as defaulted" {
			return c.Status(409).JSON(fiber.Map{"error": "can only mark active contracts as defaulted"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to mark contract as defaulted"})
	}

	return c.JSON(fiber.Map{
		"message":  "sales contract marked as defaulted",
		"contract": contract,
	})
}

func GetSalesContractSchedule(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var scheduleLines []db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		_, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		scheduleLines, err = q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to get schedule"})
	}

	return c.JSON(fiber.Map{
		"data": scheduleLines,
	})
}

func GenerateSalesContractSchedule(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	var req struct {
		Lines []struct {
			DueDate         string  `json:"due_date"`
			LineType        string  `json:"line_type"`
			Description     *string `json:"description,omitempty"`
			PrincipalAmount string  `json:"principal_amount"`
		} `json:"lines"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if len(req.Lines) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "at least one schedule line is required"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var scheduleLines []db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		contract, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if contract.Status != "draft" {
			return errors.New("can only generate schedule for draft contracts")
		}

		existingLines, err := q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if len(existingLines) > 0 {
			return errors.New("schedule already exists for this contract")
		}

		for i, line := range req.Lines {
			dueDate, err := time.Parse("2006-01-02", line.DueDate)
			if err != nil {
				return fmt.Errorf("invalid due_date for line %d: use YYYY-MM-DD", i+1)
			}

			nextLineNo, err := q.GetNextScheduleLineNumber(ctx, toPgUUID(id))
			if err != nil {
				return err
			}

			createParams := db.CreateInstallmentScheduleLineParams{
				SalesContractID: toPgUUID(id),
				LineNo:          int16(nextLineNo),
				DueDate:         pgtype.Date{Time: dueDate, Valid: true},
				LineType:        line.LineType,
				Description:     toPgText(line.Description),
				PrincipalAmount: toPgNumeric(&line.PrincipalAmount),
				Status:          "pending",
			}

			scheduleLine, err := q.CreateInstallmentScheduleLine(ctx, createParams)
			if err != nil {
				return fmt.Errorf("failed to create schedule line %d: %w", i+1, err)
			}

			scheduleLines = append(scheduleLines, scheduleLine)
		}

		afterSnapshot, _ := json.Marshal(scheduleLines)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "generate_schedule",
			EntityType:        "installment_schedule",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Generated schedule with %d lines", len(scheduleLines)),
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only generate schedule for draft contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only generate schedule for draft contracts"})
		}
		if err.Error() == "schedule already exists for this contract" {
			return c.Status(409).JSON(fiber.Map{"error": "schedule already exists for this contract"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate schedule"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "schedule generated",
		"data":    scheduleLines,
	})
}

func ListInstallmentScheduleLines(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	contractIDStr := c.Params("id")
	contractID, err := uuid.Parse(contractIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid contract id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var lines []db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		lines, err = q.ListInstallmentScheduleLines(ctx, toPgUUID(contractID))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if lines == nil {
		lines = []db.InstallmentScheduleLine{}
	}

	return c.JSON(fiber.Map{"data": lines})
}

func AddInstallmentScheduleLine(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	contractIDStr := c.Params("id")
	contractID, err := uuid.Parse(contractIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid contract id"})
	}

	var req struct {
		DueDate               string  `json:"due_date"`
		LineType              string  `json:"line_type"`
		Description           string  `json:"description"`
		PrincipalAmount       string  `json:"principal_amount"`
		PenaltyAmountAccrued  *string `json:"penalty_amount_accrued,omitempty"`
		DiscountAmountApplied *string `json:"discount_amount_applied,omitempty"`
		Status                string  `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.DueDate == "" || req.LineType == "" || req.PrincipalAmount == "" {
		return c.Status(400).JSON(fiber.Map{"error": "due_date, line_type, and principal_amount are required"})
	}

	if req.Status == "" {
		req.Status = "pending"
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var line db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		contract, err := q.GetSalesContract(ctx, toPgUUID(contractID))
		if err != nil {
			return err
		}

		if contract.Status != "draft" {
			return errors.New("can only add schedule lines to draft contracts")
		}

		nextLineNo, err := q.GetNextScheduleLineNumber(ctx, toPgUUID(contractID))
		if err != nil {
			return err
		}

		createParams := db.CreateInstallmentScheduleLineParams{
			SalesContractID:       toPgUUID(contractID),
			ReceivableID:          pgtype.UUID{Valid: false},
			LineNo:                int16(nextLineNo),
			DueDate:               toPgDate(&req.DueDate),
			LineType:              req.LineType,
			Description:           toPgText(&req.Description),
			PrincipalAmount:       toPgNumeric(&req.PrincipalAmount),
			PenaltyAmountAccrued:  toPgNumeric(req.PenaltyAmountAccrued),
			DiscountAmountApplied: toPgNumeric(req.DiscountAmountApplied),
			AmountPaid:            toPgNumeric(nil),
			Status:                req.Status,
		}

		line, err = q.CreateInstallmentScheduleLine(ctx, createParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(line)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "add_schedule_line",
			EntityType:        "installment_schedule_line",
			EntityID:          line.ID,
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Added installment schedule line",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if err.Error() == "can only add schedule lines to draft contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only add schedule lines to draft contracts"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to add schedule line"})
	}

	return c.Status(201).JSON(line)
}

func UpdateInstallmentScheduleLine(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid schedule line id"})
	}

	var req struct {
		DueDate               *string `json:"due_date,omitempty"`
		Description           *string `json:"description,omitempty"`
		PrincipalAmount       *string `json:"principal_amount,omitempty"`
		PenaltyAmountAccrued  *string `json:"penalty_amount_accrued,omitempty"`
		DiscountAmountApplied *string `json:"discount_amount_applied,omitempty"`
		Status                *string `json:"status,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var line db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetInstallmentScheduleLine(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		contract, err := q.GetSalesContract(ctx, existing.SalesContractID)
		if err != nil {
			return err
		}

		if contract.Status != "draft" {
			return errors.New("can only update schedule lines for draft contracts")
		}

		updateParams := db.UpdateInstallmentScheduleLineParams{
			ID:                    toPgUUID(id),
			DueDate:               toPgDate(req.DueDate),
			Description:           toPgText(req.Description),
			PrincipalAmount:       toPgNumeric(req.PrincipalAmount),
			PenaltyAmountAccrued:  toPgNumeric(req.PenaltyAmountAccrued),
			DiscountAmountApplied: toPgNumeric(req.DiscountAmountApplied),
		}

		if req.Status != nil {
			updateParams.Status = toPgText(req.Status)
		}

		line, err = q.UpdateInstallmentScheduleLine(ctx, updateParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(line)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "update_schedule_line",
			EntityType:        "installment_schedule_line",
			EntityID:          toPgUUID(id),
			ScopeType:         "project",
			ScopeID:           contract.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Updated installment schedule line",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "schedule line not found"})
		}
		if err.Error() == "can only update schedule lines for draft contracts" {
			return c.Status(409).JSON(fiber.Map{"error": "can only update schedule lines for draft contracts"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to update schedule line"})
	}

	return c.JSON(line)
}

func toPgDate(v *string) pgtype.Date {
	if v == nil || *v == "" {
		return pgtype.Date{Valid: false}
	}
	t, err := time.Parse("2006-01-02", *v)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}
