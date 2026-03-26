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

func ListProjects(c *fiber.Ctx) error {
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

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var total int64
	var items []db.Project

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountProjects(ctx, db.CountProjectsParams{
			BusinessEntityID: businessEntityID,
			IsActive:         isActive,
			Status:           status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListProjects(ctx, db.ListProjectsParams{
			BusinessEntityID: businessEntityID,
			IsActive:         isActive,
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
		items = []db.Project{}
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

func GetProject(c *fiber.Ctx) error {
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

	var entity db.Project

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.GetProject(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "project not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func CreateProject(c *fiber.Ctx) error {
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
		PrimaryBranchID  string  `json:"primary_branch_id" validate:"required"`
		Code             string  `json:"code" validate:"required,min=2,max=20"`
		Name             string  `json:"name" validate:"required,min=2,max=100"`
		DisplayName      *string `json:"display_name" validate:"omitempty,max=100"`
		ProjectType      string  `json:"project_type" validate:"required,oneof=residential commercial mixed managed"`
		StructureType    string  `json:"structure_type" validate:"required,oneof=tower compound villa_community mixed flat"`
		Status           string  `json:"status" validate:"omitempty,oneof=active inactive"`
		Country          string  `json:"country" validate:"required,len=2"`
		City             *string `json:"city" validate:"omitempty,max=100"`
		DistrictArea     *string `json:"district_area" validate:"omitempty,max=100"`
		AddressText      *string `json:"address_text" validate:"omitempty,max=500"`
		DefaultCurrency  string  `json:"default_currency" validate:"omitempty,len=3"`
		LaunchDate       *string `json:"launch_date" validate:"omitempty,datetime=2006-01-02"`
		HandoverDate     *string `json:"handover_date" validate:"omitempty,datetime=2006-01-02"`
		Notes            *string `json:"notes" validate:"omitempty,max=2000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Status == "" {
		req.Status = "active"
	}
	if req.DefaultCurrency == "" {
		req.DefaultCurrency = "USD"
	}

	beID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	primaryBranchID, err := uuid.Parse(req.PrimaryBranchID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid primary_branch_id"})
	}

	var launchDate pgtype.Date
	if req.LaunchDate != nil {
		launchDate = parseDate(*req.LaunchDate)
	}

	var handoverDate pgtype.Date
	if req.HandoverDate != nil {
		handoverDate = parseDate(*req.HandoverDate)
	}

	var entity db.Project

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.CreateProject(ctx, db.CreateProjectParams{
			BusinessEntityID: toPgUUID(beID),
			PrimaryBranchID:  toPgUUID(primaryBranchID),
			Code:             req.Code,
			Name:             req.Name,
			DisplayName:      toPgText(req.DisplayName),
			ProjectType:      req.ProjectType,
			StructureType:    req.StructureType,
			Status:           req.Status,
			Country:          req.Country,
			City:             toPgText(req.City),
			DistrictArea:     toPgText(req.DistrictArea),
			AddressText:      toPgText(req.AddressText),
			DefaultCurrency:  req.DefaultCurrency,
			LaunchDate:       launchDate,
			HandoverDate:     handoverDate,
			Notes:            toPgText(req.Notes),
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

func UpdateProject(c *fiber.Ctx) error {
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
		PrimaryBranchID *string `json:"primary_branch_id"`
		Code            *string `json:"code" validate:"omitempty,min=2,max=20"`
		Name            *string `json:"name" validate:"omitempty,min=2,max=100"`
		DisplayName     *string `json:"display_name" validate:"omitempty,max=100"`
		ProjectType     *string `json:"project_type" validate:"omitempty,oneof=residential commercial mixed managed"`
		StructureType   *string `json:"structure_type" validate:"omitempty,oneof=tower compound villa_community mixed flat"`
		Status          *string `json:"status" validate:"omitempty,oneof=active inactive"`
		Country         *string `json:"country" validate:"omitempty,len=2"`
		City            *string `json:"city" validate:"omitempty,max=100"`
		DistrictArea    *string `json:"district_area" validate:"omitempty,max=100"`
		AddressText     *string `json:"address_text" validate:"omitempty,max=500"`
		DefaultCurrency *string `json:"default_currency" validate:"omitempty,len=3"`
		LaunchDate      *string `json:"launch_date" validate:"omitempty,datetime=2006-01-02"`
		HandoverDate    *string `json:"handover_date" validate:"omitempty,datetime=2006-01-02"`
		Notes           *string `json:"notes" validate:"omitempty,max=2000"`
		IsActive        *bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var primaryBranchID pgtype.UUID
	if req.PrimaryBranchID != nil {
		id, err := uuid.Parse(*req.PrimaryBranchID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid primary_branch_id"})
		}
		primaryBranchID = toPgUUID(id)
	}

	var launchDate pgtype.Date
	if req.LaunchDate != nil {
		launchDate = parseDate(*req.LaunchDate)
	}

	var handoverDate pgtype.Date
	if req.HandoverDate != nil {
		handoverDate = parseDate(*req.HandoverDate)
	}

	var entity db.Project

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.UpdateProject(ctx, db.UpdateProjectParams{
			ID:              toPgUUID(id),
			PrimaryBranchID: primaryBranchID,
			Code:            toPgText(req.Code),
			Name:            toPgText(req.Name),
			DisplayName:     toPgText(req.DisplayName),
			ProjectType:     toPgText(req.ProjectType),
			StructureType:   toPgText(req.StructureType),
			Status:          toPgText(req.Status),
			Country:         toPgText(req.Country),
			City:            toPgText(req.City),
			DistrictArea:    toPgText(req.DistrictArea),
			AddressText:     toPgText(req.AddressText),
			DefaultCurrency: toPgText(req.DefaultCurrency),
			LaunchDate:      launchDate,
			HandoverDate:    handoverDate,
			Notes:           toPgText(req.Notes),
			IsActive:        toPgBool(req.IsActive),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "project not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func DeactivateProject(c *fiber.Ctx) error {
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

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		return q.DeactivateProject(ctx, toPgUUID(id))
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.SendStatus(204)
}

func parseDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}
