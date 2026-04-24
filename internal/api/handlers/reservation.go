package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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

var (
	errReservationDepositPaymentInvalid    = errors.New("invalid deposit_payment_id")
	errReservationDepositPaymentNotPosted  = errors.New("deposit payment must be posted")
	errReservationDepositPaymentPartyScope = errors.New("deposit payment does not match reservation customer")
	errReservationDepositPaymentScope      = errors.New("deposit payment does not match reservation scope")
	errReservationDepositPaymentAmount     = errors.New("deposit payment unapplied amount is less than deposit amount")
	errReservationDepositAmountInvalid     = errors.New("invalid deposit_amount")
)

func ListReservations(c *fiber.Ctx) error {
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

	listParams := db.ListReservationsParams{
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
	if customerID := c.Query("customer_id"); customerID != "" {
		if id, err := uuid.Parse(customerID); err == nil {
			listParams.CustomerID = toPgUUID(id)
		}
	}
	if status := c.Query("status"); status != "" {
		listParams.Status = toPgText(&status)
	}

	var reservations []db.Reservation
	var totalCount int64

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		reservations, err = q.ListReservations(ctx, listParams)
		if err != nil {
			return err
		}

		countParams := db.CountReservationsParams{
			BusinessEntityID: listParams.BusinessEntityID,
			BranchID:         listParams.BranchID,
			ProjectID:        listParams.ProjectID,
			UnitID:           listParams.UnitID,
			CustomerID:       listParams.CustomerID,
			Status:           listParams.Status,
		}
		totalCount, err = q.CountReservations(ctx, countParams)
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to list reservations"})
	}

	totalPages := int64(0)
	if perPage > 0 {
		totalPages = (totalCount + int64(perPage) - 1) / int64(perPage)
	}

	response := fiber.Map{
		"data": reservations,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total_count": totalCount,
			"total_pages": totalPages,
		},
	}

	return c.JSON(response)
}

func GetReservation(c *fiber.Ctx) error {
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
		return c.Status(400).JSON(fiber.Map{"error": "invalid reservation id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var reservation db.Reservation

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		reservation, err = q.GetReservation(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "reservation not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to get reservation"})
	}

	return c.JSON(reservation)
}

func CreateReservation(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	var req struct {
		BusinessEntityID  string  `json:"business_entity_id"`
		BranchID          string  `json:"branch_id"`
		ProjectID         string  `json:"project_id"`
		UnitID            string  `json:"unit_id"`
		CustomerID        string  `json:"customer_id"`
		ExpiresAt         string  `json:"expires_at"`
		DepositAmount     *string `json:"deposit_amount,omitempty"`
		DepositCurrency   *string `json:"deposit_currency,omitempty"`
		DepositPaymentID  *string `json:"deposit_payment_id,omitempty"`
		QuotedPriceAmount *string `json:"quoted_price_amount,omitempty"`
		DiscountAmount    *string `json:"discount_amount,omitempty"`
		Notes             *string `json:"notes,omitempty"`
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
	customerID, err := uuid.Parse(req.CustomerID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid customer_id"})
	}

	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid expires_at format, use RFC3339"})
	}
	if expiresAt.Before(time.Now()) {
		return c.Status(400).JSON(fiber.Map{"error": "expires_at must be in the future"})
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

	var reservation db.Reservation

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		if _, err := q.ExpireReservations(ctx); err != nil {
			return err
		}

		activeReservation, err := q.GetActiveReservationForUnit(ctx, toPgUUID(unitID))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if activeReservation.ID.Valid {
			return errors.New("unit already has an active reservation")
		}

		if err := validateReservationDepositPayment(ctx, q, req.DepositPaymentID, req.DepositAmount, businessEntityID, branchID, customerID); err != nil {
			return err
		}

		createParams := db.CreateReservationParams{
			BusinessEntityID:  toPgUUID(businessEntityID),
			BranchID:          toPgUUID(branchID),
			ProjectID:         toPgUUID(projectID),
			UnitID:            toPgUUID(unitID),
			CustomerID:        toPgUUID(customerID),
			ExpiresAt:         pgtype.Timestamptz{Time: expiresAt, Valid: true},
			DepositAmount:     toPgNumeric(req.DepositAmount),
			DepositCurrency:   ptrToString(req.DepositCurrency),
			DepositPaymentID:  toPgUUIDFromString(req.DepositPaymentID),
			QuotedPriceAmount: toPgNumeric(req.QuotedPriceAmount),
			DiscountAmount:    toPgNumeric(req.DiscountAmount),
			Notes:             toPgText(req.Notes),
			CreatedByUserID:   toPgUUID(userID),
		}

		reservation, err = q.CreateReservation(ctx, createParams)
		if err != nil {
			return err
		}

		updateUnitParams := db.UpdateUnitParams{
			ID:          toPgUUID(unitID),
			SalesStatus: toPgText(strPtr("reserved")),
		}
		_, err = q.UpdateUnit(ctx, updateUnitParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(reservation)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "create_reservation",
			EntityType:        "reservation",
			EntityID:          toPgUUID(reservation.ID.Bytes),
			ScopeType:         "project",
			ScopeID:           toPgUUID(projectID),
			ResultStatus:      "success",
			SummaryText:       "Created reservation for unit",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if err.Error() == "unit already has an active reservation" {
			return c.Status(409).JSON(fiber.Map{"error": "unit already has an active reservation"})
		}
		if isReservationDepositValidationError(err) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "reservation already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to create reservation"})
	}

	return c.Status(201).JSON(reservation)
}

