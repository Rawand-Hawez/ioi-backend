-- =============================================================================
-- Inventory Domain Queries
-- =============================================================================

-- =============================================================================
-- Projects
-- =============================================================================

-- name: ListProjects :many
SELECT * FROM public.projects
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY name ASC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountProjects :one
SELECT COUNT(*)::bigint FROM public.projects
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetProject :one
SELECT * FROM public.projects WHERE id = $1;

-- name: CreateProject :one
INSERT INTO public.projects (
    business_entity_id,
    primary_branch_id,
    code,
    name,
    display_name,
    project_type,
    structure_type,
    status,
    country,
    city,
    district_area,
    address_text,
    default_currency,
    launch_date,
    handover_date,
    notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
) RETURNING *;

-- name: UpdateProject :one
UPDATE public.projects SET
    primary_branch_id = COALESCE(sqlc.narg('primary_branch_id')::uuid, primary_branch_id),
    code = COALESCE(sqlc.narg('code')::text, code),
    name = COALESCE(sqlc.narg('name')::text, name),
    display_name = COALESCE(sqlc.narg('display_name')::text, display_name),
    project_type = COALESCE(sqlc.narg('project_type')::text, project_type),
    structure_type = COALESCE(sqlc.narg('structure_type')::text, structure_type),
    status = COALESCE(sqlc.narg('status')::text, status),
    country = COALESCE(sqlc.narg('country')::text, country),
    city = COALESCE(sqlc.narg('city')::text, city),
    district_area = COALESCE(sqlc.narg('district_area')::text, district_area),
    address_text = COALESCE(sqlc.narg('address_text')::text, address_text),
    default_currency = COALESCE(sqlc.narg('default_currency')::text, default_currency),
    launch_date = COALESCE(sqlc.narg('launch_date')::date, launch_date),
    handover_date = COALESCE(sqlc.narg('handover_date')::date, handover_date),
    notes = COALESCE(sqlc.narg('notes')::text, notes),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active)
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: DeactivateProject :exec
UPDATE public.projects SET is_active = false WHERE id = $1;

-- =============================================================================
-- Structure Nodes
-- =============================================================================

-- name: ListStructureNodes :many
SELECT * FROM public.structure_nodes
WHERE project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('parent_structure_node_id')::uuid IS NULL OR parent_structure_node_id = sqlc.narg('parent_structure_node_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY display_order ASC, code ASC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountStructureNodes :one
SELECT COUNT(*)::bigint FROM public.structure_nodes
WHERE project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('parent_structure_node_id')::uuid IS NULL OR parent_structure_node_id = sqlc.narg('parent_structure_node_id')::uuid)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: GetStructureNode :one
SELECT * FROM public.structure_nodes WHERE id = $1;

-- name: CreateStructureNode :one
INSERT INTO public.structure_nodes (
    project_id,
    parent_structure_node_id,
    node_type,
    code,
    name,
    display_order,
    status,
    notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: UpdateStructureNode :one
UPDATE public.structure_nodes SET
    parent_structure_node_id = COALESCE(sqlc.narg('parent_structure_node_id')::uuid, parent_structure_node_id),
    node_type = COALESCE(sqlc.narg('node_type')::text, node_type),
    name = COALESCE(sqlc.narg('name')::text, name),
    display_order = COALESCE(sqlc.narg('display_order')::int, display_order),
    status = COALESCE(sqlc.narg('status')::text, status),
    notes = COALESCE(sqlc.narg('notes')::text, notes),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active)
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: DeactivateStructureNode :exec
UPDATE public.structure_nodes SET is_active = false WHERE id = $1;

-- =============================================================================
-- Units
-- =============================================================================

