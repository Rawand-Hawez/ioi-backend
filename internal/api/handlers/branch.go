// backend/internal/api/handlers/branch.go

package handlers

import (
	"context"
	"errors"
	"fmt"
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

// ListBranches handles GET /api/v1/business-entities/:id/branches
func ListBranches(c *fiber.Ctx) error {
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

	businessEntityID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business entity id"})
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

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var total int64
	var items []db.Branch

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountBranches(ctx, db.CountBranchesParams{
			BusinessEntityID: toPgUUID(businessEntityID),
			IsActive:         isActive,
		})
		if err != nil {
			return fmt.Errorf("count branches: %w", err)
		}

		items, err = q.ListBranches(ctx, db.ListBranchesParams{
			BusinessEntityID: toPgUUID(businessEntityID),
			IsActive:         isActive,
			Limit:            int32(perPage),
			Offset:           int32(offset),
		})
		if err != nil {
			return fmt.Errorf("list branches: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Branch{}
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

// GetBranch handles GET /api/v1/branches/:id
func GetBranch(c *fiber.Ctx) error {
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

	var branch db.Branch

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		branch, err = q.GetBranch(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "branch not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(branch)
}

// CreateBranch handles POST /api/v1/branches
func CreateBranch(c *fiber.Ctx) error {
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
		BusinessEntityID string  `json:"business_entity_id" validate:"required,uuid"`
		Code             string  `json:"code" validate:"required,min=2,max=20"`
		Name             string  `json:"name" validate:"required,min=2,max=100"`
		DisplayName      *string `json:"display_name" validate:"omitempty,max=100"`
		Country          *string `json:"country" validate:"omitempty,len=2"`
		City             *string `json:"city" validate:"omitempty,max=100"`
		AddressText      *string `json:"address_text" validate:"omitempty,max=500"`
		Phone            *string `json:"phone" validate:"omitempty,max=50"`
		Email            *string `json:"email" validate:"omitempty,email,max=100"`
		Status           string  `json:"status" validate:"omitempty,oneof=active inactive"`
		Notes            *string `json:"notes" validate:"omitempty,max=1000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	var branch db.Branch

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		_, err := q.GetBusinessEntity(ctx, toPgUUID(businessEntityID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("business entity not found")
			}
			return err
		}

		branch, err = q.CreateBranch(ctx, db.CreateBranchParams{
			BusinessEntityID: toPgUUID(businessEntityID),
			Code:             req.Code,
			Name:             req.Name,
			DisplayName:      toPgText(req.DisplayName),
			Country:          toPgText(req.Country),
			City:             toPgText(req.City),
			AddressText:      toPgText(req.AddressText),
			Phone:            toPgText(req.Phone),
			Email:            toPgText(req.Email),
			Status:           req.Status,
			Notes:            toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		if err.Error() == "business entity not found" {
			return c.Status(400).JSON(fiber.Map{"error": "business entity not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists for this business entity"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(branch)
}

// UpdateBranch handles PATCH /api/v1/branches/:id
func UpdateBranch(c *fiber.Ctx) error {
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
		Code        *string `json:"code" validate:"omitempty,min=2,max=20"`
		Name        *string `json:"name" validate:"omitempty,min=2,max=100"`
		DisplayName *string `json:"display_name" validate:"omitempty,max=100"`
		Country     *string `json:"country" validate:"omitempty,len=2"`
		City        *string `json:"city" validate:"omitempty,max=100"`
		AddressText *string `json:"address_text" validate:"omitempty,max=500"`
		Phone       *string `json:"phone" validate:"omitempty,max=50"`
		Email       *string `json:"email" validate:"omitempty,email,max=100"`
		Status      *string `json:"status" validate:"omitempty,oneof=active inactive"`
		IsActive    *bool   `json:"is_active"`
		Notes       *string `json:"notes" validate:"omitempty,max=1000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var branch db.Branch

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		branch, err = q.UpdateBranch(ctx, db.UpdateBranchParams{
			ID:          toPgUUID(id),
			Code:        toPgText(req.Code),
			Name:        toPgText(req.Name),
			DisplayName: toPgText(req.DisplayName),
			Country:     toPgText(req.Country),
			City:        toPgText(req.City),
			AddressText: toPgText(req.AddressText),
			Phone:       toPgText(req.Phone),
			Email:       toPgText(req.Email),
			Status:      toPgText(req.Status),
			IsActive:    toPgBool(req.IsActive),
			Notes:       toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "branch not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists for this business entity"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(branch)
}
