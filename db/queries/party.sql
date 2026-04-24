-- =============================================================================
-- Party Domain Queries
-- =============================================================================

-- =============================================================================
-- Parties
-- =============================================================================

-- name: ListParties :many
SELECT * FROM public.parties
WHERE (sqlc.narg('party_type')::text IS NULL OR party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY display_name ASC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountParties :one
SELECT COUNT(*)::bigint FROM public.parties
WHERE (sqlc.narg('party_type')::text IS NULL OR party_type = sqlc.narg('party_type')::text)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetParty :one
SELECT * FROM public.parties WHERE id = $1;

-- name: CreateParty :one
INSERT INTO public.parties (
    party_type,
    party_code,
    display_name,
    full_name,
    first_name,
    middle_name,
    last_name,
    organization_name,
    primary_phone,
    secondary_phone,
    primary_email,
    date_of_birth,
    nationality,
    national_id_no,
    passport_no,
    registration_no,
    tax_no,
    preferred_language,
    legacy_ref,
    notes,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
) RETURNING *;

-- name: UpdateParty :one
UPDATE public.parties SET
    party_type = COALESCE(sqlc.narg('party_type')::text, party_type),
    display_name = COALESCE(sqlc.narg('display_name')::text, display_name),
    full_name = COALESCE(sqlc.narg('full_name')::text, full_name),
    first_name = COALESCE(sqlc.narg('first_name')::text, first_name),
    middle_name = COALESCE(sqlc.narg('middle_name')::text, middle_name),
    last_name = COALESCE(sqlc.narg('last_name')::text, last_name),
    organization_name = COALESCE(sqlc.narg('organization_name')::text, organization_name),
    primary_phone = COALESCE(sqlc.narg('primary_phone')::text, primary_phone),
    secondary_phone = COALESCE(sqlc.narg('secondary_phone')::text, secondary_phone),
    primary_email = COALESCE(sqlc.narg('primary_email')::text, primary_email),
    date_of_birth = COALESCE(sqlc.narg('date_of_birth')::date, date_of_birth),
    nationality = COALESCE(sqlc.narg('nationality')::text, nationality),
    national_id_no = COALESCE(sqlc.narg('national_id_no')::text, national_id_no),
    passport_no = COALESCE(sqlc.narg('passport_no')::text, passport_no),
    registration_no = COALESCE(sqlc.narg('registration_no')::text, registration_no),
    tax_no = COALESCE(sqlc.narg('tax_no')::text, tax_no),
    preferred_language = COALESCE(sqlc.narg('preferred_language')::text, preferred_language),
    legacy_ref = COALESCE(sqlc.narg('legacy_ref')::text, legacy_ref),
    notes = COALESCE(sqlc.narg('notes')::text, notes),
    status = COALESCE(sqlc.narg('status')::text, status)
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- =============================================================================
-- Unit Ownerships
-- =============================================================================

-- name: ListUnitOwnerships :many
SELECT * FROM public.unit_ownerships
WHERE unit_id = sqlc.arg('unit_id')::uuid
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY effective_from DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountUnitOwnerships :one
SELECT COUNT(*)::bigint FROM public.unit_ownerships
WHERE unit_id = sqlc.arg('unit_id')::uuid
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: CreateUnitOwnership :one
INSERT INTO public.unit_ownerships (
    unit_id,
    party_id,
    share_percentage,
    effective_from,
    effective_to,
    status,
    notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: CloseUnitOwnership :one
UPDATE public.unit_ownerships SET
    effective_to = sqlc.arg('effective_to')::date,
    status = 'inactive'
WHERE id = $1
RETURNING *;

-- name: GetActiveUnitOwnershipForParty :one
SELECT * FROM public.unit_ownerships
WHERE unit_id = $1
  AND party_id = $2
  AND status = 'active'
  AND effective_to IS NULL
LIMIT 1;

-- =============================================================================
-- Responsibility Assignments
-- =============================================================================

-- name: ListResponsibilityAssignments :many
SELECT * FROM public.responsibility_assignments
WHERE unit_id = sqlc.arg('unit_id')::uuid
  AND (sqlc.narg('responsibility_type')::text IS NULL OR responsibility_type = sqlc.narg('responsibility_type')::text)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY effective_from DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountResponsibilityAssignments :one
SELECT COUNT(*)::bigint FROM public.responsibility_assignments
WHERE unit_id = sqlc.arg('unit_id')::uuid
  AND (sqlc.narg('responsibility_type')::text IS NULL OR responsibility_type = sqlc.narg('responsibility_type')::text)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetActiveResponsibilityAssignment :one
SELECT * FROM public.responsibility_assignments
WHERE unit_id = $1
  AND responsibility_type = $2
  AND effective_from <= $3
  AND (effective_to IS NULL OR effective_to > $3)
  AND status = 'active'
ORDER BY effective_from DESC
LIMIT 1;

-- name: CreateResponsibilityAssignment :one
INSERT INTO public.responsibility_assignments (
    unit_id,
    party_id,
    responsibility_type,
    effective_from,
    effective_to,
    status,
    notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: CloseResponsibilityAssignment :one
UPDATE public.responsibility_assignments SET
    effective_to = sqlc.arg('effective_to')::date,
    status = 'inactive'
WHERE id = $1
RETURNING *;
