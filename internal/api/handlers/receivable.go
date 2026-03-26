package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

func ListReceivables(c *fiber.Ctx) error {
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

	var partyID pgtype.UUID
	if c.Query("party_id") != "" {
		id, err := uuid.Parse(c.Query("party_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid party_id"})
		}
		partyID = toPgUUID(id)
	}

	var unitID pgtype.UUID
	if c.Query("unit_id") != "" {
		id, err := uuid.Parse(c.Query("unit_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid unit_id"})
		}
		unitID = toPgUUID(id)
	}

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var sourceModule pgtype.Text
	if c.Query("source_module") != "" {
		sourceModule = pgtype.Text{String: c.Query("source_module"), Valid: true}
	}

	var total int64
	var items []db.Receivable

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountReceivables(ctx, db.CountReceivablesParams{
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			UnitID:           unitID,
			Status:           status,
			SourceModule:     sourceModule,
		})
		if err != nil {
			return fmt.Errorf("count receivables: %w", err)
		}

		items, err = q.ListReceivables(ctx, db.ListReceivablesParams{
			Limit:            int32(perPage),
			Offset:           int32(offset),
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			UnitID:           unitID,
			Status:           status,
			SourceModule:     sourceModule,
		})
		if err != nil {
			return fmt.Errorf("list receivables: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Receivable{}
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

func GetReceivable(c *fiber.Ctx) error {
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

	var receivable db.Receivable

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		receivable, err = q.GetReceivable(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "receivable not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(receivable)
}

func CreateReceivable(c *fiber.Ctx) error {
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
		PartyID          string  `json:"party_id" validate:"required"`
		UnitID           *string `json:"unit_id"`
		ReceivableDate   string  `json:"receivable_date" validate:"required"`
		DueDate          string  `json:"due_date" validate:"required"`
		OriginalAmount   string  `json:"original_amount" validate:"required"`
		Notes            *string `json:"notes"`
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

	partyID, err := uuid.Parse(req.PartyID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid party_id"})
	}

	var unitID pgtype.UUID
	if req.UnitID != nil {
		id, err := uuid.Parse(*req.UnitID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid unit_id"})
		}
		unitID = toPgUUID(id)
	}

	receivableDate, err := time.Parse("2006-01-02", req.ReceivableDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid receivable_date format, use YYYY-MM-DD"})
	}

	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid due_date format, use YYYY-MM-DD"})
	}

	if dueDate.Before(receivableDate) {
		return c.Status(400).JSON(fiber.Map{"error": "due_date must be >= receivable_date"})
	}

	var originalAmount pgtype.Numeric
	if err := originalAmount.Scan(req.OriginalAmount); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid original_amount"})
	}

	amt, _ := originalAmount.Float64Value()
	if !amt.Valid || amt.Float64 <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "original_amount must be positive"})
	}

	sourceRecordID := uuid.New()
	receivableNo := generateReceivableNo()

	var receivable db.Receivable

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		_, err = q.GetBusinessEntity(ctx, toPgUUID(businessEntityID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("business entity not found")
			}
			return fmt.Errorf("verify business entity: %w", err)
		}

		_, err = q.GetBranch(ctx, toPgUUID(branchID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("branch not found")
			}
			return fmt.Errorf("verify branch: %w", err)
		}

		_, err = q.GetParty(ctx, toPgUUID(partyID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("party not found")
			}
			return fmt.Errorf("verify party: %w", err)
		}

		if unitID.Valid {
			_, err = q.GetUnit(ctx, unitID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("unit not found")
				}
				return fmt.Errorf("verify unit: %w", err)
			}
		}

		receivable, err = q.CreateReceivable(ctx, db.CreateReceivableParams{
			BusinessEntityID: toPgUUID(businessEntityID),
			BranchID:         toPgUUID(branchID),
			PartyID:          toPgUUID(partyID),
			UnitID:           unitID,
			SourceModule:     "manual",
			SourceRecordType: "manual",
			SourceRecordID:   toPgUUID(sourceRecordID),
			ReceivableNo:     pgtype.Text{String: receivableNo, Valid: true},
			ReceivableDate:   pgtype.Date{Time: receivableDate, Valid: true},
			DueDate:          pgtype.Date{Time: dueDate, Valid: true},
			CurrencyCode:     "USD",
			OriginalAmount:   originalAmount,
			Notes:            toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		errStr := err.Error()
		if errStr == "business entity not found" || errStr == "branch not found" || errStr == "party not found" || errStr == "unit not found" {
			return c.Status(400).JSON(fiber.Map{"error": errStr})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "receivable_no already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(receivable)
}

func GetPartyStatement(c *fiber.Ctx) error {
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

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var items []db.Receivable

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.GetPartyStatement(ctx, db.GetPartyStatementParams{
			PartyID: toPgUUID(id),
			Status:  status,
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Receivable{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func GetUnitStatement(c *fiber.Ctx) error {
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

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var items []db.Receivable

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.GetUnitStatement(ctx, db.GetUnitStatementParams{
			UnitID: toPgUUID(id),
			Status: status,
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Receivable{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func generateReceivableNo() string {
	timestamp := time.Now().Unix()
	b := make([]byte, 3)
	rand.Read(b)
	randomSuffix := hex.EncodeToString(b)
	return fmt.Sprintf("RCV-%d-%s", timestamp, randomSuffix)
}
