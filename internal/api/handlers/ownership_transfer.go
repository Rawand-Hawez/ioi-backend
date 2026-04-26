package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

const dbTimeoutTransfer = 5 * time.Second

// RequestOwnershipTransfer handles POST /sales-contracts/:id/transfer-request
func RequestOwnershipTransfer(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	salesContractID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales contract id"})
	}

	var req struct {
		TransferType        string  `json:"transfer_type"`
		FromPartyID         string  `json:"from_party_id"`
		ToPartyID           string  `json:"to_party_id"`
		EffectiveDate       string  `json:"effective_date"`
		FinancialTreatment  string  `json:"financial_treatment"`
		TransferFeeAmount   *string `json:"transfer_fee_amount,omitempty"`
		TransferFeeCurrency *string `json:"transfer_fee_currency,omitempty"`
		Notes               *string `json:"notes,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	fromPartyID, err := uuid.Parse(req.FromPartyID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid from_party_id"})
	}
	toPartyID, err := uuid.Parse(req.ToPartyID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid to_party_id"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeoutTransfer)
	defer cancel()

	var transfer db.OwnershipTransfer
	var approvalRequestID pgtype.UUID

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		contract, err := q.GetSalesContract(ctx, toPgUUID(salesContractID))
		if err != nil {
			return err
		}

		if contract.Status != "active" {
			return ErrTransferContractNotActive
		}

		if _, err := q.GetActiveSalesContractPartyForParty(ctx, db.GetActiveSalesContractPartyForPartyParams{
			SalesContractID: toPgUUID(salesContractID),
			PartyID:         toPgUUID(fromPartyID),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrTransferFromPartyInvalid
			}
			return err
		}

		currency := "USD"
		if req.TransferFeeCurrency != nil && *req.TransferFeeCurrency != "" {
			currency = *req.TransferFeeCurrency
		}

		feeAmount := req.TransferFeeAmount
		if feeAmount == nil || *feeAmount == "" {
			zero := "0.00"
			feeAmount = &zero
		}

		transferParams := db.CreateOwnershipTransferParams{
			BusinessEntityID:    contract.BusinessEntityID,
			BranchID:            contract.BranchID,
			ProjectID:           contract.ProjectID,
			UnitID:              contract.UnitID,
			SalesContractID:     toPgUUID(salesContractID),
			ApprovalRequestID:   pgtype.UUID{Valid: false},
			TransferType:        req.TransferType,
			FromPartyID:         toPgUUID(fromPartyID),
			ToPartyID:           toPgUUID(toPartyID),
			EffectiveDate:       toPgDate(&req.EffectiveDate),
			FinancialTreatment:  req.FinancialTreatment,
			TransferFeeAmount:   toPgNumeric(feeAmount),
			TransferFeeCurrency: currency,
			Notes:               toPgText(req.Notes),
			Status:              "pending",
			RequestedByUserID:   toPgUUID(userID),
		}

		transfer, err = q.CreateOwnershipTransfer(ctx, transferParams)
		if err != nil {
			return err
		}

		payload, _ := json.Marshal(transfer)
		approvalReq, err := q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
			BusinessEntityID:    contract.BusinessEntityID,
			BranchID:            contract.BranchID,
			Module:              "sales",
			RequestType:         "ownership_transfer",
			SourceRecordType:    "ownership_transfer",
			SourceRecordID:      transfer.ID,
			RequestedByUserID:   toPgUUID(userID),
			Status:              "pending",
			PayloadSnapshotJson: payload,
		})
		if err != nil {
			return err
		}

		transfer, err = q.UpdateOwnershipTransferApprovalRequest(ctx, db.UpdateOwnershipTransferApprovalRequestParams{
			ID:                transfer.ID,
			ApprovalRequestID: approvalReq.ID,
		})
		if err != nil {
			return err
		}
		approvalRequestID = approvalReq.ID

		afterSnapshot, _ := json.Marshal(transfer)
		if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:                   toPgUUID(userID),
			Module:                   "sales",
			ActionType:               "request_ownership_transfer",
			EntityType:               "ownership_transfer",
			EntityID:                 transfer.ID,
			ScopeType:                "project",
			ScopeID:                  contract.ProjectID,
			ResultStatus:             "success",
			SummaryText:              "Requested ownership transfer",
			AfterSnapshotJson:        afterSnapshot,
			RelatedApprovalRequestID: approvalReq.ID,
		}); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "sales contract not found"})
		}
		if code := businessHTTPStatus(err); code != 0 {
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to request ownership transfer"})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":                  uuid.UUID(transfer.ID.Bytes).String(),
		"sales_contract_id":   uuid.UUID(transfer.SalesContractID.Bytes).String(),
		"unit_id":             uuid.UUID(transfer.UnitID.Bytes).String(),
		"from_party_id":       uuid.UUID(transfer.FromPartyID.Bytes).String(),
		"to_party_id":         uuid.UUID(transfer.ToPartyID.Bytes).String(),
		"transfer_type":       transfer.TransferType,
		"financial_treatment": transfer.FinancialTreatment,
		"status":              transfer.Status,
		"approval_request_id": uuid.UUID(approvalRequestID.Bytes).String(),
	})
}

// GetOwnershipTransfer handles GET /ownership-transfers/:id
func GetOwnershipTransfer(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid ownership transfer id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeoutTransfer)
	defer cancel()

	var transfer db.OwnershipTransfer

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		transfer, err = q.GetOwnershipTransfer(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ownership transfer not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to get ownership transfer"})
	}

	return c.JSON(transfer)
}

// CompleteOwnershipTransfer handles POST /ownership-transfers/:id/complete
func CompleteOwnershipTransfer(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid ownership transfer id"})
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok || userIDStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeoutTransfer)
	defer cancel()

	var transfer db.OwnershipTransfer

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		transfer, err = q.GetOwnershipTransfer(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if transfer.Status != "pending" {
			return errors.New("transfer is not in pending status")
		}

		if !transfer.ApprovalRequestID.Valid {
			return errors.New("transfer has no linked approval request")
		}

		approvalReq, err := q.GetApprovalRequest(ctx, transfer.ApprovalRequestID)
		if err != nil {
			return err
		}

		if approvalReq.Status != "approved" {
			return errors.New("approval not granted")
		}

		contract, err := q.GetSalesContract(ctx, transfer.SalesContractID)
		if err != nil {
			return err
		}
		if contract.Status != "active" {
			return errors.New("contract is not active")
		}
		beforeContract := contract

		fromContractParty, err := q.GetActiveSalesContractPartyForParty(ctx, db.GetActiveSalesContractPartyForPartyParams{
			SalesContractID: transfer.SalesContractID,
			PartyID:         transfer.FromPartyID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.New("no active sales_contract_party for from_party")
			}
			return err
		}

		effectiveDate := transfer.EffectiveDate
		if !effectiveDate.Valid {
			effectiveDate = pgtype.Date{Time: time.Now(), Valid: true}
		}

		if _, err := q.CloseSalesContractParty(ctx, db.CloseSalesContractPartyParams{
			ID:          fromContractParty.ID,
			EffectiveTo: effectiveDate,
		}); err != nil {
			return err
		}

		if _, err := q.CreateSalesContractParty(ctx, db.CreateSalesContractPartyParams{
			SalesContractID: transfer.SalesContractID,
			PartyID:         transfer.ToPartyID,
			Role:            "primary_buyer",
			IsPrimary:       true,
			EffectiveFrom:   effectiveDate,
			Status:          "active",
		}); err != nil {
			return err
		}

		updatedContract, err := q.UpdateSalesContractPrimaryBuyer(ctx, db.UpdateSalesContractPrimaryBuyerParams{
			ID:             transfer.SalesContractID,
			PrimaryBuyerID: transfer.ToPartyID,
		})
		if err != nil {
			return err
		}

		var sharePercentage pgtype.Numeric
		fromOwnership, err := q.GetActiveUnitOwnershipForParty(ctx, db.GetActiveUnitOwnershipForPartyParams{
			UnitID:  transfer.UnitID,
			PartyID: transfer.FromPartyID,
		})
		switch {
		case err == nil:
			if _, err := q.CloseUnitOwnership(ctx, db.CloseUnitOwnershipParams{
				ID:          fromOwnership.ID,
				EffectiveTo: effectiveDate,
			}); err != nil {
				return err
			}
			sharePercentage = fromOwnership.SharePercentage
		case errors.Is(err, pgx.ErrNoRows):
			sharePercentage = pgtype.Numeric{}
		default:
			return err
		}

		if !sharePercentage.Valid {
			defaultShare := "100.00"
			sharePercentage = toPgNumeric(&defaultShare)
		}

		if _, err := q.CreateUnitOwnership(ctx, db.CreateUnitOwnershipParams{
			UnitID:          transfer.UnitID,
			PartyID:         transfer.ToPartyID,
			SharePercentage: sharePercentage,
			EffectiveFrom:   effectiveDate,
			Status:          "active",
		}); err != nil {
			return err
		}

		transfer, err = q.SetOwnershipTransferCompletion(ctx, db.SetOwnershipTransferCompletionParams{
			ID:                toPgUUID(id),
			CompletedByUserID: toPgUUID(userID),
		})
		if err != nil {
			return err
		}

		beforeSnapshot, _ := json.Marshal(map[string]any{
			"transfer":             "pending",
			"sales_contract":       beforeContract,
			"from_contract_party":  fromContractParty,
		})
		afterSnapshot, _ := json.Marshal(map[string]any{
			"transfer":       transfer,
			"sales_contract": updatedContract,
		})
		if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			EventTime:                pgtype.Timestamptz{Time: time.Now(), Valid: true},
			UserID:                   toPgUUID(userID),
			Module:                   "sales",
			ActionType:               "complete_ownership_transfer",
			EntityType:               "ownership_transfer",
			EntityID:                 toPgUUID(id),
			ScopeType:                "project",
			ScopeID:                  transfer.ProjectID,
			ResultStatus:             "success",
			SummaryText:              "Completed ownership transfer",
			BeforeSnapshotJson:       beforeSnapshot,
			AfterSnapshotJson:        afterSnapshot,
			RelatedApprovalRequestID: transfer.ApprovalRequestID,
		}); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ownership transfer not found"})
		}
		switch err.Error() {
		case "transfer is not in pending status",
			"contract is not active":
			return c.Status(409).JSON(fiber.Map{"error": err.Error()})
		case "approval not granted",
			"transfer has no linked approval request",
			"no active sales_contract_party for from_party":
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "failed to complete ownership transfer"})
	}

	return c.JSON(fiber.Map{
		"message":  "ownership transfer completed",
		"transfer": transfer,
	})
}
