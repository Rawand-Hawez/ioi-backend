-- =============================================================================
-- Business Structure Domain
-- =============================================================================
-- Entities: business_entity, branch
-- Dependencies: None (foundation domain)
-- =============================================================================

-- =============================================================================
-- 1. Tables
-- =============================================================================

-- Business Entity: Legal/accounting boundary within the deployment
CREATE TABLE IF NOT EXISTS public.business_entities (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT,
    default_currency TEXT NOT NULL DEFAULT 'USD',
    country TEXT,
    registration_no TEXT,
    tax_no TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    is_active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    CONSTRAINT business_entities_code_key UNIQUE (code),
    CONSTRAINT business_entities_status_check CHECK (status IN ('active', 'inactive'))
);

-- Branch: Operational branch or office under one business entity
CREATE TABLE IF NOT EXISTS public.branches (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    business_entity_id UUID NOT NULL REFERENCES public.business_entities(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT,
    country TEXT,
    city TEXT,
    address_text TEXT,
    phone TEXT,
    email TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    is_active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    CONSTRAINT branches_business_entity_code_key UNIQUE (business_entity_id, code),
    CONSTRAINT branches_status_check CHECK (status IN ('active', 'inactive'))
);

-- =============================================================================
-- 2. Row Level Security
-- =============================================================================

ALTER TABLE public.business_entities ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.branches ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE, DELETE ON public.business_entities TO authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.branches TO authenticated;

-- Business entities: all authenticated users can view, admins manage
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can view business entities' AND tablename = 'business_entities') THEN
        CREATE POLICY "Authenticated users can view business entities" ON public.business_entities FOR SELECT TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can insert business entities' AND tablename = 'business_entities') THEN
        CREATE POLICY "Authenticated users can insert business entities" ON public.business_entities FOR INSERT TO authenticated WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can update business entities' AND tablename = 'business_entities') THEN
        CREATE POLICY "Authenticated users can update business entities" ON public.business_entities FOR UPDATE TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can delete business entities' AND tablename = 'business_entities') THEN
        CREATE POLICY "Authenticated users can delete business entities" ON public.business_entities FOR DELETE TO authenticated USING (is_active = false);
    END IF;
END $$;

-- Branches: all authenticated users can view, scoped by business entity
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can view branches' AND tablename = 'branches') THEN
        CREATE POLICY "Authenticated users can view branches" ON public.branches FOR SELECT TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can insert branches' AND tablename = 'branches') THEN
        CREATE POLICY "Authenticated users can insert branches" ON public.branches FOR INSERT TO authenticated WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can update branches' AND tablename = 'branches') THEN
        CREATE POLICY "Authenticated users can update branches" ON public.branches FOR UPDATE TO authenticated USING (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Authenticated users can delete branches' AND tablename = 'branches') THEN
        CREATE POLICY "Authenticated users can delete branches" ON public.branches FOR DELETE TO authenticated USING (is_active = false);
    END IF;
END $$;

-- =============================================================================
-- 3. Indexes
-- =============================================================================

CREATE INDEX IF NOT EXISTS idx_branches_business_entity_id ON public.branches(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_business_entities_is_active ON public.business_entities(is_active);
CREATE INDEX IF NOT EXISTS idx_branches_is_active ON public.branches(is_active);

-- =============================================================================
-- 4. Triggers
-- =============================================================================

-- Updated_at trigger for business_entities
CREATE OR REPLACE FUNCTION update_business_entities_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'business_entities_updated_at') THEN
        CREATE TRIGGER business_entities_updated_at
            BEFORE UPDATE ON public.business_entities
            FOR EACH ROW EXECUTE FUNCTION update_business_entities_updated_at();
    END IF;
END $$;

-- Updated_at trigger for branches
CREATE OR REPLACE FUNCTION update_branches_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'branches_updated_at') THEN
        CREATE TRIGGER branches_updated_at
            BEFORE UPDATE ON public.branches
            FOR EACH ROW EXECUTE FUNCTION update_branches_updated_at();
    END IF;
END $$;

-- Realtime notifications
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'business_entities_realtime') THEN
        CREATE TRIGGER business_entities_realtime
            AFTER INSERT OR UPDATE OR DELETE ON public.business_entities
            FOR EACH ROW EXECUTE FUNCTION notify_realtime();
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'branches_realtime') THEN
        CREATE TRIGGER branches_realtime
            AFTER INSERT OR UPDATE OR DELETE ON public.branches
            FOR EACH ROW EXECUTE FUNCTION notify_realtime();
    END IF;
END $$;

-- =============================================================================
-- 5. Notify PostgREST to reload schema
-- =============================================================================
NOTIFY pgrst, 'reload schema';
