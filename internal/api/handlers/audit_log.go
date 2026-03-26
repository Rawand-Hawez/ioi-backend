// backend/internal/api/handlers/audit_log.go

package handlers

import (
	"context"
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

func parseTimestampParam(s string) (pgtype.Timestamptz, error) {
	if s == "" {
		return pgtype.Timestamptz{Valid: false}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return pgtype.Timestamptz{Time: t, Valid: true}, nil
	}
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return pgtype.Timestamptz{Time: t, Valid: true}, nil
	}
	return pgtype.Timestamptz{Valid: false}, fmt.Errorf("invalid timestamp format")
}

func ListAuditLogs(c *fiber.Ctx) error {
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

	var entityType pgtype.Text
	if c.Query("entity_type") != "" {
		entityType = pgtype.Text{String: c.Query("entity_type"), Valid: true}
	}

	var entityID pgtype.UUID
	if c.Query("entity_id") != "" {
		id, err := uuid.Parse(c.Query("entity_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid entity_id"})
		}
		entityID = toPgUUID(id)
	}

	var userID pgtype.UUID
	if c.Query("user_id") != "" {
		id, err := uuid.Parse(c.Query("user_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid user_id"})
		}
		userID = toPgUUID(id)
	}

	var actionType pgtype.Text
	if c.Query("action_type") != "" {
		actionType = pgtype.Text{String: c.Query("action_type"), Valid: true}
	}

	var module pgtype.Text
	if c.Query("module") != "" {
		module = pgtype.Text{String: c.Query("module"), Valid: true}
	}

	dateFrom, err := parseTimestampParam(c.Query("date_from"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid date_from format"})
	}

	dateTo, err := parseTimestampParam(c.Query("date_to"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid date_to format"})
	}

	var total int64
	var items []db.AuditLog

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountAuditLogs(ctx, db.CountAuditLogsParams{
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     userID,
			ActionType: actionType,
			Module:     module,
			DateFrom:   dateFrom,
			DateTo:     dateTo,
		})
		if err != nil {
			return fmt.Errorf("count audit logs: %w", err)
		}

		items, err = q.ListAuditLogs(ctx, db.ListAuditLogsParams{
			Limit:      int32(perPage),
			Offset:     int32(offset),
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     userID,
			ActionType: actionType,
			Module:     module,
			DateFrom:   dateFrom,
			DateTo:     dateTo,
		})
		if err != nil {
			return fmt.Errorf("list audit logs: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.AuditLog{}
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

func GetAuditLogsForEntity(c *fiber.Ctx) error {
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

	entityType := c.Params("entity_type")
	if entityType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "entity_type is required"})
	}

	entityIDStr := c.Params("entity_id")
	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid entity_id"})
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

	var total int64
	var items []db.AuditLog

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountAuditLogsForEntity(ctx, db.CountAuditLogsForEntityParams{
			EntityType: entityType,
			EntityID:   toPgUUID(entityID),
		})
		if err != nil {
			return fmt.Errorf("count audit logs for entity: %w", err)
		}

		items, err = q.GetAuditLogsForEntity(ctx, db.GetAuditLogsForEntityParams{
			EntityType: entityType,
			EntityID:   toPgUUID(entityID),
			Limit:      int32(perPage),
			Offset:     int32(offset),
		})
		if err != nil {
			return fmt.Errorf("get audit logs for entity: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.AuditLog{}
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
