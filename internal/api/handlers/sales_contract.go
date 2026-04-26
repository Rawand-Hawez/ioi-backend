package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

type derivedContractAmounts struct {
	Discount string
	Net      string
	Down     string
	Financed string
}

func deriveContractAmounts(salePrice string, discount, net, down, financed *string) (derivedContractAmounts, error) {
	salePriceAmount, err := parseRequiredAmount("sale_price_amount", &salePrice)
	if err != nil {
		return derivedContractAmounts{}, err
	}
	discountAmount, err := parseOptionalAmount("discount_amount", discount, big.NewRat(0, 1))
	if err != nil {
		return derivedContractAmounts{}, err
	}
	downAmount, err := parseOptionalAmount("down_payment_amount", down, big.NewRat(0, 1))
	if err != nil {
		return derivedContractAmounts{}, err
	}

	expectedNetAmount := new(big.Rat).Sub(salePriceAmount, discountAmount)
	netAmount, err := parseOptionalAmount("net_contract_amount", net, expectedNetAmount)
	if err != nil {
		return derivedContractAmounts{}, err
	}
	if net != nil && *net != "" && netAmount.Cmp(expectedNetAmount) != 0 {
		return derivedContractAmounts{}, fmt.Errorf("net_contract_amount must equal sale_price_amount minus discount_amount")
	}

	expectedFinancedAmount := new(big.Rat).Sub(netAmount, downAmount)
	financedAmount, err := parseOptionalAmount("financed_amount", financed, expectedFinancedAmount)
	if err != nil {
		return derivedContractAmounts{}, err
	}
	if financed != nil && *financed != "" && financedAmount.Cmp(expectedFinancedAmount) != 0 {
		return derivedContractAmounts{}, fmt.Errorf("financed_amount must equal net_contract_amount minus down_payment_amount")
	}

	zero := big.NewRat(0, 1)
	if salePriceAmount.Cmp(zero) < 0 || discountAmount.Cmp(zero) < 0 || netAmount.Cmp(zero) < 0 || downAmount.Cmp(zero) < 0 || financedAmount.Cmp(zero) < 0 {
		return derivedContractAmounts{}, fmt.Errorf("amounts must be non-negative")
	}

	return derivedContractAmounts{
		Discount: formatContractAmount(discountAmount),
		Net:      formatContractAmount(netAmount),
		Down:     formatContractAmount(downAmount),
		Financed: formatContractAmount(financedAmount),
	}, nil
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

func parseOptionalAmount(field string, value *string, defaultAmount *big.Rat) (*big.Rat, error) {
	if value == nil || *value == "" {
		return new(big.Rat).Set(defaultAmount), nil
	}
	return parseRequiredAmount(field, value)
}

func numericContractAmount(field string, amount pgtype.Numeric) (string, error) {
	if !amount.Valid {
		return "", fmt.Errorf("%s is required", field)
	}
	return formatContractAmount(numericToRat(&amount)), nil
}

type generateSalesContractScheduleRequest struct {
	ContractDate string `json:"contract_date"`
	HandoverDate string `json:"handover_date"`
}

type salesContractAmountView struct {
	NetContractAmount pgtype.Numeric
}

type scheduleAnchors struct {
	ContractDate      time.Time
	HandoverDate      time.Time
	HandoverDateValid bool
}

type scheduleLineInput struct {
	DueDate         time.Time
	LineType        string
	Description     string
	PrincipalAmount string
}

type scheduleLineDraft struct {
	dueDate     time.Time
	lineType    string
	description string
	amount      *big.Rat
}

func buildScheduleLinesFromTemplate(contract salesContractAmountView, rule paymentPlanGenerationRule, anchors scheduleAnchors) ([]scheduleLineInput, error) {
	netAmount := numericToRat(&contract.NetContractAmount)
	if netAmount == nil || netAmount.Cmp(big.NewRat(0, 1)) <= 0 {
		return nil, errors.New("net_contract_amount must be positive")
	}

	drafts := []scheduleLineDraft{
		{
			dueDate:     scheduleDueDate(rule.DownPayment, anchors, 0),
			lineType:    "down_payment",
			description: "Down payment",
			amount:      percentageAmount(netAmount, rule.DownPayment.Percentage.value, 1),
		},
	}

	for _, tranche := range rule.Tranches {
		lineType := tranche.LineType
		if lineType == "" {
			lineType = "installment"
		}
		for i := int32(0); i < tranche.InstallmentCount; i++ {
			drafts = append(drafts, scheduleLineDraft{
				dueDate:     scheduleDueDate(tranche, anchors, i),
				lineType:    lineType,
				description: fmt.Sprintf("Installment %d", i+1),
				amount:      percentageAmount(netAmount, tranche.Percentage.value, tranche.InstallmentCount),
			})
		}
	}

	totalCents := roundRatToCents(netAmount)
	var allocatedCents int64
	lines := make([]scheduleLineInput, 0, len(drafts))
	for i, draft := range drafts {
		cents := totalCents - allocatedCents
		if i < len(drafts)-1 {
			cents = roundRatToCents(draft.amount)
			allocatedCents += cents
		}
		lines = append(lines, scheduleLineInput{
			DueDate:         draft.dueDate,
			LineType:        draft.lineType,
			Description:     draft.description,
			PrincipalAmount: formatCents(cents),
		})
	}
	return lines, nil
}

func percentageAmount(base *big.Rat, percentage *big.Rat, count int32) *big.Rat {
	amount := new(big.Rat).Mul(base, percentage)
	amount.Quo(amount, big.NewRat(100, 1))
	if count > 1 {
		amount.Quo(amount, big.NewRat(int64(count), 1))
	}
	return amount
}

func scheduleDueDate(segment paymentPlanRuleSegment, anchors scheduleAnchors, installmentIndex int32) time.Time {
	dueDate := anchorDate(segment.Anchor, anchors).AddDate(0, 0, int(segment.OffsetDays))
	switch segment.Frequency {
	case "monthly":
		return addMonthsClamped(dueDate, int(installmentIndex))
	case "quarterly":
		return addMonthsClamped(dueDate, int(installmentIndex)*3)
	case "semiannual":
		return addMonthsClamped(dueDate, int(installmentIndex)*6)
	case "annual":
		return addMonthsClamped(dueDate, int(installmentIndex)*12)
	default:
		return dueDate
	}
}

func anchorDate(anchor string, anchors scheduleAnchors) time.Time {
	switch anchor {
	case "handover_date":
		return anchors.HandoverDate
	default:
		return anchors.ContractDate
	}
}

func usesHandoverDate(rule paymentPlanGenerationRule) bool {
	if rule.DownPayment.Anchor == "handover_date" {
		return true
	}
	for _, tranche := range rule.Tranches {
		if tranche.Anchor == "handover_date" {
			return true
		}
	}
	return false
}

func scheduleGenerationAnchors(req generateSalesContractScheduleRequest, contract db.SalesContract, rule paymentPlanGenerationRule) (scheduleAnchors, error) {
	contractDate := contract.ContractDate.Time
	if req.ContractDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ContractDate)
		if err != nil {
			return scheduleAnchors{}, errors.New("invalid contract_date format, use YYYY-MM-DD")
		}
		contractDate = parsed
	}
	if contractDate.IsZero() {
		return scheduleAnchors{}, errors.New("contract_date is required")
	}

	anchors := scheduleAnchors{ContractDate: contractDate}
	if req.HandoverDate != "" {
		handoverDate, err := time.Parse("2006-01-02", req.HandoverDate)
		if err != nil {
			return scheduleAnchors{}, errors.New("invalid handover_date format, use YYYY-MM-DD")
		}
		anchors.HandoverDate = handoverDate
		anchors.HandoverDateValid = true
	}
	if usesHandoverDate(rule) && !anchors.HandoverDateValid {
		return scheduleAnchors{}, errors.New("handover_date is required for handover_date anchored schedule rules")
	}
	return anchors, nil
}

