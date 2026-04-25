// backend/internal/api/handlers/approval_request.go

package handlers

import (
	"context"
	"encoding/json"
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

var (
	ErrSelfApprovalBlocked = errors.New("self-approval is not allowed by policy")
	ErrRequestNotPending   = errors.New("approval request is not in pending status")
	ErrNotEligibleApprover = errors.New("you are not an eligible pending approver for this request, or you have already registered your decision")
)

var validModules = map[string]bool{
	"sales": true, "finance": true, "rentals": true,
	"service_charges": true, "utilities": true,
}

var validRequestTypes = map[string]bool{
	"ownership_transfer": true, "payment_void": true, "deposit_refund": true,
	"contract_cancellation": true, "contract_termination": true,
	"schedule_restructure": true, "financial_adjustment": true,
	"lease_termination": true, "manual_override": true, "prepaid_adjustment": true,
}

func ListApprovalRequests(c *fiber.Ctx) error {
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

	var branchID pgtype.UUID
	if c.Query("branch_id") != "" {
		id, err := uuid.Parse(c.Query("branch_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid branch_id"})
		}
		branchID = toPgUUID(id)
	}

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var module pgtype.Text
	if c.Query("module") != "" {
		module = pgtype.Text{String: c.Query("module"), Valid: true}
	}

	var requestType pgtype.Text
	if c.Query("request_type") != "" {
		requestType = pgtype.Text{String: c.Query("request_type"), Valid: true}
	}

	var requestedByUserID pgtype.UUID
	if c.Query("requested_by_user_id") != "" {
		id, err := uuid.Parse(c.Query("requested_by_user_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid requested_by_user_id"})
		}
		requestedByUserID = toPgUUID(id)
	}

	var total int64
	var items []db.ApprovalRequest

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountApprovalRequests(ctx, db.CountApprovalRequestsParams{
			BusinessEntityID:  businessEntityID,
			BranchID:          branchID,
			Status:            status,
			Module:            module,
			RequestType:       requestType,
			RequestedByUserID: requestedByUserID,
		})
		if err != nil {
			return fmt.Errorf("count approval requests: %w", err)
		}

		items, err = q.ListApprovalRequests(ctx, db.ListApprovalRequestsParams{
			BusinessEntityID:  businessEntityID,
			BranchID:          branchID,
			Status:            status,
			Module:            module,
			RequestType:       requestType,
			RequestedByUserID: requestedByUserID,
			Limit:             int32(perPage),
			Offset:            int32(offset),
		})
		if err != nil {
			return fmt.Errorf("list approval requests: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.ApprovalRequest{}
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

func GetApprovalRequest(c *fiber.Ctx) error {
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

	var request db.ApprovalRequest
	var approvers []db.ApprovalRequestApprover

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		request, err = q.GetApprovalRequest(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		approvers, err = q.ListApprovalRequestApprovers(ctx, toPgUUID(id))
		if err != nil {
			return fmt.Errorf("list approvers: %w", err)
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "approval request not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if approvers == nil {
		approvers = []db.ApprovalRequestApprover{}
	}

	return c.JSON(fiber.Map{
		"request":   request,
		"approvers": approvers,
	})
}

func CreateApprovalRequest(c *fiber.Ctx) error {
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
		BusinessEntityID string          `json:"business_entity_id" validate:"required"`
		BranchID         *string         `json:"branch_id"`
		ApprovalPolicyID *string         `json:"approval_policy_id"`
		Module           string          `json:"module" validate:"required"`
		RequestType      string          `json:"request_type" validate:"required"`
		SourceRecordType string          `json:"source_record_type" validate:"required"`
		SourceRecordID   string          `json:"source_record_id" validate:"required"`
		AssignedToUserID *string         `json:"assigned_to_user_id"`
		Status           string          `json:"status"`
		PayloadSnapshot  json.RawMessage `json:"payload_snapshot" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if !validModules[req.Module] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid module value"})
	}
	if !validRequestTypes[req.RequestType] {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request_type value"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	var branchID pgtype.UUID
	if req.BranchID != nil {
		id, err := uuid.Parse(*req.BranchID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid branch_id"})
		}
		branchID = toPgUUID(id)
	}

	var approvalPolicyID pgtype.UUID
	if req.ApprovalPolicyID != nil {
		id, err := uuid.Parse(*req.ApprovalPolicyID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid approval_policy_id"})
		}
		approvalPolicyID = toPgUUID(id)
	}

	sourceRecordID, err := uuid.Parse(req.SourceRecordID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid source_record_id"})
	}

	var assignedToUserID pgtype.UUID
	if req.AssignedToUserID != nil {
		id, err := uuid.Parse(*req.AssignedToUserID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid assigned_to_user_id"})
		}
		assignedToUserID = toPgUUID(id)
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	requestedByUserID, err := uuid.Parse(sub)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	status := req.Status
	if status == "" {
		status = "pending"
	}

	payloadBytes, err := json.Marshal(req.PayloadSnapshot)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload_snapshot"})
	}

	var request db.ApprovalRequest

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		request, err = q.CreateApprovalRequest(ctx, db.CreateApprovalRequestParams{
			BusinessEntityID:    toPgUUID(businessEntityID),
			BranchID:            branchID,
			ApprovalPolicyID:    approvalPolicyID,
			Module:              req.Module,
			RequestType:         req.RequestType,
			SourceRecordType:    req.SourceRecordType,
			SourceRecordID:      toPgUUID(sourceRecordID),
			RequestedByUserID:   toPgUUID(requestedByUserID),
			AssignedToUserID:    assignedToUserID,
			Status:              status,
			PayloadSnapshotJson: payloadBytes,
		})
		if err != nil {
			return fmt.Errorf("create approval request: %w", err)
		}

		if req.AssignedToUserID != nil {
			_, err = q.CreateApprovalRequestApprover(ctx, db.CreateApprovalRequestApproverParams{
				ApprovalRequestID: request.ID,
				UserID:            assignedToUserID,
			})
			if err != nil {
				return fmt.Errorf("create approver: %w", err)
			}
		}

		summaryBytes, _ := json.Marshal(fiber.Map{"request_type": req.RequestType, "module": req.Module})
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			UserID:            toPgUUID(requestedByUserID),
			Module:            req.Module,
			ActionType:        "approval_request_created",
			EntityType:        "approval_request",
			EntityID:          request.ID,
			ScopeType:         "business_entity",
			ScopeID:           toPgUUID(businessEntityID),
			ResultStatus:      "success",
			SummaryText:       "Approval request created",
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			AfterSnapshotJson: summaryBytes,
		})
		if err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}

		return nil
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "approval request already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(request)
}

func DecideApprovalRequest(c *fiber.Ctx) error {
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
		Decision string  `json:"decision" validate:"required,oneof=approved rejected"`
		Reason   *string `json:"reason"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	currentUserID, err := uuid.Parse(sub)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var request db.ApprovalRequest

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		request, err = q.GetApprovalRequest(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if request.Status != "pending" {
			return ErrRequestNotPending
		}

		if request.ApprovalPolicyID.Valid {
			policy, err := q.GetApprovalPolicy(ctx, request.ApprovalPolicyID)
			if err != nil {
				return fmt.Errorf("get approval policy: %w", err)
			}

			if policy.PreventSelfApproval && request.RequestedByUserID.Bytes == currentUserID {
				return ErrSelfApprovalBlocked
			}
		}

		_, err = q.RecordApproverDecision(ctx, db.RecordApproverDecisionParams{
			ApprovalRequestID: toPgUUID(id),
			UserID:            toPgUUID(currentUserID),
			Decision:          pgtype.Text{String: req.Decision, Valid: true},
			DecisionReason:    toPgText(req.Reason),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotEligibleApprover
			}
			return fmt.Errorf("record approver decision: %w", err)
		}

		if req.Decision == "rejected" {
			request, err = q.UpdateApprovalRequestStatus(ctx, db.UpdateApprovalRequestStatusParams{
				ID:     toPgUUID(id),
				Status: "rejected",
			})
			if err != nil {
				return fmt.Errorf("update request status to rejected: %w", err)
			}
		} else if req.Decision == "approved" {
			approvedCount, err := q.CountApprovedDecisions(ctx, toPgUUID(id))
			if err != nil {
				return fmt.Errorf("count approved decisions: %w", err)
			}

			minApprovers := int64(1)
			if request.ApprovalPolicyID.Valid {
				policy, err := q.GetApprovalPolicy(ctx, request.ApprovalPolicyID)
				if err != nil {
					return fmt.Errorf("get approval policy for min_approvers: %w", err)
				}
				minApprovers = int64(policy.MinApprovers)
			}

			if approvedCount >= minApprovers {
				request, err = q.UpdateApprovalRequestStatus(ctx, db.UpdateApprovalRequestStatusParams{
					ID:     toPgUUID(id),
					Status: "approved",
				})
				if err != nil {
					return fmt.Errorf("update request status to approved: %w", err)
				}
			}
		}

		summaryBytes, _ := json.Marshal(fiber.Map{"decision": req.Decision, "request_id": id.String()})
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			UserID:            toPgUUID(currentUserID),
			Module:            request.Module,
			ActionType:        "approval_decision_recorded",
			EntityType:        "approval_request",
			EntityID:          toPgUUID(id),
			ScopeType:         "business_entity",
			ScopeID:           request.BusinessEntityID,
			ResultStatus:      "success",
			SummaryText:       "Approval decision recorded",
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			AfterSnapshotJson: summaryBytes,
		})
		if err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "approval request not found"})
		}
		if errors.Is(err, ErrRequestNotPending) || errors.Is(err, ErrSelfApprovalBlocked) || errors.Is(err, ErrNotEligibleApprover) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(request)
}