func ConvertReservation(c *fiber.Ctx) error {
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
		return c.Status(400).JSON(fiber.Map{"error": "invalid reservation id"})
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

		reservation, err := q.GetReservation(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if reservation.Status != "active" {
			return errors.New("reservation is not active")
		}

		contractParams := db.CreateSalesContractParams{
			BusinessEntityID:    reservation.BusinessEntityID,
			BranchID:            reservation.BranchID,
			ProjectID:           reservation.ProjectID,
			UnitID:              reservation.UnitID,
			PrimaryBuyerID:      reservation.CustomerID,
			SourceReservationID: toPgUUID(id),
			ContractDate:        pgtype.Date{Time: time.Now(), Valid: true},
			EffectiveDate:       pgtype.Date{Time: time.Now(), Valid: true},
			SalePriceAmount:     reservation.QuotedPriceAmount,
			DiscountAmount:      reservation.DiscountAmount,
			SalePriceCurrency:   reservation.DepositCurrency,
			CreatedByUserID:     toPgUUID(userID),
		}

		contract, err = q.CreateSalesContract(ctx, contractParams)
		if err != nil {
			return err
		}

		partyParams := db.CreateSalesContractPartyParams{
			SalesContractID: toPgUUID(contract.ID.Bytes),
			PartyID:         reservation.CustomerID,
			Role:            "primary_buyer",
			IsPrimary:       true,
			EffectiveFrom:   pgtype.Date{Time: time.Now(), Valid: true},
			Status:          "active",
		}
		_, err = q.CreateSalesContractParty(ctx, partyParams)
		if err != nil {
			return err
		}

		updateResParams := db.UpdateReservationStatusParams{
			ID:       toPgUUID(id),
			Status:   "converted",
			Status_2: "active",
		}
		_, err = q.UpdateReservationStatus(ctx, updateResParams)
		if err != nil {
			return err
		}

		afterSnapshot, _ := json.Marshal(contract)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "convert_reservation",
			EntityType:        "sales_contract",
			EntityID:          toPgUUID(contract.ID.Bytes),
			ScopeType:         "project",
			ScopeID:           reservation.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Converted reservation to sales contract",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "reservation not found"})
		}
		if err.Error() == "reservation is not active" {
			return c.Status(409).JSON(fiber.Map{"error": "reservation is not active"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to convert reservation"})
	}

	return c.Status(201).JSON(fiber.Map{
		"contract_id": contract.ID.Bytes,
		"contract":    contract,
	})
}

func CancelReservation(c *fiber.Ctx) error {
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
		return c.Status(400).JSON(fiber.Map{"error": "invalid reservation id"})
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

	var reservation db.Reservation

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		reservation, err = q.GetReservation(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if reservation.Status != "active" {
			return errors.New("reservation is not active")
		}

		updateResParams := db.UpdateReservationStatusParams{
			ID:       toPgUUID(id),
			Status:   "cancelled",
			Status_2: "active",
		}
		_, err = q.UpdateReservationStatus(ctx, updateResParams)
		if err != nil {
			return err
		}

		_, err = q.GetActiveSalesContractForUnit(ctx, reservation.UnitID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				_, err = q.GetActiveReservationForUnit(ctx, reservation.UnitID)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if errors.Is(err, pgx.ErrNoRows) {
					updateUnitParams := db.UpdateUnitParams{
						ID:          reservation.UnitID,
						SalesStatus: toPgText(strPtr("available")),
					}
					_, err = q.UpdateUnit(ctx, updateUnitParams)
					if err != nil {
						return err
					}
				}
			} else {
				return err
			}
		}

		afterSnapshot, _ := json.Marshal(reservation)
		auditParams := db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "sales",
			ActionType:        "cancel_reservation",
			EntityType:        "reservation",
			EntityID:          toPgUUID(reservation.ID.Bytes),
			ScopeType:         "project",
			ScopeID:           reservation.ProjectID,
			ResultStatus:      "success",
			SummaryText:       "Cancelled reservation",
			AfterSnapshotJson: afterSnapshot,
		}
		_, err = q.InsertAuditLog(ctx, auditParams)
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "reservation not found"})
		}
		if err.Error() == "reservation is not active" {
			return c.Status(409).JSON(fiber.Map{"error": "reservation is not active"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to cancel reservation"})
	}

	return c.JSON(fiber.Map{
		"message":     "reservation cancelled",
		"reservation": reservation,
	})
}

func toPgUUIDFromString(s *string) pgtype.UUID {
	if s == nil || *s == "" {
		return pgtype.UUID{Valid: false}
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return pgtype.UUID{Valid: false}
	}
	return toPgUUID(id)
}

func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strPtr(s string) *string {
	return &s
}

func validateReservationDepositPayment(
	ctx context.Context,
	q *db.Queries,
	paymentID *string,
	depositAmount *string,
	businessEntityID uuid.UUID,
	branchID uuid.UUID,
	customerID uuid.UUID,
) error {
	if paymentID == nil || *paymentID == "" {
		return nil
	}

	parsedPaymentID, err := uuid.Parse(*paymentID)
	if err != nil {
		return errReservationDepositPaymentInvalid
	}

	payment, err := q.GetPayment(ctx, toPgUUID(parsedPaymentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errReservationDepositPaymentInvalid
		}
		return err
	}

	if payment.Status != "posted" {
		return errReservationDepositPaymentNotPosted
	}
	if payment.PartyID.Bytes != customerID {
		return errReservationDepositPaymentPartyScope
	}
	if payment.BusinessEntityID.Bytes != businessEntityID || payment.BranchID.Bytes != branchID {
		return errReservationDepositPaymentScope
	}
	if depositAmount != nil {
		requiredAmount := toPgNumeric(depositAmount)
		if !requiredAmount.Valid {
			return errReservationDepositAmountInvalid
		}
		if numericToRat(&payment.UnappliedAmount).Cmp(numericToRat(&requiredAmount)) < 0 {
			return errReservationDepositPaymentAmount
		}
	}

	return nil
}

func isReservationDepositValidationError(err error) bool {
	return errors.Is(err, errReservationDepositPaymentInvalid) ||
		errors.Is(err, errReservationDepositPaymentNotPosted) ||
		errors.Is(err, errReservationDepositPaymentPartyScope) ||
		errors.Is(err, errReservationDepositPaymentScope) ||
		errors.Is(err, errReservationDepositPaymentAmount) ||
		errors.Is(err, errReservationDepositAmountInvalid)
}