-- name: ListUnits :many
SELECT * FROM public.units
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('structure_node_id')::uuid IS NULL OR structure_node_id = sqlc.narg('structure_node_id')::uuid)
  AND (sqlc.narg('inventory_status')::text IS NULL OR inventory_status = sqlc.narg('inventory_status')::text)
  AND (sqlc.narg('sales_status')::text IS NULL OR sales_status = sqlc.narg('sales_status')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY unit_code ASC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: CountUnits :one
SELECT COUNT(*)::bigint FROM public.units
WHERE (sqlc.narg('business_entity_id')::uuid IS NULL OR business_entity_id = sqlc.narg('business_entity_id')::uuid)
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('structure_node_id')::uuid IS NULL OR structure_node_id = sqlc.narg('structure_node_id')::uuid)
  AND (sqlc.narg('inventory_status')::text IS NULL OR inventory_status = sqlc.narg('inventory_status')::text)
  AND (sqlc.narg('sales_status')::text IS NULL OR sales_status = sqlc.narg('sales_status')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: GetUnit :one
SELECT * FROM public.units WHERE id = $1;

-- name: CreateUnit :one
INSERT INTO public.units (
    business_entity_id,
    branch_id,
    project_id,
    structure_node_id,
    parent_unit_id,
    unit_type,
    commercial_disposition,
    unit_code,
    display_code,
    unit_no,
    floor_value,
    floor_sort_value,
    section_no,
    entrance_no,
    bedroom_count,
    bathroom_count,
    area_gross_sqm,
    area_net_sqm,
    area_chargeable_sqm,
    land_area_sqm,
    facing_direction,
    inventory_status,
    sales_status,
    occupancy_status,
    maintenance_status,
    list_price_amount,
    list_price_currency,
    valuation_amount,
    metadata_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
) RETURNING *;

-- name: UpdateUnit :one
UPDATE public.units SET
    structure_node_id = COALESCE(sqlc.narg('structure_node_id')::uuid, structure_node_id),
    parent_unit_id = COALESCE(sqlc.narg('parent_unit_id')::uuid, parent_unit_id),
    unit_type = COALESCE(sqlc.narg('unit_type')::text, unit_type),
    commercial_disposition = COALESCE(sqlc.narg('commercial_disposition')::text, commercial_disposition),
    display_code = COALESCE(sqlc.narg('display_code')::text, display_code),
    unit_no = COALESCE(sqlc.narg('unit_no')::text, unit_no),
    floor_value = COALESCE(sqlc.narg('floor_value')::text, floor_value),
    floor_sort_value = COALESCE(sqlc.narg('floor_sort_value')::int, floor_sort_value),
    section_no = COALESCE(sqlc.narg('section_no')::text, section_no),
    entrance_no = COALESCE(sqlc.narg('entrance_no')::text, entrance_no),
    bedroom_count = COALESCE(sqlc.narg('bedroom_count')::smallint, bedroom_count),
    bathroom_count = COALESCE(sqlc.narg('bathroom_count')::smallint, bathroom_count),
    area_gross_sqm = COALESCE(sqlc.narg('area_gross_sqm')::numeric, area_gross_sqm),
    area_net_sqm = COALESCE(sqlc.narg('area_net_sqm')::numeric, area_net_sqm),
    area_chargeable_sqm = COALESCE(sqlc.narg('area_chargeable_sqm')::numeric, area_chargeable_sqm),
    land_area_sqm = COALESCE(sqlc.narg('land_area_sqm')::numeric, land_area_sqm),
    facing_direction = COALESCE(sqlc.narg('facing_direction')::text, facing_direction),
    inventory_status = COALESCE(sqlc.narg('inventory_status')::text, inventory_status),
    sales_status = COALESCE(sqlc.narg('sales_status')::text, sales_status),
    occupancy_status = COALESCE(sqlc.narg('occupancy_status')::text, occupancy_status),
    maintenance_status = COALESCE(sqlc.narg('maintenance_status')::text, maintenance_status),
    list_price_amount = COALESCE(sqlc.narg('list_price_amount')::numeric, list_price_amount),
    list_price_currency = COALESCE(sqlc.narg('list_price_currency')::text, list_price_currency),
    valuation_amount = COALESCE(sqlc.narg('valuation_amount')::numeric, valuation_amount),
    metadata_json = COALESCE(sqlc.narg('metadata_json')::jsonb, metadata_json),
    is_active = COALESCE(sqlc.narg('is_active')::boolean, is_active)
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: UpdateUnitCode :one
-- Permission: inventory.unit.edit_code (per technical-specs.md Section 12.2)
UPDATE public.units SET
    unit_code = sqlc.arg('unit_code')::text,
    display_code = COALESCE(sqlc.narg('display_code')::text, display_code)
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: UpdateUnitInventoryStatus :exec
UPDATE public.units SET inventory_status = $2 WHERE id = $1;

-- name: UpdateUnitOccupancyStatus :exec
UPDATE public.units SET occupancy_status = $2 WHERE id = $1;

-- name: DeactivateUnit :exec
UPDATE public.units SET is_active = false WHERE id = $1;
