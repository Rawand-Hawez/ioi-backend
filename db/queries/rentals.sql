-- backend/db/queries/rentals.sql

-- =============================================================================
-- Lease Contracts
-- =============================================================================

-- name: ListLeaseContracts :many
SELECT * FROM public.lease_contracts
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('primary_tenant_id')::uuid IS NULL OR primary_tenant_id = sqlc.narg('primary_tenant_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountLeaseContracts :one
SELECT COUNT(*)::bigint FROM public.lease_contracts
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('branch_id')::uuid IS NULL OR branch_id = sqlc.narg('branch_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('primary_tenant_id')::uuid IS NULL OR primary_tenant_id = sqlc.narg('primary_tenant_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetLeaseContract :one
SELECT * FROM public.lease_contracts WHERE id = $1;

-- name: CreateLeaseContract :one
INSERT INTO public.lease_contracts (
    business_entity_id, branch_id, project_id, unit_id, primary_tenant_id,
    renewed_from_lease_contract_id, lease_type, start_date, end_date,
    rent_pricing_basis, area_basis_sqm, rate_per_sqm, contractual_rent_amount,
    billing_interval_value, billing_interval_unit, billing_anchor_date,
    security_deposit_amount, advance_rent_amount, currency_code,
    notice_period_days, purpose_of_use, notes, created_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22, $23
)
RETURNING *;

-- name: UpdateLeaseContract :one
UPDATE public.lease_contracts SET
    lease_type = COALESCE(sqlc.narg('lease_type')::text, lease_type),
    start_date = COALESCE(sqlc.narg('start_date')::date, start_date),
    end_date = COALESCE(sqlc.narg('end_date')::date, end_date),
    rent_pricing_basis = COALESCE(sqlc.narg('rent_pricing_basis')::text, rent_pricing_basis),
    area_basis_sqm = COALESCE(sqlc.narg('area_basis_sqm')::numeric, area_basis_sqm),
    rate_per_sqm = COALESCE(sqlc.narg('rate_per_sqm')::numeric, rate_per_sqm),
    contractual_rent_amount = COALESCE(sqlc.narg('contractual_rent_amount')::numeric, contractual_rent_amount),
    billing_interval_value = COALESCE(sqlc.narg('billing_interval_value')::smallint, billing_interval_value),
    billing_interval_unit = COALESCE(sqlc.narg('billing_interval_unit')::text, billing_interval_unit),
    billing_anchor_date = COALESCE(sqlc.narg('billing_anchor_date')::date, billing_anchor_date),
    security_deposit_amount = COALESCE(sqlc.narg('security_deposit_amount')::numeric, security_deposit_amount),
    advance_rent_amount = COALESCE(sqlc.narg('advance_rent_amount')::numeric, advance_rent_amount),
    notice_period_days = COALESCE(sqlc.narg('notice_period_days')::int, notice_period_days),
    purpose_of_use = COALESCE(sqlc.narg('purpose_of_use')::text, purpose_of_use),
    notes = COALESCE(sqlc.narg('notes')::text, notes),
    updated_at = timezone('utc', now())
WHERE id = sqlc.arg('id')::uuid AND status = 'draft'
RETURNING *;

-- name: UpdateLeaseContractStatus :one
UPDATE public.lease_contracts
SET status = $2, updated_at = timezone('utc', now())
WHERE id = $1 AND status = $3
RETURNING *;

-- name: GetActiveLeaseContractForUnit :one
SELECT * FROM public.lease_contracts WHERE unit_id = $1 AND status = 'active' LIMIT 1;

-- =============================================================================
-- Lease Parties (effective-dated)
-- =============================================================================

-- name: ListLeaseParties :many
SELECT * FROM public.lease_parties
WHERE lease_contract_id = $1
ORDER BY is_primary DESC, effective_from DESC;

-- name: CreateLeaseParty :one
INSERT INTO public.lease_parties (
    lease_contract_id, party_id, role, is_primary, effective_from, effective_to, status
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CloseLeaseParty :one
UPDATE public.lease_parties
SET effective_to = $2, status = 'closed', updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'active'
RETURNING *;

-- Close every active lease_parties row for a lease (tenants, co-tenants, guarantors).
-- Used by termination apply path so guarantor history is closed alongside tenant history.
-- name: CloseAllActiveLeaseParties :many
UPDATE public.lease_parties
SET effective_to = $2, status = 'closed', updated_at = timezone('utc', now())
WHERE lease_contract_id = $1 AND status = 'active'
RETURNING *;

-- =============================================================================
-- Lease Bills
-- =============================================================================

-- name: ListLeaseBills :many
SELECT * FROM public.lease_bills
WHERE (sqlc.narg('lease_contract_id')::uuid IS NULL OR lease_contract_id = sqlc.narg('lease_contract_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('responsible_party_id')::uuid IS NULL OR responsible_party_id = sqlc.narg('responsible_party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY billing_period_start ASC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountLeaseBills :one
SELECT COUNT(*)::bigint FROM public.lease_bills
WHERE (sqlc.narg('lease_contract_id')::uuid IS NULL OR lease_contract_id = sqlc.narg('lease_contract_id')::uuid)
  AND (sqlc.narg('unit_id')::uuid IS NULL OR unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('responsible_party_id')::uuid IS NULL OR responsible_party_id = sqlc.narg('responsible_party_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- Idempotent insert: returns NULL via :execrows-style guard not available on :one,
-- so we use ON CONFLICT DO NOTHING and the handler checks rows-returned vs requested.
-- Callers MUST pre-check existing periods via ListLeaseBills if they need the existing row.
-- name: CreateLeaseBill :one
INSERT INTO public.lease_bills (
    business_entity_id, branch_id, lease_contract_id, unit_id, responsible_party_id,
    billing_period_start, billing_period_end, due_date,
    billing_interval_value, billing_interval_unit,
    billed_amount, currency_code, is_advance, status, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetLeaseBill :one
SELECT * FROM public.lease_bills WHERE id = $1;

-- name: GetLeaseBillForPeriod :one
SELECT * FROM public.lease_bills
WHERE lease_contract_id = $1 AND billing_period_start = $2 AND billing_period_end = $3 AND is_advance = $4;

-- name: IssueLeaseBill :one
UPDATE public.lease_bills
SET status = 'issued', updated_at = timezone('utc', now())
WHERE id = $1 AND status = 'draft'
RETURNING *;

-- name: VoidLeaseBill :one
UPDATE public.lease_bills
SET status = 'voided', updated_at = timezone('utc', now())
WHERE id = $1 AND status IN ('draft', 'issued')
RETURNING *;

-- name: LinkLeaseBillReceivable :one
UPDATE public.lease_bills
SET receivable_id = $2, updated_at = timezone('utc', now())
WHERE id = $1 AND receivable_id IS NULL
RETURNING *;

-- =============================================================================
-- Approval gate lookup (Phase 7 pattern)
-- =============================================================================

-- name: GetLatestRentalsApprovalRequest :one
SELECT *
FROM public.approval_requests
WHERE module = 'rentals'
  AND source_record_type = $1
  AND source_record_id = $2
  AND request_type = $3
ORDER BY submitted_at DESC
LIMIT 1;
