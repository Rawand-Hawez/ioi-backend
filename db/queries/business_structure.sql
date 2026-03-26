-- backend/db/queries/business_structure.sql

-- name: ListBusinessEntities :many
SELECT * FROM business_entities
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'))
ORDER BY name ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountBusinessEntities :one
SELECT COUNT(*) FROM business_entities
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'));

-- name: GetBusinessEntity :one
SELECT * FROM business_entities WHERE id = $1;

-- name: GetBusinessEntityByCode :one
SELECT * FROM business_entities WHERE code = $1;

-- name: CreateBusinessEntity :one
INSERT INTO business_entities (
    code, name, display_name, default_currency, country,
    registration_no, tax_no, status, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: UpdateBusinessEntity :one
UPDATE business_entities SET
    code = COALESCE(sqlc.narg('code')::text, code),
    name = COALESCE(sqlc.narg('name')::text, name),
    display_name = COALESCE(sqlc.narg('display_name')::text, display_name),
    default_currency = COALESCE(sqlc.narg('default_currency')::text, default_currency),
    country = COALESCE(sqlc.narg('country')::text, country),
    registration_no = COALESCE(sqlc.narg('registration_no')::text, registration_no),
    tax_no = COALESCE(sqlc.narg('tax_no')::text, tax_no),
    status = COALESCE(sqlc.narg('status')::text, status),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active),
    notes = COALESCE(sqlc.narg('notes')::text, notes)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeactivateBusinessEntity :exec
UPDATE business_entities SET is_active = false WHERE id = $1;

-- ============================================================================
-- BRANCH
-- ============================================================================

-- name: ListBranches :many
SELECT * FROM branches
WHERE business_entity_id = $1
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'))
ORDER BY name ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountBranches :one
SELECT COUNT(*) FROM branches
WHERE business_entity_id = $1
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active'));

-- name: GetBranch :one
SELECT * FROM branches WHERE id = $1;

-- name: GetBranchByCode :one
SELECT * FROM branches WHERE business_entity_id = $1 AND code = $2;

-- name: CreateBranch :one
INSERT INTO branches (
    business_entity_id, code, name, display_name, country, city,
    address_text, phone, email, status, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: UpdateBranch :one
UPDATE branches SET
    code = COALESCE(sqlc.narg('code')::text, code),
    name = COALESCE(sqlc.narg('name')::text, name),
    display_name = COALESCE(sqlc.narg('display_name')::text, display_name),
    country = COALESCE(sqlc.narg('country')::text, country),
    city = COALESCE(sqlc.narg('city')::text, city),
    address_text = COALESCE(sqlc.narg('address_text')::text, address_text),
    phone = COALESCE(sqlc.narg('phone')::text, phone),
    email = COALESCE(sqlc.narg('email')::text, email),
    status = COALESCE(sqlc.narg('status')::text, status),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active),
    notes = COALESCE(sqlc.narg('notes')::text, notes)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeactivateBranch :exec
UPDATE branches SET is_active = false WHERE id = $1;

-- name: CountBranchesByBusinessEntity :one
SELECT COUNT(*) FROM branches WHERE business_entity_id = $1 AND is_active = true;
