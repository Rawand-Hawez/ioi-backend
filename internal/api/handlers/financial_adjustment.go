package handlers

import (
	"context"
	"errors"
	"log"
	"math"
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

func ListFinancialAdjustments(c *fiber.Ctx) error {
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
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	var businessEntityID pgtype.UUID
	if c.Query("business_entity_id") != "" {
		id, err := uuid.Parse(c.Query("business_entity_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
		}
		businessEntityID = toPgUUID(id)
	}

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var total int64
	var items []db.FinancialAdjustment

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountFinancialAdjustments(ctx, db.CountFinancialAdjustmentsParams{
			BusinessEntityID: businessEntityID,
			Status:           status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListFinancialAdjustments(ctx, db.ListFinancialAdjustmentsParams{
			Limit:            int32(perPage),
			Offset:           int32(offset),
			BusinessEntityID: businessEntityID,
			Status:           status,
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.FinancialAdjustment{}
	}

	return c.JSON(fiber.Map{
		"data": items,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total":       total,
			"total_pages": int(math.Ceil(float64(total) / float64(perPage))),
		},
	})
}

func GetFinancialAdjustment(c *fiber.Ctx) error {
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

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var adjustment db.FinancialAdjustment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		adjustment, err = q.GetFinancialAdjustment(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "adjustment not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(adjustment)
}

func CreateFinancialAdjustment(c *fiber.Ctx) error {
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

	var req struct {
		BusinessEntityID string  `json:"business_entity_id" validate:"required"`
		BranchID         string  `json:"branch_id" validate:"required"`
		PartyID          *string `json:"party_id"`
		SourceModule     string  `json:"source_module" validate:"required"`
		SourceRecordType string  `json:"source_record_type" validate:"required"`
		SourceRecordID   string  `json:"source_record_id" validate:"required"`
		AdjustmentType   string  `json:"adjustment_type" validate:"required,oneof=debit credit waiver penalty correction"`
		Amount           string  `json:"amount" validate:"required"`
		EffectiveDate    string  `json:"effective_date" validate:"required"`
		Reason           string  `json:"reason" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	validAdjustmentTypes := map[string]bool{
		"debit": true, "credit": true, "waiver": true, "penalty": true, "correction": true,
	}
	validSourceModules := map[string]bool{
		"sales": true, "rentals": true, "service_charges": true, "utilities": true, "manual": true,
	}

	if !validAdjustmentTypes[req.AdjustmentType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid adjustment_type, must be one of: debit, credit, waiver, penalty, correction"})
	}
	if !validSourceModules[req.SourceModule] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid source_module, must be one of: sales, rentals, service_charges, utilities, manual"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	branchID, err := uuid.Parse(req.BranchID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid branch_id"})
	}

	var partyID pgtype.UUID
	if req.PartyID != nil {
		pid, err := uuid.Parse(*req.PartyID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid party_id"})
		}
		partyID = toPgUUID(pid)
	}

	sourceRecordID, err := uuid.Parse(req.SourceRecordID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid source_record_id"})
	}

	var amount pgtype.Numeric
	if err := amount.Scan(req.Amount); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}
	amtFloat, _ := amount.Float64Value()
	if !amtFloat.Valid || amtFloat.Float64 <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "amount must be positive"})
	}

	effectiveDate, err := time.Parse("2006-01-02", req.EffectiveDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid effective_date"})
	}

	requestedByUserIDStr, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID in token"})
	}
	requestedByUserID, err := uuid.Parse(requestedByUserIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	var adjustment db.FinancialAdjustment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		adjustment, err = q.CreateFinancialAdjustment(ctx, db.CreateFinancialAdjustmentParams{
			BusinessEntityID:  toPgUUID(businessEntityID),
			BranchID:          toPgUUID(branchID),
			PartyID:           partyID,
			SourceModule:      req.SourceModule,
			SourceRecordType:  req.SourceRecordType,
			SourceRecordID:    toPgUUID(sourceRecordID),
			AdjustmentType:    req.AdjustmentType,
			Amount:            amount,
			CurrencyCode:      "USD",
			EffectiveDate:     pgtype.Date{Time: effectiveDate, Valid: true},
			Reason:            req.Reason,
			RequestedByUserID: toPgUUID(requestedByUserID),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(adjustment)
}

func ApproveFinancialAdjustment(c *fiber.Ctx) error {
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

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	approvedByUserIDStr, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID in token"})
	}
	approvedByUserID, err := uuid.Parse(approvedByUserIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	var adjustment db.FinancialAdjustment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		adjustment, err = q.ApproveAdjustment(ctx, db.ApproveAdjustmentParams{
			ID:               toPgUUID(id),
			ApprovedByUserID: toPgUUID(approvedByUserID),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "adjustment not found or not pending"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(adjustment)
}

func RejectFinancialAdjustment(c *fiber.Ctx) error {
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

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	approvedByUserIDStr, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID in token"})
	}
	approvedByUserID, err := uuid.Parse(approvedByUserIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid user ID format"})
	}

	var adjustment db.FinancialAdjustment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		adjustment, err = q.RejectAdjustment(ctx, db.RejectAdjustmentParams{
			ID:               toPgUUID(id),
			ApprovedByUserID: toPgUUID(approvedByUserID),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "adjustment not found or not pending"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(adjustment)
}
