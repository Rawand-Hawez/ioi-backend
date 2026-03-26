-- backend/db/queries/approvals_audit.sql

-- =============================================================================
-- Approvals + Audit Domain Queries
-- =============================================================================

-- name: ListApprovalPolicies :many
SELECT * FROM public.approval_policies
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountApprovalPolicies :one
SELECT count(*) FROM public.approval_policies
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: GetApprovalPolicy :one
SELECT * FROM public.approval_policies WHERE id = $1;

-- name: GetApprovalPolicyByCode :one
SELECT * FROM public.approval_policies 
WHERE business_entity_id = $1 AND code = $2;

-- name: CreateApprovalPolicy :one
INSERT INTO public.approval_policies (
    business_entity_id, code, name, module, request_type,
    min_approvers, prevent_self_approval, approver_role_id,
    auto_approve_below_amount, expiry_hours, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateApprovalPolicy :one
UPDATE public.approval_policies
SET name = COALESCE(sqlc.narg('name')::text, name),
    min_approvers = COALESCE(sqlc.narg('min_approvers')::smallint, min_approvers),
    prevent_self_approval = COALESCE(sqlc.narg('prevent_self_approval')::boolean, prevent_self_approval),
    approver_role_id = COALESCE(sqlc.narg('approver_role_id')::uuid, approver_role_id),
    auto_approve_below_amount = COALESCE(sqlc.narg('auto_approve_below_amount')::numeric, auto_approve_below_amount),
    expiry_hours = COALESCE(sqlc.narg('expiry_hours')::integer, expiry_hours),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active),
    updated_at = timezone('utc', now())
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ListApprovalRequests :many
SELECT * FROM public.approval_requests
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('request_type')::text IS NULL OR request_type = sqlc.narg('request_type')::text)
  AND (sqlc.narg('requested_by_user_id')::uuid IS NULL OR requested_by_user_id = sqlc.narg('requested_by_user_id')::uuid)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountApprovalRequests :one
SELECT count(*) FROM public.approval_requests
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('request_type')::text IS NULL OR request_type = sqlc.narg('request_type')::text)
  AND (sqlc.narg('requested_by_user_id')::uuid IS NULL OR requested_by_user_id = sqlc.narg('requested_by_user_id')::uuid);

-- name: GetApprovalRequest :one
SELECT * FROM public.approval_requests WHERE id = $1;

-- name: CreateApprovalRequest :one
INSERT INTO public.approval_requests (
    business_entity_id, branch_id, approval_policy_id,
    module, request_type, source_record_type, source_record_id,
    requested_by_user_id, assigned_to_user_id, status,
    payload_snapshot_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateApprovalRequestStatus :one
UPDATE public.approval_requests
SET status = $2,
    decided_at = CASE WHEN $2 IN ('approved', 'rejected') THEN timezone('utc', now()) ELSE decided_at END,
    decision_reason = COALESCE($3, decision_reason),
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: CancelApprovalRequest :one
UPDATE public.approval_requests
SET status = 'cancelled',
    updated_at = timezone('utc', now())
WHERE id = $1 AND status IN ('draft', 'pending')
RETURNING *;

-- name: CreateApprovalRequestApprover :one
INSERT INTO public.approval_request_approvers (
    approval_request_id, user_id
) VALUES ($1, $2)
RETURNING *;

-- name: RecordApproverDecision :one
UPDATE public.approval_request_approvers
SET decision = $2,
    decided_at = timezone('utc', now()),
    decision_reason = $3
WHERE approval_request_id = $1 AND user_id = $4 AND decision IS NULL
RETURNING *;

-- name: CountApprovedDecisions :one
SELECT count(*) FROM public.approval_request_approvers
WHERE approval_request_id = $1 AND decision = 'approved';

-- name: CountPendingApprovers :one
SELECT count(*) FROM public.approval_request_approvers
WHERE approval_request_id = $1 AND decision IS NULL;

-- name: ListApprovalRequestApprovers :many
SELECT * FROM public.approval_request_approvers
WHERE approval_request_id = $1
ORDER BY created_at ASC;

-- name: InsertAuditLog :one
INSERT INTO public.audit_logs (
    event_time, user_id, module, action_type,
    entity_type, entity_id, scope_type, scope_id,
    result_status, reason, summary_text,
    before_snapshot_json, after_snapshot_json,
    related_approval_request_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: ListAuditLogs :many
SELECT * FROM public.audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id')::uuid)
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
  AND (sqlc.narg('action_type')::text IS NULL OR action_type = sqlc.narg('action_type')::text)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR event_time >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR event_time <= sqlc.narg('date_to')::timestamptz)
ORDER BY event_time DESC
LIMIT $1 OFFSET $2;

-- name: CountAuditLogs :one
SELECT count(*) FROM public.audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id')::uuid)
  AND (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
  AND (sqlc.narg('action_type')::text IS NULL OR action_type = sqlc.narg('action_type')::text)
  AND (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR event_time >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR event_time <= sqlc.narg('date_to')::timestamptz);

-- name: GetAuditLogsForEntity :many
SELECT * FROM public.audit_logs
WHERE entity_type = $1 AND entity_id = $2
ORDER BY event_time DESC
LIMIT $3 OFFSET $4;

-- name: CountAuditLogsForEntity :one
SELECT count(*) FROM public.audit_logs
WHERE entity_type = $1 AND entity_id = $2;
