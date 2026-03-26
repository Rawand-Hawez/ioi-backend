// backend/internal/api/handlers/approval_policy.go

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

func ListApprovalPolicies(c *fiber.Ctx) error {
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

	var module pgtype.Text
	if c.Query("module") != "" {
		module = pgtype.Text{String: c.Query("module"), Valid: true}
	}

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var total int64
	var items []db.ApprovalPolicy

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountApprovalPolicies(ctx, db.CountApprovalPoliciesParams{
			BusinessEntityID: businessEntityID,
			Module:           module,
			IsActive:         isActive,
		})
		if err != nil {
			return fmt.Errorf("count approval policies: %w", err)
		}

		items, err = q.ListApprovalPolicies(ctx, db.ListApprovalPoliciesParams{
			BusinessEntityID: businessEntityID,
			Module:           module,
			IsActive:         isActive,
			Limit:            int32(perPage),
			Offset:           int32(offset),
		})
		if err != nil {
			return fmt.Errorf("list approval policies: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.ApprovalPolicy{}
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

func GetApprovalPolicy(c *fiber.Ctx) error {
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

	var policy db.ApprovalPolicy

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		policy, err = q.GetApprovalPolicy(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "approval policy not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(policy)
}

func CreateApprovalPolicy(c *fiber.Ctx) error {
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
		BusinessEntityID       string  `json:"business_entity_id" validate:"required"`
		Code                   string  `json:"code" validate:"required"`
		Name                   string  `json:"name" validate:"required"`
		Module                 string  `json:"module" validate:"required"`
		RequestType            string  `json:"request_type" validate:"required"`
		MinApprovers           *int16  `json:"min_approvers"`
		PreventSelfApproval    *bool   `json:"prevent_self_approval"`
		ApproverRoleID         *string `json:"approver_role_id"`
		AutoApproveBelowAmount *string `json:"auto_approve_below_amount"`
		ExpiryHours            *int32  `json:"expiry_hours"`
		IsActive               *bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	validModules := map[string]bool{
		"sales": true, "finance": true, "rentals": true,
		"service_charges": true, "utilities": true,
	}
	if !validModules[req.Module] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid module, must be one of: sales, finance, rentals, service_charges, utilities"})
	}

	validRequestTypes := map[string]bool{
		"ownership_transfer": true, "payment_void": true, "deposit_refund": true,
		"contract_cancellation": true, "schedule_restructure": true,
		"financial_adjustment": true, "lease_termination": true,
		"manual_override": true, "prepaid_adjustment": true,
	}
	if !validRequestTypes[req.RequestType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request_type"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	minApprovers := int16(1)
	if req.MinApprovers != nil {
		minApprovers = *req.MinApprovers
	}

	preventSelfApproval := true
	if req.PreventSelfApproval != nil {
		preventSelfApproval = *req.PreventSelfApproval
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var approverRoleID pgtype.UUID
	if req.ApproverRoleID != nil {
		id, err := uuid.Parse(*req.ApproverRoleID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid approver_role_id"})
		}
		approverRoleID = toPgUUID(id)
	}

	var autoApproveBelowAmount pgtype.Numeric
	if req.AutoApproveBelowAmount != nil {
		if err := autoApproveBelowAmount.Scan(*req.AutoApproveBelowAmount); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid auto_approve_below_amount"})
		}
	}

	var expiryHours pgtype.Int4
	if req.ExpiryHours != nil {
		expiryHours = pgtype.Int4{Int32: *req.ExpiryHours, Valid: true}
	}

	var policy db.ApprovalPolicy

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		policy, err = q.CreateApprovalPolicy(ctx, db.CreateApprovalPolicyParams{
			BusinessEntityID:       toPgUUID(businessEntityID),
			Code:                   req.Code,
			Name:                   req.Name,
			Module:                 req.Module,
			RequestType:            req.RequestType,
			MinApprovers:           minApprovers,
			PreventSelfApproval:    preventSelfApproval,
			ApproverRoleID:         approverRoleID,
			AutoApproveBelowAmount: autoApproveBelowAmount,
			ExpiryHours:            expiryHours,
			IsActive:               isActive,
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "policy code already exists for this business entity"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(policy)
}

func UpdateApprovalPolicy(c *fiber.Ctx) error {
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
		Name                   *string `json:"name"`
		MinApprovers           *int16  `json:"min_approvers"`
		PreventSelfApproval    *bool   `json:"prevent_self_approval"`
		ApproverRoleID         *string `json:"approver_role_id"`
		AutoApproveBelowAmount *string `json:"auto_approve_below_amount"`
		ExpiryHours            *int32  `json:"expiry_hours"`
		IsActive               *bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var approverRoleID pgtype.UUID
	if req.ApproverRoleID != nil {
		rid, err := uuid.Parse(*req.ApproverRoleID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid approver_role_id"})
		}
		approverRoleID = toPgUUID(rid)
	}

	var autoApproveBelowAmount pgtype.Numeric
	if req.AutoApproveBelowAmount != nil {
		if err := autoApproveBelowAmount.Scan(*req.AutoApproveBelowAmount); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid auto_approve_below_amount"})
		}
	}

	var policy db.ApprovalPolicy

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		policy, err = q.UpdateApprovalPolicy(ctx, db.UpdateApprovalPolicyParams{
			ID:                     toPgUUID(id),
			Name:                   toPgText(req.Name),
			MinApprovers:           toPgInt2(req.MinApprovers),
			PreventSelfApproval:    toPgBool(req.PreventSelfApproval),
			ApproverRoleID:         approverRoleID,
			AutoApproveBelowAmount: autoApproveBelowAmount,
			ExpiryHours:            toPgInt4(req.ExpiryHours),
			IsActive:               toPgBool(req.IsActive),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "approval policy not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "policy code already exists for this business entity"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(policy)
}
