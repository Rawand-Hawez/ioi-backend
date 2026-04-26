-- =============================================================================
-- Authorization Domain Queries
-- =============================================================================

-- =============================================================================
-- Roles
-- =============================================================================

-- name: ListRoles :many
SELECT * FROM public.roles
WHERE (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY code ASC;

-- name: GetRole :one
SELECT * FROM public.roles WHERE id = $1;

-- =============================================================================
-- Permissions
-- =============================================================================

-- name: ListPermissions :many
SELECT * FROM public.permissions
WHERE (sqlc.narg('module')::text IS NULL OR module = sqlc.narg('module')::text)
ORDER BY key ASC;

-- name: GetPermission :one
SELECT * FROM public.permissions WHERE id = $1;

-- =============================================================================
-- Role Permissions
-- =============================================================================

-- name: ListRolePermissions :many
SELECT p.* FROM public.permissions p
JOIN public.role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.key ASC;

-- =============================================================================
-- User Role Scope Assignments
-- =============================================================================

-- name: ListUserRoleAssignments :many
SELECT * FROM public.user_role_scope_assignments
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetUserRoleAssignment :one
SELECT * FROM public.user_role_scope_assignments WHERE id = $1;

-- name: AssignRoleToUser :one
INSERT INTO public.user_role_scope_assignments (
    user_id, role_id, scope_type, scope_id
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: RemoveRoleFromUser :one
DELETE FROM public.user_role_scope_assignments WHERE id = $1
RETURNING *;

-- =============================================================================
-- Permission Resolution (for middleware + /me/permissions)
-- =============================================================================

-- name: GetUserPermissions :many
-- Returns all distinct permission keys for a user through their role assignments
SELECT DISTINCT p.id, p.key, p.module, p.description, p.created_at, ursa.scope_type, ursa.scope_id
FROM public.user_role_scope_assignments ursa
JOIN public.roles r ON r.id = ursa.role_id
JOIN public.role_permissions rp ON rp.role_id = r.id
JOIN public.permissions p ON p.id = rp.permission_id
WHERE ursa.user_id = $1
  AND r.is_active = true
ORDER BY p.key ASC;

-- name: CheckUserPermission :one
-- Returns true if user has a specific permission through any active role
SELECT EXISTS (
    SELECT 1
    FROM public.user_role_scope_assignments ursa
    JOIN public.roles r ON r.id = ursa.role_id
    JOIN public.role_permissions rp ON rp.role_id = r.id
    JOIN public.permissions p ON p.id = rp.permission_id
    WHERE ursa.user_id = $1
      AND p.key = $2
      AND r.is_active = true
) AS has_permission;

-- name: CheckUserPermissionInScope :one
-- Returns true if user has permission at deployment scope or at the target business_entity/branch/project scope.
SELECT EXISTS (
    SELECT 1
    FROM public.user_role_scope_assignments ursa
    JOIN public.roles r ON r.id = ursa.role_id
    JOIN public.role_permissions rp ON rp.role_id = r.id
    JOIN public.permissions p ON p.id = rp.permission_id
    WHERE ursa.user_id = $1
      AND p.key = $2
      AND r.is_active = true
      AND (
        ursa.scope_type = 'deployment'
        OR (ursa.scope_type = 'business_entity' AND ursa.scope_id = $3)
        OR (ursa.scope_type = 'branch' AND ursa.scope_id = $4)
        OR (ursa.scope_type = 'project' AND ursa.scope_id = $5)
      )
) AS has_permission;
