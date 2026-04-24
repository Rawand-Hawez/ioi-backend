-- backend/db/queries/sales.sql

-- =============================================================================
-- Sales Domain Queries
-- =============================================================================

-- =============================================================================
-- Reservations
-- =============================================================================

-- name: ListReservations :many
SELECT * FROM public.reservations
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('customer_id')::uuid IS NULL OR customer_id = sqlc.narg('customer_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountReservations :one
SELECT COUNT(*)::bigint FROM public.reservations
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('customer_id')::uuid IS NULL OR customer_id = sqlc.narg('customer_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetReservation :one
SELECT * FROM public.reservations WHERE id = $1;

-- name: CreateReservation :one
INSERT INTO public.reservations (
    business_entity_id, branch_id, project_id, unit_id, customer_id,
    expires_at, deposit_amount, deposit_currency, deposit_payment_id,
    quoted_price_amount, discount_amount, notes, created_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateReservationStatus :one
UPDATE public.reservations
SET status = $2, updated_at = timezone('utc', now())
WHERE id = $1 AND status = $3
RETURNING *;

-- name: GetActiveReservationForUnit :one
SELECT * FROM public.reservations WHERE unit_id = $1 AND status = 'active' LIMIT 1;

-- name: ExpireReservations :many
UPDATE public.reservations
SET status = 'expired', updated_at = timezone('utc', now())
WHERE status = 'active' AND expires_at < timezone('utc', now())
RETURNING id;

-- name: LinkReservationDepositPayment :one
UPDATE public.reservations
SET deposit_payment_id = $2, updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- =============================================================================
-- Sales Contracts
-- =============================================================================

-- name: ListSalesContracts :many
SELECT * FROM public.sales_contracts
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('primary_buyer_id')::uuid IS NULL OR primary_buyer_id = sqlc.narg('primary_buyer_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountSalesContracts :one
SELECT COUNT(*)::bigint FROM public.sales_contracts
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('primary_buyer_id')::uuid IS NULL OR primary_buyer_id = sqlc.narg('primary_buyer_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetSalesContract :one
SELECT * FROM public.sales_contracts WHERE id = $1;

-- name: CreateSalesContract :one
INSERT INTO public.sales_contracts (
    business_entity_id, branch_id, project_id, unit_id, primary_buyer_id,
    source_reservation_id, contract_date, effective_date, sale_price_amount,
    sale_price_currency, discount_amount, net_contract_amount,
    down_payment_amount, financed_amount, payment_plan_template_id,
    handover_date_planned, notes, created_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING *;

-- name: UpdateSalesContract :one
UPDATE public.sales_contracts SET
    sale_price_amount = COALESCE(sqlc.narg('sale_price_amount')::numeric, sale_price_amount),
    discount_amount = COALESCE(sqlc.narg('discount_amount')::numeric, discount_amount),
    net_contract_amount = COALESCE(sqlc.narg('net_contract_amount')::numeric, net_contract_amount),
    down_payment_amount = COALESCE(sqlc.narg('down_payment_amount')::numeric, down_payment_amount),
    financed_amount = COALESCE(sqlc.narg('financed_amount')::numeric, financed_amount),
    payment_plan_template_id = COALESCE(sqlc.narg('payment_plan_template_id')::uuid, payment_plan_template_id),
    handover_date_planned = COALESCE(sqlc.narg('handover_date_planned')::date, handover_date_planned),
    handover_date_actual = COALESCE(sqlc.narg('handover_date_actual')::date, handover_date_actual),
    notes = COALESCE(sqlc.narg('notes')::text, notes),
    updated_at = timezone('utc', now())
WHERE id = sqlc.arg('id')::uuid AND status = 'draft'
RETURNING *;

-- name: UpdateSalesContractStatus :one
UPDATE public.sales_contracts
SET status = $2, updated_at = timezone('utc', now())
WHERE id = $1 AND status = $3
RETURNING *;

-- name: GetActiveSalesContractForUnit :one
SELECT * FROM public.sales_contracts WHERE unit_id = $1 AND status = 'active' LIMIT 1;

-- name: GetSalesContractBySourceReservation :one
SELECT * FROM public.sales_contracts WHERE source_reservation_id = $1 LIMIT 1;

-- =============================================================================
-- Payment Plan Templates
-- =============================================================================

-- name: ListPaymentPlanTemplates :many
SELECT * FROM public.payment_plan_templates
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountPaymentPlanTemplates :one
SELECT COUNT(*)::bigint FROM public.payment_plan_templates
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetPaymentPlanTemplate :one
SELECT * FROM public.payment_plan_templates WHERE id = $1;

-- name: CreatePaymentPlanTemplate :one
INSERT INTO public.payment_plan_templates (
    business_entity_id, project_id, code, name, status,
    frequency_type, installment_count, generation_rule_json, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- =============================================================================
-- Installment Schedule Lines
-- =============================================================================

-- name: ListInstallmentScheduleLines :many
SELECT * FROM public.installment_schedule_lines
WHERE sales_contract_id = $1
ORDER BY line_no ASC;

-- name: GetInstallmentScheduleLine :one
SELECT * FROM public.installment_schedule_lines WHERE id = $1;

-- name: CreateInstallmentScheduleLine :one
INSERT INTO public.installment_schedule_lines (
    sales_contract_id, receivable_id, line_no, due_date, line_type,
    description, principal_amount, penalty_amount_accrued,
    discount_amount_applied, amount_paid, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateInstallmentScheduleLine :one
UPDATE public.installment_schedule_lines SET
    due_date = COALESCE(sqlc.narg('due_date')::date, due_date),
    description = COALESCE(sqlc.narg('description')::text, description),
    principal_amount = COALESCE(sqlc.narg('principal_amount')::numeric, principal_amount),
    penalty_amount_accrued = COALESCE(sqlc.narg('penalty_amount_accrued')::numeric, penalty_amount_accrued),
    discount_amount_applied = COALESCE(sqlc.narg('discount_amount_applied')::numeric, discount_amount_applied),
    status = COALESCE(sqlc.narg('status')::text, status),
    updated_at = timezone('utc', now())
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: LinkScheduleLineReceivable :one
UPDATE public.installment_schedule_lines
SET receivable_id = $2, updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- name: GetNextScheduleLineNumber :one
SELECT COALESCE(MAX(line_no), 0)::smallint + 1 AS next_line_no
FROM public.installment_schedule_lines
WHERE sales_contract_id = $1;

-- =============================================================================
-- Sales Contract Parties
-- =============================================================================

-- name: ListSalesContractParties :many
SELECT * FROM public.sales_contract_parties
WHERE sales_contract_id = $1
ORDER BY is_primary DESC, effective_from DESC;

-- name: CreateSalesContractParty :one
INSERT INTO public.sales_contract_parties (
    sales_contract_id, party_id, role, is_primary, effective_from, effective_to, status
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CloseSalesContractParty :one
UPDATE public.sales_contract_parties
SET effective_to = $2, status = 'inactive', updated_at = timezone('utc', now())
WHERE id = $1 AND effective_to IS NULL
RETURNING *;

-- =============================================================================
-- Ownership Transfers
-- =============================================================================

-- name: CreateOwnershipTransfer :one
INSERT INTO public.ownership_transfers (
    business_entity_id, branch_id, project_id, unit_id, sales_contract_id,
    approval_request_id, transfer_type, from_party_id, to_party_id,
    effective_date, financial_treatment, transfer_fee_amount,
    transfer_fee_currency, notes, status, requested_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetOwnershipTransfer :one
SELECT * FROM public.ownership_transfers WHERE id = $1;

-- name: GetOwnershipTransferByApprovalRequest :one
SELECT * FROM public.ownership_transfers WHERE approval_request_id = $1;

-- name: UpdateOwnershipTransferStatus :one
UPDATE public.ownership_transfers
SET status = $2, updated_at = timezone('utc', now())
WHERE id = $1 AND status = $3
RETURNING *;

-- name: SetOwnershipTransferCompletion :one
UPDATE public.ownership_transfers
SET status = 'completed',
    completed_by_user_id = $2,
    updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;
