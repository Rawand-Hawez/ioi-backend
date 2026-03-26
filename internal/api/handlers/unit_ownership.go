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

func ListUnitOwnerships(c *fiber.Ctx) error {
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

	unitID, err := uuid.Parse(c.Params("unit_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid unit_id"})
	}

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

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var total int64
	var items []db.UnitOwnership

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountUnitOwnerships(ctx, db.CountUnitOwnershipsParams{
			UnitID: toPgUUID(unitID),
			Status: status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListUnitOwnerships(ctx, db.ListUnitOwnershipsParams{
			UnitID: toPgUUID(unitID),
			Status: status,
			Limit:  int32(perPage),
			Offset: int32(offset),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.UnitOwnership{}
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

func CreateUnitOwnership(c *fiber.Ctx) error {
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

	unitID, err := uuid.Parse(c.Params("unit_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid unit_id"})
	}

	var req struct {
		PartyID         string  `json:"party_id" validate:"required"`
		SharePercentage *string `json:"share_percentage" validate:"required"`
		EffectiveFrom   *string `json:"effective_from" validate:"required"`
		EffectiveTo     *string `json:"effective_to"`
		Status          string  `json:"status"`
		Notes           *string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.EffectiveFrom == nil {
		return c.Status(400).JSON(fiber.Map{"error": "effective_from is required"})
	}
	if req.SharePercentage == nil || *req.SharePercentage == "" {
		defaultPct := "100.00"
		req.SharePercentage = &defaultPct
	}

	if req.Status == "" {
		req.Status = "active"
	}

	partyID, err := uuid.Parse(req.PartyID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid party_id"})
	}

	var entity db.UnitOwnership

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.CreateUnitOwnership(ctx, db.CreateUnitOwnershipParams{
			UnitID:          toPgUUID(unitID),
			PartyID:         toPgUUID(partyID),
			SharePercentage: toPgNumeric(req.SharePercentage),
			EffectiveFrom:   parseDate(*req.EffectiveFrom),
			EffectiveTo:     parseDatePtr(req.EffectiveTo),
			Status:          req.Status,
			Notes:           toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(entity)
}

func CloseUnitOwnership(c *fiber.Ctx) error {
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
		EffectiveTo string `json:"effective_to" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var closedEntity db.UnitOwnership
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		closedEntity, err = q.CloseUnitOwnership(ctx, db.CloseUnitOwnershipParams{
			ID:          toPgUUID(id),
			EffectiveTo: parseDate(req.EffectiveTo),
		})
		return err
	})
	_ = closedEntity

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "unit ownership not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.SendStatus(204)
}

func parseDatePtr(s *string) pgtype.Date {
	if s == nil {
		return pgtype.Date{Valid: false}
	}
	return parseDate(*s)
}
