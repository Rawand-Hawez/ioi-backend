-- backend/db/queries/finance.sql

-- =============================================================================
-- Shared Finance Domain Queries
-- =============================================================================

-- =============================================================================
-- Receivables
-- =============================================================================

-- name: ListReceivables :many
SELECT * FROM public.receivables
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_module')::text IS NULL OR source_module = sqlc.narg('source_module')::text)
ORDER BY due_date ASC
LIMIT $1 OFFSET $2;

-- name: CountReceivables :one
SELECT count(*) FROM public.receivables
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('source_module')::text IS NULL OR source_module = sqlc.narg('source_module')::text);

-- name: GetReceivable :one
SELECT * FROM public.receivables WHERE id = $1;

-- name: CreateReceivable :one
INSERT INTO public.receivables (
    business_entity_id, branch_id, party_id, unit_id,
    source_module, source_record_type, source_record_id,
    receivable_no, receivable_date, due_date,
    currency_code, original_amount, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateReceivablePaidAmount :one
UPDATE public.receivables
SET paid_amount = paid_amount + $2,
    status = CASE
        WHEN (original_amount + adjusted_amount - (paid_amount + $2) - credited_amount) <= 0 THEN 'paid'
        WHEN (paid_amount + $2) > 0 THEN 'partially_paid'
        WHEN credited_amount > 0 THEN 'partially_paid'
        ELSE 'open'
    END,
    updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- name: UpdateReceivableCreditedAmount :one
UPDATE public.receivables
SET credited_amount = credited_amount + $2,
    status = CASE
        WHEN (original_amount + adjusted_amount - paid_amount - (credited_amount + $2)) <= 0 THEN 'paid'
        WHEN (credited_amount + $2) > 0 OR paid_amount > 0 THEN 'partially_paid'
        ELSE status
    END,
    updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- name: UpdateReceivableAdjustedAmount :one
UPDATE public.receivables
SET adjusted_amount = adjusted_amount + $2,
    updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- name: VoidReceivable :one
UPDATE public.receivables
SET status = 'voided',
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'open'
RETURNING *;

-- =============================================================================
-- Payments
-- =============================================================================

-- name: ListPayments :many
SELECT * FROM public.payments
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY payment_date DESC, created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPayments :one
SELECT count(*) FROM public.payments
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetPayment :one
SELECT * FROM public.payments WHERE id = $1;

-- name: CreatePayment :one
INSERT INTO public.payments (
    business_entity_id, branch_id, party_id,
    payment_no, receipt_no, payment_date, payment_method,
    currency_code, amount_received, unapplied_amount,
    reference_no, notes, received_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11, $12)
RETURNING *;

-- name: PostPayment :one
UPDATE public.payments
SET status = 'posted',
    posted_at = timezone('utc', now()),
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'draft'
RETURNING *;

-- name: VoidPayment :one
UPDATE public.payments
SET status = 'voided',
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'posted'
RETURNING *;

-- name: UpdatePaymentUnapplied :one
UPDATE public.payments
SET unapplied_amount = unapplied_amount - $2,
    updated_at = timezone('utc', now())
WHERE id = $1
RETURNING *;

-- =============================================================================
-- Payment Allocations
-- =============================================================================

-- name: CreatePaymentAllocation :one
INSERT INTO public.payment_allocations (
    payment_id, receivable_id, allocated_amount, allocation_date, allocation_order, notes
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAllocationsForPayment :many
SELECT * FROM public.payment_allocations
WHERE payment_id = $1
ORDER BY allocation_order ASC;

-- name: ListAllocationsForReceivable :many
SELECT * FROM public.payment_allocations
WHERE receivable_id = $1
ORDER BY allocation_date ASC;

-- =============================================================================
-- Credit Balances
-- =============================================================================

-- name: ListCreditBalances :many
SELECT * FROM public.credit_balances
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountCreditBalances :one
SELECT count(*) FROM public.credit_balances
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('party_id')::uuid IS NULL OR party_id = sqlc.narg('party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetCreditBalance :one
SELECT * FROM public.credit_balances WHERE id = $1;

-- name: CreateCreditBalance :one
INSERT INTO public.credit_balances (
    business_entity_id, branch_id, party_id,
    source_module, source_record_type, source_record_id,
    origin_payment_id, origin_adjustment_id,
    currency_code, amount_total, reason
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ApplyCreditBalance :one
UPDATE public.credit_balances
SET amount_used = amount_used + $2,
    status = CASE
        WHEN (amount_total - (amount_used + $2)) <= 0 THEN 'exhausted'
        ELSE status
    END,
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'available'
RETURNING *;

-- =============================================================================
-- Financial Adjustments
-- =============================================================================

-- name: ListFinancialAdjustments :many
SELECT * FROM public.financial_adjustments
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountFinancialAdjustments :one
SELECT count(*) FROM public.financial_adjustments
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetFinancialAdjustment :one
SELECT * FROM public.financial_adjustments WHERE id = $1;

-- name: CreateFinancialAdjustment :one
INSERT INTO public.financial_adjustments (
    business_entity_id, branch_id, party_id,
    source_module, source_record_type, source_record_id,
    adjustment_type, amount, currency_code,
    effective_date, reason, requested_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ApproveAdjustment :one
UPDATE public.financial_adjustments
SET status = 'approved',
    approved_by_user_id = $2,
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: RejectAdjustment :one
UPDATE public.financial_adjustments
SET status = 'rejected',
    approved_by_user_id = $2,
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ApplyAdjustment :one
UPDATE public.financial_adjustments
SET status = 'applied',
    updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'approved'
RETURNING *;

-- =============================================================================
-- Statement Queries
-- =============================================================================

-- name: GetPartyStatement :many
SELECT * FROM public.receivables
WHERE party_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY due_date ASC;

-- name: GetUnitStatement :many
SELECT * FROM public.receivables
WHERE unit_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY due_date ASC;
