package handlers

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

var (
	ErrBusinessEntityNotFound = errors.New("business entity not found")
	ErrBranchNotFound         = errors.New("branch not found")
	ErrProjectNotFound        = errors.New("project not found")
)

func ListRoles(c *fiber.Ctx) error {
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

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var items []db.Role

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.ListRoles(ctx, isActive)
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Role{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func ListPermissions(c *fiber.Ctx) error {
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

	var module pgtype.Text
	if c.Query("module") != "" {
		module = pgtype.Text{String: c.Query("module"), Valid: true}
	}

	var items []db.Permission

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.ListPermissions(ctx, module)
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Permission{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func ListRolePermissions(c *fiber.Ctx) error {
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

	roleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid role id"})
	}

	var items []db.Permission

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		_, err := q.GetRole(ctx, toPgUUID(roleID))
		if err != nil {
			return err
		}
		items, err = q.ListRolePermissions(ctx, toPgUUID(roleID))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "role not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Permission{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func ListUserRoleAssignments(c *fiber.Ctx) error {
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

	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	var items []db.UserRoleScopeAssignment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.ListUserRoleAssignments(ctx, toPgUUID(userID))
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.UserRoleScopeAssignment{}
	}

	return c.JSON(fiber.Map{"data": items})
}

func GetUserRoleAssignment(c *fiber.Ctx) error {
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

	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid assignment id"})
	}

	var assignment db.UserRoleScopeAssignment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		assignment, err = q.GetUserRoleAssignment(ctx, toPgUUID(assignmentID))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "assignment not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(assignment)
}

func AssignRoleToUser(c *fiber.Ctx) error {
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

	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req struct {
		RoleID    string  `json:"role_id"`
		ScopeType string  `json:"scope_type"`
		ScopeID   *string `json:"scope_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid role_id"})
	}

	validScopeTypes := map[string]bool{
		"deployment":      true,
		"business_entity": true,
		"branch":          true,
		"project":         true,
	}
	if !validScopeTypes[req.ScopeType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid scope_type, must be one of: deployment, business_entity, branch, project"})
	}

	var scopeID pgtype.UUID
	if req.ScopeID != nil && *req.ScopeID != "" {
		parsedScopeID, err := uuid.Parse(*req.ScopeID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid scope_id"})
		}
		scopeID = toPgUUID(parsedScopeID)
	}

	if req.ScopeType == "deployment" && scopeID.Valid {
		return c.Status(400).JSON(fiber.Map{"error": "scope_id must be null for deployment scope type"})
	}

	if req.ScopeType != "deployment" && !scopeID.Valid {
		return c.Status(400).JSON(fiber.Map{"error": "scope_id is required for non-deployment scope types"})
	}

	var assignment db.UserRoleScopeAssignment

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		if scopeID.Valid {
			switch req.ScopeType {
			case "business_entity":
				_, err := q.GetBusinessEntity(ctx, scopeID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return ErrBusinessEntityNotFound
					}
					return err
				}
			case "branch":
				_, err := q.GetBranch(ctx, scopeID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return ErrBranchNotFound
					}
					return err
				}
			case "project":
				_, err := q.GetProject(ctx, scopeID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return ErrProjectNotFound
					}
					return err
				}
			}
		}

		var err error
		assignment, err = q.AssignRoleToUser(ctx, db.AssignRoleToUserParams{
			UserID:    toPgUUID(userID),
			RoleID:    toPgUUID(roleID),
			ScopeType: req.ScopeType,
			ScopeID:   scopeID,
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "role already assigned to user with this scope"})
		}
		if errors.Is(err, ErrBusinessEntityNotFound) ||
			errors.Is(err, ErrBranchNotFound) ||
			errors.Is(err, ErrProjectNotFound) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(assignment)
}

func RemoveRoleFromUser(c *fiber.Ctx) error {
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

	assignmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid assignment id"})
	}

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		_, err := q.RemoveRoleFromUser(ctx, toPgUUID(assignmentID))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "assignment not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(204).SendString("")
}

func GetMyPermissions(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var items []db.GetUserPermissionsRow

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		items, err = q.GetUserPermissions(ctx, toPgUUID(userID))
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.GetUserPermissionsRow{}
	}

	return c.JSON(fiber.Map{"data": items, "user_id": userIDStr})
}
