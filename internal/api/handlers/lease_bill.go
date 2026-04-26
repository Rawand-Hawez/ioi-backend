package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

// IssueLeaseBill: draft -> issued. Creates one explicit receivable
// (source_module=rentals, source_record_type=lease_bill) and links bill.receivable_id.
func IssueLeaseBill(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease bill id"})
	}

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var bill db.LeaseBill
	var receivable db.Receivable
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetLeaseBill(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		lease, err := q.GetLeaseContract(ctx, existing.LeaseContractID)
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.bill.issue", existing.BusinessEntityID, existing.BranchID, lease.ProjectID); err != nil {
			return err
		}
		if existing.Status != "draft" {
			return ErrLeaseBillIssueNotDraft
		}

		notes := fmt.Sprintf("Lease bill for period %s to %s",
			existing.BillingPeriodStart.Time.Format("2006-01-02"),
			existing.BillingPeriodEnd.Time.Format("2006-01-02"))

		receivable, err = q.CreateReceivable(ctx, db.CreateReceivableParams{
			BusinessEntityID: existing.BusinessEntityID,
			BranchID:         existing.BranchID,
			PartyID:          existing.ResponsiblePartyID,
			UnitID:           existing.UnitID,
			SourceModule:     "rentals",
			SourceRecordType: "lease_bill",
			SourceRecordID:   existing.ID,
			ReceivableNo:     pgtype.Text{String: generateReceivableNo(), Valid: true},
			ReceivableDate:   existing.BillingPeriodStart,
			DueDate:          existing.DueDate,
			CurrencyCode:     existing.CurrencyCode,
			OriginalAmount:   existing.BilledAmount,
			Notes:            pgtype.Text{String: notes, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("create receivable: %w", err)
		}

		linked, err := q.LinkLeaseBillReceivable(ctx, db.LinkLeaseBillReceivableParams{
			ID:           existing.ID,
			ReceivableID: receivable.ID,
		})
		if err != nil {
			return err
		}

		issued, err := q.IssueLeaseBill(ctx, linked.ID)
		if err != nil {
			return err
		}
		bill = issued

		afterSnapshot, _ := json.Marshal(map[string]any{"bill": issued, "receivable": receivable})
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:            toPgUUID(userID),
			Module:            "rentals",
			ActionType:        "lease_bill_issued",
			EntityType:        "lease_bill",
			EntityID:          issued.ID,
			ScopeType:         "branch",
			ScopeID:           issued.BranchID,
			ResultStatus:      "success",
			SummaryText:       fmt.Sprintf("Issued lease bill and created receivable %s", receivable.ReceivableNo.String),
			AfterSnapshotJson: afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease bill not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("IssueLeaseBill error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to issue lease bill"})
	}
	return c.JSON(fiber.Map{"bill": bill, "receivable": receivable})
}

type voidLeaseBillRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// VoidLeaseBill voids a draft or issued bill. For issued bills, the linked receivable
// must be 'open'; partially_paid/paid/voided/written_off all refuse with 409.
func VoidLeaseBill(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lease bill id"})
	}

	var req voidLeaseBillRequest
	_ = c.BodyParser(&req)

	userIDStr, _ := claims["sub"].(string)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var bill db.LeaseBill
	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		existing, err := q.GetLeaseBill(ctx, toPgUUID(id))
		if err != nil {
			return err
		}
		lease, err := q.GetLeaseContract(ctx, existing.LeaseContractID)
		if err != nil {
			return err
		}
		if err := requireScopedPermission(ctx, q, userID, "rentals.bill.void", existing.BusinessEntityID, existing.BranchID, lease.ProjectID); err != nil {
			return err
		}

		switch existing.Status {
		case "draft":
			// no receivable to handle
		case "issued":
			if !existing.ReceivableID.Valid {
				return ErrLeaseBillMissingReceivable
			}
			receivable, err := q.GetReceivable(ctx, existing.ReceivableID)
			if err != nil {
				return err
			}
			if receivable.Status != "open" {
				return voidBillLinkedReceivableError(receivable.Status)
			}
			if _, err := q.VoidReceivable(ctx, receivable.ID); err != nil {
				return err
			}
		default:
			return voidBillStatusError(existing.Status)
		}

		voided, err := q.VoidLeaseBill(ctx, existing.ID)
		if err != nil {
			return err
		}
		bill = voided

		summary := fmt.Sprintf("Voided lease bill (was %s)", existing.Status)
		if req.Reason != nil && *req.Reason != "" {
			summary = fmt.Sprintf("%s: %s", summary, *req.Reason)
		}
		beforeSnapshot, _ := json.Marshal(existing)
		afterSnapshot, _ := json.Marshal(voided)
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:          pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:             toPgUUID(userID),
			Module:             "rentals",
			ActionType:         "lease_bill_voided",
			EntityType:         "lease_bill",
			EntityID:           voided.ID,
			ScopeType:          "branch",
			ScopeID:            voided.BranchID,
			ResultStatus:       "success",
			SummaryText:        summary,
			BeforeSnapshotJson: beforeSnapshot,
			AfterSnapshotJson:  afterSnapshot,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "lease bill not found"})
		}
		if errors.Is(err, errScopedPermissionDenied) {
			return c.Status(403).JSON(fiber.Map{"error": "permission denied for requested scope"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("VoidLeaseBill error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to void lease bill"})
	}
	return c.JSON(bill)
}