func CancelApprovalRequest(c *fiber.Ctx) error {
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
		Reason *string `json:"reason"`
	}

	if err := c.BodyParser(&req); err != nil && err.Error() != "cannot parse empty body" {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id in token"})
	}
	currentUserID, err := uuid.Parse(sub)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
	}

	var request db.ApprovalRequest

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		request, err = q.GetApprovalRequest(ctx, toPgUUID(id))
		if err != nil {
			return err
		}

		if request.RequestedByUserID.Bytes != currentUserID {
			return fiber.NewError(403, "only the requester can cancel")
		}

		request, err = q.CancelApprovalRequest(ctx, toPgUUID(id))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fiber.NewError(400, "request cannot be cancelled (not in draft or pending status)")
			}
			return fmt.Errorf("cancel approval request: %w", err)
		}

		summaryBytes, _ := json.Marshal(fiber.Map{"request_id": id.String(), "reason": req.Reason})
		_, err = q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			UserID:            toPgUUID(currentUserID),
			Module:            request.Module,
			ActionType:        "approval_request_cancelled",
			EntityType:        "approval_request",
			EntityID:          toPgUUID(id),
			ScopeType:         "business_entity",
			ScopeID:           request.BusinessEntityID,
			ResultStatus:      "success",
			SummaryText:       "Approval request cancelled",
			EventTime:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
			AfterSnapshotJson: summaryBytes,
		})
		if err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "approval request not found"})
		}
		if fiberErr, ok := err.(*fiber.Error); ok {
			return c.Status(fiberErr.Code).JSON(fiber.Map{"error": fiberErr.Message})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(request)
}

func AddApprovalRequestApprover(c *fiber.Ctx) error {
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
		ApprovalRequestID string `json:"approval_request_id" validate:"required"`
		UserID            string `json:"user_id" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	approvalRequestID, err := uuid.Parse(req.ApprovalRequestID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid approval_request_id"})
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user_id"})
	}

	var approver db.ApprovalRequestApprover

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		request, err := q.GetApprovalRequest(ctx, toPgUUID(approvalRequestID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fiber.NewError(404, "approval request not found")
			}
			return fmt.Errorf("get approval request: %w", err)
		}

		if request.Status != "pending" {
			return fiber.NewError(400, "can only add approvers to pending requests")
		}

		approver, err = q.CreateApprovalRequestApprover(ctx, db.CreateApprovalRequestApproverParams{
			ApprovalRequestID: toPgUUID(approvalRequestID),
			UserID:            toPgUUID(userID),
		})
		if err != nil {
			return fmt.Errorf("create approver: %w", err)
		}

		return nil
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "user is already an approver"})
		}
		if fiberErr, ok := err.(*fiber.Error); ok {
			return c.Status(fiberErr.Code).JSON(fiber.Map{"error": fiberErr.Message})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(approver)
}
