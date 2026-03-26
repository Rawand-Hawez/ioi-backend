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

func ListStructureNodes(c *fiber.Ctx) error {
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

	projectIDStr := c.Params("id")
	if projectIDStr == "" {
		projectIDStr = c.Query("project_id")
	}
	if projectIDStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "project_id is required"})
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
	}

	var parentStructureNodeID pgtype.UUID
	if c.Query("parent_structure_node_id") != "" {
		id, err := uuid.Parse(c.Query("parent_structure_node_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid parent_structure_node_id"})
		}
		parentStructureNodeID = toPgUUID(id)
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
	var items []db.StructureNode

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountStructureNodes(ctx, db.CountStructureNodesParams{
			ProjectID:             toPgUUID(projectID),
			ParentStructureNodeID: parentStructureNodeID,
			IsActive:              isActive,
			Status:                status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListStructureNodes(ctx, db.ListStructureNodesParams{
			ProjectID:             toPgUUID(projectID),
			ParentStructureNodeID: parentStructureNodeID,
			IsActive:              isActive,
			Status:                status,
			Limit:                 int32(perPage),
			Offset:                int32(offset),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.StructureNode{}
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

func GetStructureNode(c *fiber.Ctx) error {
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

	var entity db.StructureNode

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.GetStructureNode(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "structure node not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func CreateStructureNode(c *fiber.Ctx) error {
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
		ProjectID             string  `json:"project_id" validate:"required"`
		ParentStructureNodeID *string `json:"parent_structure_node_id"`
		NodeType              string  `json:"node_type" validate:"required,oneof=building tower block cluster floor"`
		Code                  string  `json:"code" validate:"required,min=1,max=20"`
		Name                  string  `json:"name" validate:"required,min=1,max=100"`
		DisplayOrder          int32   `json:"display_order"`
		Status                string  `json:"status" validate:"omitempty,oneof=active inactive"`
		Notes                 *string `json:"notes" validate:"omitempty,max=1000"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
	}

	var parentStructureNodeID pgtype.UUID
	if req.ParentStructureNodeID != nil {
		id, err := uuid.Parse(*req.ParentStructureNodeID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid parent_structure_node_id"})
		}
		parentStructureNodeID = toPgUUID(id)
	}

	var entity db.StructureNode

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.CreateStructureNode(ctx, db.CreateStructureNodeParams{
			ProjectID:             toPgUUID(projectID),
			ParentStructureNodeID: parentStructureNodeID,
			NodeType:              req.NodeType,
			Code:                  req.Code,
			Name:                  req.Name,
			DisplayOrder:          req.DisplayOrder,
			Status:                req.Status,
			Notes:                 toPgText(req.Notes),
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists in project"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(entity)
}

func UpdateStructureNode(c *fiber.Ctx) error {
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
		ParentStructureNodeID *string `json:"parent_structure_node_id"`
		NodeType              *string `json:"node_type" validate:"omitempty,oneof=building tower block cluster floor"`
		Name                  *string `json:"name" validate:"omitempty,min=1,max=100"`
		DisplayOrder          *int32  `json:"display_order"`
		Status                *string `json:"status" validate:"omitempty,oneof=active inactive"`
		Notes                 *string `json:"notes" validate:"omitempty,max=1000"`
		IsActive              *bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var parentStructureNodeID pgtype.UUID
	if req.ParentStructureNodeID != nil {
		id, err := uuid.Parse(*req.ParentStructureNodeID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid parent_structure_node_id"})
		}
		parentStructureNodeID = toPgUUID(id)
	}

	var displayOrder pgtype.Int4
	if req.DisplayOrder != nil {
		displayOrder = pgtype.Int4{Int32: *req.DisplayOrder, Valid: true}
	}

	var entity db.StructureNode

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.UpdateStructureNode(ctx, db.UpdateStructureNodeParams{
			ID:                    toPgUUID(id),
			ParentStructureNodeID: parentStructureNodeID,
			NodeType:              toPgText(req.NodeType),
			Name:                  toPgText(req.Name),
			DisplayOrder:          displayOrder,
			Status:                toPgText(req.Status),
			Notes:                 toPgText(req.Notes),
			IsActive:              toPgBool(req.IsActive),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "structure node not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists in project"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func DeactivateStructureNode(c *fiber.Ctx) error {
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
		return q.DeactivateStructureNode(ctx, toPgUUID(id))
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.SendStatus(204)
}
