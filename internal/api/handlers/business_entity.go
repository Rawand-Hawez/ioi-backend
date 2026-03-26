// backend/internal/api/handlers/business_entity.go

package handlers

import (
	"context"
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

const dbTimeout = 5 * time.Second

// ListBusinessEntities handles GET /api/v1/business-entities
func ListBusinessEntities(c *fiber.Ctx) error {
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

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var total int64
	var items []db.BusinessEntity

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountBusinessEntities(ctx, isActive)
		if err != nil {
			return fmt.Errorf("count business entities: %w", err)
		}

		items, err = q.ListBusinessEntities(ctx, db.ListBusinessEntitiesParams{
			IsActive: isActive,
			Limit:    int32(perPage),
			Offset:   int32(offset),
		})
		if err != nil {
			return fmt.Errorf("list business entities: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.BusinessEntity{}
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

// GetBusinessEntity handles GET /api/v1/business-entities/:id
func GetBusinessEntity(c *fiber.Ctx) error {
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

	var entity db.BusinessEntity

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		entity, err = q.GetBusinessEntity(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "business entity not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

// CreateBusinessEntity handles POST /api/v1/business-entities
func CreateBusinessEntity(c *fiber.Ctx) error {
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
		Code            string  `json:"code" validate:"required,min=2,max=20"`
		Name            string  `json:"name" validate:"required,min=2,max=100"`
		DisplayName     *string `json:"display_name" validate:"omitempty,max=100"`
		DefaultCurrency string  `json:"default_currency" validate:"omitempty,len=3"`
		Country         *string `json:"country" validate:"omitempty,len=2"`
		RegistrationNo  *string `json:"registration_no" validate:"omitempty,max=50"`
		TaxNo           *string `json:"tax_no" validate:"omitempty,max=50"`
		Status          string  `json:"status" validate:"omitempty,oneof=active inactive"`
		Notes           *string `json:"notes" validate:"omitempty,max=1000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.DefaultCurrency == "" {
		req.DefaultCurrency = "USD"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	var entity db.BusinessEntity

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		entity, err = q.CreateBusinessEntity(ctx, db.CreateBusinessEntityParams{
			Code:            req.Code,
			Name:            req.Name,
			DisplayName:     toPgText(req.DisplayName),
			DefaultCurrency: req.DefaultCurrency,
			Country:         toPgText(req.Country),
			RegistrationNo:  toPgText(req.RegistrationNo),
			TaxNo:           toPgText(req.TaxNo),
			Status:          req.Status,
			Notes:           toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(entity)
}

// UpdateBusinessEntity handles PATCH /api/v1/business-entities/:id
func UpdateBusinessEntity(c *fiber.Ctx) error {
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
		Code            *string `json:"code" validate:"omitempty,min=2,max=20"`
		Name            *string `json:"name" validate:"omitempty,min=2,max=100"`
		DisplayName     *string `json:"display_name" validate:"omitempty,max=100"`
		DefaultCurrency *string `json:"default_currency" validate:"omitempty,len=3"`
		Country         *string `json:"country" validate:"omitempty,len=2"`
		RegistrationNo  *string `json:"registration_no" validate:"omitempty,max=50"`
		TaxNo           *string `json:"tax_no" validate:"omitempty,max=50"`
		Status          *string `json:"status" validate:"omitempty,oneof=active inactive"`
		IsActive        *bool   `json:"is_active"`
		Notes           *string `json:"notes" validate:"omitempty,max=1000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var entity db.BusinessEntity

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		entity, err = q.UpdateBusinessEntity(ctx, db.UpdateBusinessEntityParams{
			ID:              toPgUUID(id),
			Code:            toPgText(req.Code),
			Name:            toPgText(req.Name),
			DisplayName:     toPgText(req.DisplayName),
			DefaultCurrency: toPgText(req.DefaultCurrency),
			Country:         toPgText(req.Country),
			RegistrationNo:  toPgText(req.RegistrationNo),
			TaxNo:           toPgText(req.TaxNo),
			Status:          toPgText(req.Status),
			IsActive:        toPgBool(req.IsActive),
			Notes:           toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "business entity not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func toPgText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toPgBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func isDuplicateKeyError(err error) bool {
	return err != nil && (pgErrCode(err) == "23505")
}

func pgErrCode(err error) string {
	if pgErr, ok := err.(interface{ SQLState() string }); ok {
		return pgErr.SQLState()
	}
	return ""
}
