-- =============================================================================
-- Inventory Domain
-- =============================================================================
-- Entities: projects, structure_nodes, units
-- Dependencies: business_entities, branches
-- =============================================================================

-- =============================================================================
-- 1. Tables
-- =============================================================================

-- Project: A real estate development or portfolio under a business entity
CREATE TABLE IF NOT EXISTS public.projects (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    business_entity_id UUID NOT NULL REFERENCES public.business_entities(id) ON DELETE RESTRICT,
    primary_branch_id UUID NOT NULL REFERENCES public.branches(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT,
    project_type TEXT NOT NULL,
    structure_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    country TEXT NOT NULL DEFAULT 'IQ',
    city TEXT,
    district_area TEXT,
    address_text TEXT,
    default_currency TEXT NOT NULL DEFAULT 'USD',
    launch_date DATE,
    handover_date DATE,
    notes TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    CONSTRAINT projects_business_entity_code_key UNIQUE (business_entity_id, code),
    CONSTRAINT projects_project_type_check CHECK (project_type IN ('residential', 'commercial', 'mixed', 'managed')),
    CONSTRAINT projects_structure_type_check CHECK (structure_type IN ('tower', 'compound', 'villa_community', 'mixed', 'flat')),
    CONSTRAINT projects_status_check CHECK (status IN ('active', 'inactive'))
);

COMMENT ON COLUMN public.projects.project_type IS 'residential, commercial, mixed, managed';
COMMENT ON COLUMN public.projects.structure_type IS 'tower, compound, villa_community, mixed, flat';

-- Structure Node: Hierarchical organization within a project (building, tower, floor, block, cluster)
CREATE TABLE IF NOT EXISTS public.structure_nodes (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES public.projects(id) ON DELETE RESTRICT,
    parent_structure_node_id UUID REFERENCES public.structure_nodes(id) ON DELETE RESTRICT,
    node_type TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    is_active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    CONSTRAINT structure_nodes_project_code_key UNIQUE (project_id, code),
    CONSTRAINT structure_nodes_node_type_check CHECK (node_type IN ('building', 'tower', 'block', 'cluster', 'floor')),
    CONSTRAINT structure_nodes_status_check CHECK (status IN ('active', 'inactive'))
);

COMMENT ON COLUMN public.structure_nodes.node_type IS 'building, tower, block, cluster, floor';

-- Unit: Individual saleable/rentable unit within a project
CREATE TABLE IF NOT EXISTS public.units (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    business_entity_id UUID NOT NULL REFERENCES public.business_entities(id) ON DELETE RESTRICT,
    branch_id UUID NOT NULL REFERENCES public.branches(id) ON DELETE RESTRICT,
    project_id UUID NOT NULL REFERENCES public.projects(id) ON DELETE RESTRICT,
    structure_node_id UUID REFERENCES public.structure_nodes(id) ON DELETE RESTRICT,
    parent_unit_id UUID REFERENCES public.units(id) ON DELETE RESTRICT,
    unit_type TEXT NOT NULL,
    commercial_disposition TEXT NOT NULL DEFAULT 'sale_or_rent',
    unit_code TEXT NOT NULL,
    display_code TEXT,
    unit_no TEXT,
    floor_value TEXT,
    floor_sort_value INTEGER,
    section_no TEXT,
    entrance_no TEXT,
    bedroom_count SMALLINT,
    bathroom_count SMALLINT,
    area_gross_sqm NUMERIC(10,2),
    area_net_sqm NUMERIC(10,2),
    area_chargeable_sqm NUMERIC(10,2),
    land_area_sqm NUMERIC(10,2),
    facing_direction TEXT,
    inventory_status TEXT NOT NULL DEFAULT 'available',
    sales_status TEXT NOT NULL DEFAULT 'available',
    occupancy_status TEXT NOT NULL DEFAULT 'vacant',
    maintenance_status TEXT NOT NULL DEFAULT 'none',
    list_price_amount NUMERIC(18,2),
    list_price_currency TEXT NOT NULL DEFAULT 'USD',
    valuation_amount NUMERIC(18,2),
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata_json JSONB,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    CONSTRAINT units_project_unit_code_key UNIQUE (project_id, unit_code),
    CONSTRAINT units_unit_type_check CHECK (unit_type IN ('apartment', 'villa', 'retail', 'office', 'parking', 'storage', 'townhouse')),
    CONSTRAINT units_commercial_disposition_check CHECK (commercial_disposition IN ('sale_only', 'rent_only', 'sale_or_rent', 'internal_use', 'inactive')),
    CONSTRAINT units_inventory_status_check CHECK (inventory_status IN ('available', 'reserved', 'sold', 'leased', 'owner_occupied', 'inactive')),
    CONSTRAINT units_sales_status_check CHECK (sales_status IN ('available', 'reserved', 'sold', 'not_for_sale')),
    CONSTRAINT units_occupancy_status_check CHECK (occupancy_status IN ('vacant', 'occupied', 'owner_occupied')),
    CONSTRAINT units_maintenance_status_check CHECK (maintenance_status IN ('none', 'under_maintenance', 'restricted'))
);

COMMENT ON COLUMN public.units.unit_type IS 'apartment, villa, retail, office, parking, storage, townhouse';
COMMENT ON COLUMN public.units.commercial_disposition IS 'sale_only, rent_only, sale_or_rent, internal_use, inactive';
COMMENT ON COLUMN public.units.inventory_status IS 'available, reserved, sold, leased, owner_occupied, inactive';
COMMENT ON COLUMN public.units.sales_status IS 'available, reserved, sold, not_for_sale';
COMMENT ON COLUMN public.units.occupancy_status IS 'vacant, occupied, owner_occupied';
COMMENT ON COLUMN public.units.maintenance_status IS 'none, under_maintenance, restricted';

-- =============================================================================
-- 2. Row Level Security
-- =============================================================================

ALTER TABLE public.projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.structure_nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.units ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON public.projects TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.structure_nodes TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.units TO authenticated;

-- Projects: all authenticated users can view and manage
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can view projects' AND tablename = 'projects') THEN
        CREATE POLICY "Authenticated users can view projects" ON public.projects FOR SELECT TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can insert projects' AND tablename = 'projects') THEN
        CREATE POLICY "Authenticated users can insert projects" ON public.projects FOR INSERT TO authenticated WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can update projects' AND tablename = 'projects') THEN
        CREATE POLICY "Authenticated users can update projects" ON public.projects FOR UPDATE TO authenticated USING (true);
    END IF;
END $$;

-- Structure nodes: all authenticated users can view and manage
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can view structure nodes' AND tablename = 'structure_nodes') THEN
        CREATE POLICY "Authenticated users can view structure nodes" ON public.structure_nodes FOR SELECT TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can insert structure nodes' AND tablename = 'structure_nodes') THEN
        CREATE POLICY "Authenticated users can insert structure nodes" ON public.structure_nodes FOR INSERT TO authenticated WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can update structure nodes' AND tablename = 'structure_nodes') THEN
        CREATE POLICY "Authenticated users can update structure nodes" ON public.structure_nodes FOR UPDATE TO authenticated USING (true);
    END IF;
END $$;

-- Units: all authenticated users can view and manage
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can view units' AND tablename = 'units') THEN
        CREATE POLICY "Authenticated users can view units" ON public.units FOR SELECT TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can insert units' AND tablename = 'units') THEN
        CREATE POLICY "Authenticated users can insert units" ON public.units FOR INSERT TO authenticated WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can update units' AND tablename = 'units') THEN
        CREATE POLICY "Authenticated users can update units" ON public.units FOR UPDATE TO authenticated USING (true);
    END IF;
END $$;

-- =============================================================================
-- 3. Indexes
-- =============================================================================

-- Projects indexes
-- Note: business_entity_id is leading column of UNIQUE constraint, no separate index needed
CREATE INDEX IF NOT EXISTS idx_projects_primary_branch_id ON public.projects(primary_branch_id);
CREATE INDEX IF NOT EXISTS idx_projects_is_active ON public.projects(is_active);
CREATE INDEX IF NOT EXISTS idx_projects_status ON public.projects(status);

-- Structure nodes indexes
-- Note: project_id is leading column of UNIQUE constraint, no separate index needed
CREATE INDEX IF NOT EXISTS idx_structure_nodes_parent_structure_node_id ON public.structure_nodes(parent_structure_node_id) WHERE parent_structure_node_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_structure_nodes_is_active ON public.structure_nodes(is_active);
CREATE INDEX IF NOT EXISTS idx_structure_nodes_status ON public.structure_nodes(status);

-- Units indexes
-- Note: project_id is leading column of UNIQUE constraint, no separate index needed
CREATE INDEX IF NOT EXISTS idx_units_business_entity_id ON public.units(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_units_branch_id ON public.units(branch_id);
CREATE INDEX IF NOT EXISTS idx_units_structure_node_id ON public.units(structure_node_id) WHERE structure_node_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_units_parent_unit_id ON public.units(parent_unit_id) WHERE parent_unit_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_units_is_active ON public.units(is_active);
CREATE INDEX IF NOT EXISTS idx_units_inventory_status ON public.units(inventory_status);
CREATE INDEX IF NOT EXISTS idx_units_sales_status ON public.units(sales_status);
CREATE INDEX IF NOT EXISTS idx_units_occupancy_status ON public.units(occupancy_status);

-- =============================================================================
-- 4. Triggers
-- =============================================================================

-- Updated_at trigger for projects
CREATE OR REPLACE FUNCTION update_projects_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'projects_updated_at') THEN
        CREATE TRIGGER projects_updated_at
            BEFORE UPDATE ON public.projects
            FOR EACH ROW EXECUTE FUNCTION update_projects_updated_at();
    END IF;
END $$;

-- Updated_at trigger for structure_nodes
CREATE OR REPLACE FUNCTION update_structure_nodes_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'structure_nodes_updated_at') THEN
        CREATE TRIGGER structure_nodes_updated_at
            BEFORE UPDATE ON public.structure_nodes
            FOR EACH ROW EXECUTE FUNCTION update_structure_nodes_updated_at();
    END IF;
END $$;

-- Updated_at trigger for units
CREATE OR REPLACE FUNCTION update_units_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'units_updated_at') THEN
        CREATE TRIGGER units_updated_at
            BEFORE UPDATE ON public.units
            FOR EACH ROW EXECUTE FUNCTION update_units_updated_at();
    END IF;
END $$;

-- Realtime notifications for units (per backend-plan.md Section 5)
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'units_realtime') THEN
        CREATE TRIGGER units_realtime
            AFTER INSERT OR UPDATE OR DELETE ON public.units
            FOR EACH ROW EXECUTE FUNCTION notify_realtime();
    END IF;
END $$;

-- =============================================================================
-- 5. Notify PostgREST to reload schema
-- =============================================================================
NOTIFY pgrst, 'reload schema';
