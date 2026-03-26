// backend/internal/api/handlers/credit_balance.go

package handlers

import (
	"context"
	"errors"
	"log"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

func ListCreditBalances(c *fiber.Ctx) error {
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

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var total int64
	var items []db.CreditBalance

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountCreditBalances(ctx, db.CountCreditBalancesParams{
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			Status:           status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListCreditBalances(ctx, db.ListCreditBalancesParams{
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			Status:           status,
			Limit:            int32(perPage),
			Offset:           int32(offset),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.CreditBalance{}
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

func ApplyCreditBalance(c *fiber.Ctx) error {
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

	creditID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid credit balance id"})
	}

	var req struct {
		ReceivableID string `json:"receivable_id" validate:"required"`
		Amount       string `json:"amount" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var amountNumeric pgtype.Numeric
	if err := amountNumeric.Scan(req.Amount); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount format"})
	}

	amountFloat, _ := amountNumeric.Float64Value()
	if !amountFloat.Valid || amountFloat.Float64 <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "amount must be greater than zero"})
	}

	receivableID, err := uuid.Parse(req.ReceivableID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid receivable_id"})
	}

	var updatedCreditBalance db.CreditBalance

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		creditBalance, err := q.GetCreditBalance(ctx, toPgUUID(creditID))
		if err != nil {
			return err
		}

		if creditBalance.Status != "available" {
			return errors.New("credit balance not available")
		}

		remainingRat := numericToRat(&creditBalance.AmountRemaining)
		amountRat := numericToRat(&amountNumeric)
		if amountRat.Cmp(remainingRat) > 0 {
			return errors.New("amount exceeds credit balance remaining")
		}

		receivable, err := q.GetReceivable(ctx, toPgUUID(receivableID))
		if err != nil {
			return err
		}

		if receivable.Status != "open" && receivable.Status != "partially_paid" {
			return errors.New("receivable not open or partially paid")
		}

		outstandingRat := numericToRat(&receivable.OutstandingAmount)
		if amountRat.Cmp(outstandingRat) > 0 {
			return errors.New("amount exceeds receivable outstanding amount")
		}

		updatedCreditBalance, err = q.ApplyCreditBalance(ctx, db.ApplyCreditBalanceParams{
			ID:         toPgUUID(creditID),
			AmountUsed: amountNumeric,
		})
		if err != nil {
			return err
		}

		_, err = q.UpdateReceivableCreditedAmount(ctx, db.UpdateReceivableCreditedAmountParams{
			ID:             toPgUUID(receivableID),
			CreditedAmount: amountNumeric,
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "credit balance or receivable not found"})
		}
		if err.Error() == "credit balance not available" {
			return c.Status(400).JSON(fiber.Map{"error": "credit balance is not available"})
		}
		if err.Error() == "amount exceeds credit balance remaining" {
			return c.Status(400).JSON(fiber.Map{"error": "amount exceeds credit balance remaining"})
		}
		if err.Error() == "receivable not open or partially paid" {
			return c.Status(400).JSON(fiber.Map{"error": "receivable must be open or partially paid"})
		}
		if err.Error() == "amount exceeds receivable outstanding amount" {
			return c.Status(400).JSON(fiber.Map{"error": "amount exceeds receivable outstanding amount"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(updatedCreditBalance)
}