func createScheduleLinesFromTemplate(ctx context.Context, q *db.Queries, contractID uuid.UUID, contract db.SalesContract, req generateSalesContractScheduleRequest) ([]db.InstallmentScheduleLine, error) {
	if !contract.PaymentPlanTemplateID.Valid {
		return nil, errors.New("payment_plan_template_id is required")
	}

	template, err := q.GetPaymentPlanTemplate(ctx, contract.PaymentPlanTemplateID)
	if err != nil {
		return nil, err
	}
	rule, err := parsePaymentPlanGenerationRule(json.RawMessage(template.GenerationRuleJson), template.InstallmentCount)
	if err != nil {
		return nil, err
	}
	anchors, err := scheduleGenerationAnchors(req, contract, rule)
	if err != nil {
		return nil, err
	}
	lineInputs, err := buildScheduleLinesFromTemplate(salesContractAmountView{NetContractAmount: contract.NetContractAmount}, rule, anchors)
	if err != nil {
		return nil, err
	}

	zeroAmount := "0.00"
	scheduleLines := make([]db.InstallmentScheduleLine, 0, len(lineInputs))
	for i, line := range lineInputs {
		createParams := db.CreateInstallmentScheduleLineParams{
			SalesContractID:       toPgUUID(contractID),
			LineNo:                int16(i + 1),
			DueDate:               pgtype.Date{Time: line.DueDate, Valid: true},
			LineType:              line.LineType,
			Description:           toPgText(&line.Description),
			PrincipalAmount:       toPgNumeric(&line.PrincipalAmount),
			PenaltyAmountAccrued:  toPgNumeric(&zeroAmount),
			DiscountAmountApplied: toPgNumeric(&zeroAmount),
			AmountPaid:            toPgNumeric(&zeroAmount),
			Status:                "scheduled",
		}

		scheduleLine, err := q.CreateInstallmentScheduleLine(ctx, createParams)
		if err != nil {
			return nil, fmt.Errorf("failed to create schedule line %d: %w", i+1, err)
		}

		scheduleLines = append(scheduleLines, scheduleLine)
	}
	return scheduleLines, nil
}

