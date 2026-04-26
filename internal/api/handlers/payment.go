// backend/internal/api/handlers/payment.go

package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
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

var (
	ErrPaymentNotPosted        = errors.New("payment must be in posted status")
	ErrAllocationExceedsAmount = errors.New("total allocation exceeds unapplied amount")
)

func ListPayments(c *fiber.Ctx) error {
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
	var items []db.Payment

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountPayments(ctx, db.CountPaymentsParams{
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			Status:           status,
		})
		if err != nil {
			return fmt.Errorf("count payments: %w", err)
		}

		items, err = q.ListPayments(ctx, db.ListPaymentsParams{
			BusinessEntityID: businessEntityID,
			PartyID:          partyID,
			Status:           status,
			Limit:            int32(perPage),
			Offset:           int32(offset),
		})
		if err != nil {
			return fmt.Errorf("list payments: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Payment{}
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

func GetPayment(c *fiber.Ctx) error {
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

	var payment db.Payment
	var allocations []db.PaymentAllocation

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		payment, err = q.GetPayment(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		allocations, err = q.ListAllocationsForPayment(ctx, toPgUUID(id))
		if err != nil {
			return fmt.Errorf("list allocations: %w", err)
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "payment not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if allocations == nil {
		allocations = []db.PaymentAllocation{}
	}

	return c.JSON(fiber.Map{
		"payment":     payment,
		"allocations": allocations,
	})
}

func CreatePayment(c *fiber.Ctx) error {
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
		PaymentDate      string  `json:"payment_date" validate:"required"`
		PaymentMethod    string  `json:"payment_method" validate:"required,oneof=cash bank_transfer cheque card credit_balance"`
		AmountReceived   string  `json:"amount_received" validate:"required"`
		ReceiptNo        *string `json:"receipt_no"`
		ReferenceNo      *string `json:"reference_no"`
		Notes            *string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	validPaymentMethods := map[string]bool{
		"cash": true, "bank_transfer": true, "cheque": true, "card": true, "credit_balance": true,
	}

	if !validPaymentMethods[req.PaymentMethod] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payment_method, must be one of: cash, bank_transfer, cheque, card, credit_balance"})
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

	paymentDate, err := time.Parse("2006-01-02", req.PaymentDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payment_date"})
	}

	var amountReceived pgtype.Numeric
	if err := amountReceived.Scan(req.AmountReceived); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount_received"})
	}

	if amountReceived.Int == nil || amountReceived.Int.Sign() <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "amount_received must be positive"})
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	receivedByUserID, err := uuid.Parse(sub)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	paymentNo := fmt.Sprintf("PAY-%d-%s", time.Now().Unix(), generateRandomSuffix(6))

	var payment db.Payment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		payment, err = q.CreatePayment(ctx, db.CreatePaymentParams{
			BusinessEntityID: toPgUUID(businessEntityID),
			BranchID:         toPgUUID(branchID),
			PartyID:          toPgUUID(partyID),
			PaymentNo:        paymentNo,
			ReceiptNo:        toPgText(req.ReceiptNo),
			PaymentDate:      pgtype.Date{Time: paymentDate, Valid: true},
			PaymentMethod:    req.PaymentMethod,
			CurrencyCode:     "USD",
			AmountReceived:   amountReceived,
			ReferenceNo:      toPgText(req.ReferenceNo),
			Notes:            toPgText(req.Notes),
			ReceivedByUserID: toPgUUID(receivedByUserID),
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "payment_no already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(payment)
}

func PostPayment(c *fiber.Ctx) error {
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

	var payment db.Payment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		payment, err = q.PostPayment(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "payment not found or not in draft status"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(payment)
}

func AllocatePayment(c *fiber.Ctx) error {
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

	var req struct {
		Allocations []struct {
			ReceivableID string `json:"receivable_id" validate:"required"`
			Amount       string `json:"amount" validate:"required"`
		} `json:"allocations" validate:"required,min=1"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if len(req.Allocations) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "at least one allocation required"})
	}

	var payment db.Payment
	var createdAllocations []db.PaymentAllocation

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		payment, err = q.GetPayment(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if payment.Status != "posted" {
			return ErrPaymentNotPosted
		}

		allocationAmounts := make([]pgtype.Numeric, len(req.Allocations))
		totalAllocation := big.NewRat(0, 1)

		for i, alloc := range req.Allocations {
			var amount pgtype.Numeric
			if err := amount.Scan(alloc.Amount); err != nil {
				return fmt.Errorf("invalid amount for allocation %d: %w", i, err)
			}
			if amount.Int == nil || amount.Int.Sign() <= 0 {
				return fmt.Errorf("amount must be positive for allocation %d", i)
			}
			allocationAmounts[i] = amount

			amtRat := numericToRat(&amount)
			totalAllocation.Add(totalAllocation, amtRat)
		}

		unappliedRat := numericToRat(&payment.UnappliedAmount)
		if totalAllocation.Cmp(unappliedRat) > 0 {
			return ErrAllocationExceedsAmount
		}

		existingAllocations, err := q.ListAllocationsForPayment(ctx, toPgUUID(id))
		if err != nil {
			return fmt.Errorf("list existing allocations: %w", err)
		}

		maxOrder := int16(0)
		for _, a := range existingAllocations {
			if a.AllocationOrder > maxOrder {
				maxOrder = a.AllocationOrder
			}
		}

		createdAllocations = make([]db.PaymentAllocation, len(req.Allocations))
		today := pgtype.Date{Time: time.Now(), Valid: true}

		for i, alloc := range req.Allocations {
			receivableID, err := uuid.Parse(alloc.ReceivableID)
			if err != nil {
				return fmt.Errorf("invalid receivable_id for allocation %d", i)
			}

			receivable, err := q.GetReceivable(ctx, toPgUUID(receivableID))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("receivable not found for allocation %d", i)
				}
				return fmt.Errorf("get receivable for allocation %d: %w", i, err)
			}

			if receivable.Status != "open" && receivable.Status != "partially_paid" {
				return fmt.Errorf("receivable must be open or partially_paid for allocation %d", i)
			}

			allocRat := numericToRat(&allocationAmounts[i])
			outstandingRat := numericToRat(&receivable.OutstandingAmount)
			if allocRat.Cmp(outstandingRat) > 0 {
				return fmt.Errorf("allocation amount exceeds receivable outstanding amount for allocation %d", i)
			}

			newAllocation, err := q.CreatePaymentAllocation(ctx, db.CreatePaymentAllocationParams{
				PaymentID:       toPgUUID(id),
				ReceivableID:    toPgUUID(receivableID),
				AllocatedAmount: allocationAmounts[i],
				AllocationDate:  today,
				AllocationOrder: maxOrder + int16(i) + 1,
			})
			if err != nil {
				return fmt.Errorf("create allocation %d: %w", i, err)
			}
			createdAllocations[i] = newAllocation

			_, err = q.UpdateReceivablePaidAmount(ctx, db.UpdateReceivablePaidAmountParams{
				ID:         toPgUUID(receivableID),
				PaidAmount: allocationAmounts[i],
			})
			if err != nil {
				return fmt.Errorf("update receivable paid amount for allocation %d: %w", i, err)
			}
		}

		totalAllocNumeric := ratToNumeric(totalAllocation)
		_, err = q.UpdatePaymentUnapplied(ctx, db.UpdatePaymentUnappliedParams{
			ID:              toPgUUID(id),
			UnappliedAmount: totalAllocNumeric,
		})
		if err != nil {
			return fmt.Errorf("update payment unapplied amount: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "payment not found"})
		}
		if errors.Is(err, ErrPaymentNotPosted) || errors.Is(err, ErrAllocationExceedsAmount) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		errStr := err.Error()
		if len(errStr) >= 3 && (errStr[0:3] == "inv" || errStr[0:3] == "amo" || errStr[0:3] == "rec" || errStr[0:3] == "all") {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(fiber.Map{"allocations": createdAllocations})
}

func VoidPayment(c *fiber.Ctx) error {
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

	var payment db.Payment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		payment, err = q.GetPayment(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if payment.Status != "posted" {
			return ErrPaymentNotPosted
		}

		allocations, err := q.ListAllocationsForPayment(ctx, toPgUUID(id))
		if err != nil {
			return fmt.Errorf("list allocations: %w", err)
		}

		for _, alloc := range allocations {
			negAmount := negateNumeric(&alloc.AllocatedAmount)

			_, err = q.UpdateReceivablePaidAmount(ctx, db.UpdateReceivablePaidAmountParams{
				ID:         alloc.ReceivableID,
				PaidAmount: negAmount,
			})
			if err != nil {
				return fmt.Errorf("reverse allocation for receivable %s: %w", alloc.ReceivableID, err)
			}
		}

		payment, err = q.VoidPayment(ctx, toPgUUID(id))
		if err != nil {
			return fmt.Errorf("void payment: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "payment not found"})
		}
		if errors.Is(err, ErrPaymentNotPosted) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(payment)
}

func generateRandomSuffix(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}