func isScheduleGenerationBadRequest(err error) bool {
	msg := err.Error()
	return msg == "payment_plan_template_id is required" ||
		msg == "net_contract_amount must be positive" ||
		strings.Contains(msg, "generation_rule_json") ||
		strings.Contains(msg, "installment_count") ||
		strings.Contains(msg, "anchor") ||
		strings.Contains(msg, "frequency") ||
		strings.Contains(msg, "percentage") ||
		strings.Contains(msg, "contract_date") ||
		strings.Contains(msg, "handover_date")
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

	amounts, err := deriveContractAmounts(req.SalePriceAmount, req.DiscountAmount, req.NetContractAmount, req.DownPaymentAmount, req.FinancedAmount)
	if err != nil {
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
			DiscountAmount:        toPgNumeric(&amounts.Discount),
			NetContractAmount:     toPgNumeric(&amounts.Net),
			DownPaymentAmount:     toPgNumeric(&amounts.Down),
			FinancedAmount:        toPgNumeric(&amounts.Financed),
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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
			return ErrSalesNotDraft
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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
			return ErrSalesActivateNotDraft
		}

		activeContract, err := q.GetActiveSalesContractForUnit(ctx, existing.UnitID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if activeContract.ID.Valid && activeContract.ID.Bytes != id {
			return ErrSalesUnitHasActiveContract
		}

		scheduleLines, err = q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if len(scheduleLines) == 0 {
			scheduleLines, err = createScheduleLinesFromTemplate(ctx, q, id, existing, generateSalesContractScheduleRequest{})
			if err != nil {
				if err.Error() == "payment_plan_template_id is required" {
					return ErrSalesActivateNoScheduleLines
				}
				return err
			}
		}

		for i, line := range scheduleLines {
			if line.ReceivableID.Valid {
				receivable, err := q.GetReceivable(ctx, line.ReceivableID)
				if err != nil {
					return err
				}
				if receivable.SourceModule != "sales" ||
					receivable.SourceRecordType != "installment_schedule_line" ||
					receivable.SourceRecordID.Bytes != line.ID.Bytes {
					return ErrSalesReceivableMismatch
				}
				continue
			}

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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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

type approvalGatedOutcome struct {
	Action            string
	ApprovalRequestID pgtype.UUID
	Contract          db.SalesContract
}

func handleSalesContractApprovalGate(
	ctx context.Context,
	q *db.Queries,
	contract db.SalesContract,
	requestType string,
	targetStatus string,
	actionLabel string,
	requestedByUserID uuid.UUID,
	reason *string,
) (approvalGatedOutcome, error) {
	out := approvalGatedOutcome{Contract: contract}

	latest, lerr := q.GetLatestSalesApprovalRequest(ctx, db.GetLatestSalesApprovalRequestParams{
		SourceRecordType: "sales_contract",
		SourceRecordID:   contract.ID,
		RequestType:      requestType,
	})
	if lerr != nil && !errors.Is(lerr, pgx.ErrNoRows) {
		return out, lerr
	}

	if lerr == nil {
		switch latest.Status {
		case "approved":
			updated, err := q.UpdateSalesContractStatus(ctx, db.UpdateSalesContractStatusParams{
				ID:       contract.ID,
				Status:   targetStatus,
				Status_2: "active",
			})
			if err != nil {
				return out, err
			}
			if _, err := q.UpdateUnit(ctx, db.UpdateUnitParams{
				ID:          contract.UnitID,
				SalesStatus: toPgText(strPtr("available")),
			}); err != nil {
				return out, fmt.Errorf("failed to reset unit sales status: %w", err)
			}
			beforeSnapshot, _ := json.Marshal(contract)
			afterSnapshot, _ := json.Marshal(updated)
			if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
				EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UserID:                   toPgUUID(requestedByUserID),
				Module:                   "sales",
				ActionType:               fmt.Sprintf("apply_contract_%s", actionLabel),
				EntityType:               "sales_contract",
				EntityID:                 contract.ID,
				ScopeType:                "project",
				ScopeID:                  contract.ProjectID,
				ResultStatus:             "success",
				SummaryText:              fmt.Sprintf("Applied approved contract %s", actionLabel),
				BeforeSnapshotJson:       beforeSnapshot,
				AfterSnapshotJson:        afterSnapshot,
				RelatedApprovalRequestID: latest.ID,
			}); err != nil {
				return out, err
			}
			out.Action = "applied"
			out.ApprovalRequestID = latest.ID
			out.Contract = updated
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

	payload := []byte("{}")
	if reason != nil {
		if b, err := json.Marshal(map[string]string{"reason": *reason}); err == nil {
			payload = b
		}
	}
	created, err := q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
		BusinessEntityID:    contract.BusinessEntityID,
		BranchID:            contract.BranchID,
		Module:              "sales",
		RequestType:         requestType,
		SourceRecordType:    "sales_contract",
		SourceRecordID:      contract.ID,
		RequestedByUserID:   toPgUUID(requestedByUserID),
		Status:              "pending",
		PayloadSnapshotJson: payload,
	})
	if err != nil {
		return out, err
	}

	afterSnapshot, _ := json.Marshal(contract)
	if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UserID:                   toPgUUID(requestedByUserID),
		Module:                   "sales",
		ActionType:               fmt.Sprintf("request_contract_%s", actionLabel),
		EntityType:               "sales_contract",
		EntityID:                 contract.ID,
		ScopeType:                "project",
		ScopeID:                  contract.ProjectID,
		ResultStatus:             "success",
		SummaryText:              fmt.Sprintf("Requested approval for contract %s", actionLabel),
		AfterSnapshotJson:        afterSnapshot,
		RelatedApprovalRequestID: created.ID,
	}); err != nil {
		return out, err
	}

	out.Action = "pending_new"
	out.ApprovalRequestID = created.ID
	return out, nil
}

func formatApprovalRequestUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
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

	var outcome approvalGatedOutcome

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status == "draft" {
			updated, err := q.UpdateSalesContractStatus(ctx, db.UpdateSalesContractStatusParams{
				ID:       toPgUUID(id),
				Status:   "cancelled",
				Status_2: "draft",
			})
			if err != nil {
				return err
			}

			if _, err := q.UpdateUnit(ctx, db.UpdateUnitParams{
				ID:          existing.UnitID,
				SalesStatus: toPgText(strPtr("available")),
			}); err != nil {
				return err
			}

			afterSnapshot, _ := json.Marshal(updated)
			if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
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
			}); err != nil {
				return err
			}

			outcome = approvalGatedOutcome{Action: "applied", Contract: updated}
			return nil
		}

		if existing.Status == "active" {
			o, err := handleSalesContractApprovalGate(ctx, q, existing, "contract_cancellation", "cancelled", "cancellation", userID, req.Reason)
			if err != nil {
				return err
			}
			outcome = o
			return nil
		}

		return ErrSalesCancelInvalidStatus
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to cancel sales contract"})
	}

	switch outcome.Action {
	case "applied":
		resp := fiber.Map{
			"message":  "sales contract cancelled",
			"contract": outcome.Contract,
		}
		if outcome.ApprovalRequestID.Valid {
			resp["approval_request_id"] = formatApprovalRequestUUID(outcome.ApprovalRequestID)
		}
		return c.JSON(resp)
	case "pending_new", "pending_existing":
		return c.Status(202).JSON(fiber.Map{
			"message":             "cancellation request submitted for approval",
			"contract_id":         id,
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	case "rejected":
		return c.Status(409).JSON(fiber.Map{
			"error":               "latest cancellation request was rejected",
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	default:
		log.Printf("unexpected cancel outcome: %q", outcome.Action)
		return c.Status(500).JSON(fiber.Map{"error": "failed to cancel sales contract"})
	}
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

	var outcome approvalGatedOutcome

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if existing.Status != "active" {
			return ErrSalesTerminateNotActive
		}

		o, err := handleSalesContractApprovalGate(ctx, q, existing, "contract_termination", "terminated", "termination", userID, req.Reason)
		if err != nil {
			return err
		}
		outcome = o
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to terminate sales contract"})
	}

	switch outcome.Action {
	case "applied":
		return c.JSON(fiber.Map{
			"message":             "sales contract terminated",
			"contract":            outcome.Contract,
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	case "pending_new", "pending_existing":
		return c.Status(202).JSON(fiber.Map{
			"message":             "termination request submitted for approval",
			"contract_id":         id,
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	case "rejected":
		return c.Status(409).JSON(fiber.Map{
			"error":               "latest termination request was rejected",
			"approval_request_id": formatApprovalRequestUUID(outcome.ApprovalRequestID),
		})
	default:
		log.Printf("unexpected terminate outcome: %q", outcome.Action)
		return c.Status(500).JSON(fiber.Map{"error": "failed to terminate sales contract"})
	}
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
			return ErrSalesCompleteNotActive
		}

		scheduleLines, err := q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if len(scheduleLines) == 0 {
			return ErrSalesCompleteNoSchedule
		}

		zero := big.NewRat(0, 1)
		for _, line := range scheduleLines {
			if !line.ReceivableID.Valid {
				return ErrSalesCompleteNoReceivable
			}
			receivable, err := q.GetReceivable(ctx, line.ReceivableID)
			if err != nil {
				return fmt.Errorf("failed to load receivable for schedule line: %w", err)
			}
			if receivable.Status != "paid" {
				return ErrSalesCompleteReceivableOpen
			}
			outstanding := numericToRat(&receivable.OutstandingAmount)
			if outstanding == nil || outstanding.Cmp(zero) != 0 {
				return ErrSalesCompleteOutstanding
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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
			return ErrSalesDefaultNotActive
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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

	var req generateSalesContractScheduleRequest
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

	var scheduleLines []db.InstallmentScheduleLine

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		contract, err := q.GetSalesContract(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if contract.Status != "draft" {
			return ErrSalesScheduleNotDraft
		}
		if !contract.PaymentPlanTemplateID.Valid {
			return errors.New("payment_plan_template_id is required")
		}

		existingLines, err := q.ListInstallmentScheduleLines(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		if len(existingLines) > 0 {
			for _, line := range existingLines {
				if line.ReceivableID.Valid {
					return ErrSalesScheduleHasReceivables
				}
			}
			return ErrSalesScheduleExists
		}

		scheduleLines, err = createScheduleLinesFromTemplate(ctx, q, id, contract, req)
		if err != nil {
			return err
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		if isScheduleGenerationBadRequest(err) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
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
			return ErrSalesAddLineNotDraft
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
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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
	var approvalRequestID pgtype.UUID
	var action string

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

		switch contract.Status {
		case "draft":
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
			if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
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
			}); err != nil {
				return err
			}
			action = "applied"
			return nil

		case "active":
			if existing.ReceivableID.Valid {
				receivable, err := q.GetReceivable(ctx, existing.ReceivableID)
				if err != nil {
					return err
				}
				if receivable.Status == "paid" || receivable.Status == "partially_paid" {
					return ErrSalesLineReceivablePaid
				}
			}

			if existing.DueDate.Valid && !existing.DueDate.Time.After(time.Now()) {
				return ErrSalesLinePastDue
			}

			payload, _ := json.Marshal(req)
			created, err := q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
				BusinessEntityID:    contract.BusinessEntityID,
				BranchID:            contract.BranchID,
				Module:              "sales",
				RequestType:         "schedule_restructure",
				SourceRecordType:    "installment_schedule_line",
				SourceRecordID:      toPgUUID(id),
				RequestedByUserID:   toPgUUID(userID),
				Status:              "pending",
				PayloadSnapshotJson: payload,
			})
			if err != nil {
				return err
			}
			approvalRequestID = created.ID

			beforeSnapshot, _ := json.Marshal(existing)
			if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
				EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UserID:                   toPgUUID(userID),
				Module:                   "sales",
				ActionType:               "request_schedule_restructure",
				EntityType:               "installment_schedule_line",
				EntityID:                 toPgUUID(id),
				ScopeType:                "project",
				ScopeID:                  contract.ProjectID,
				ResultStatus:             "success",
				SummaryText:              "Requested approval for schedule line restructure",
				BeforeSnapshotJson:       beforeSnapshot,
				RelatedApprovalRequestID: created.ID,
			}); err != nil {
				return err
			}
			action = "approval_requested"
			line = existing
			return nil

		default:
			return ErrSalesUpdateLineBadStatus
		}
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "schedule line not found"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to update schedule line"})
	}

	if action == "approval_requested" {
		return c.Status(202).JSON(fiber.Map{
			"message":             "schedule restructure request submitted for approval",
			"schedule_line_id":    id,
			"approval_request_id": formatApprovalRequestUUID(approvalRequestID),
		})
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
