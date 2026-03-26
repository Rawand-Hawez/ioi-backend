-- Party Domain Schema
-- This file creates the parties, unit_ownerships, and responsibility_assignments tables

-- ============================================================================
-- PARTIES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.parties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    party_type TEXT NOT NULL CHECK (party_type IN ('person', 'organization')),
    party_code TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    full_name TEXT,
    first_name TEXT,
    middle_name TEXT,
    last_name TEXT,
    organization_name TEXT,
    primary_phone TEXT NOT NULL,
    secondary_phone TEXT,
    primary_email TEXT,
    date_of_birth DATE,
    nationality TEXT,
    national_id_no TEXT,
    passport_no TEXT,
    registration_no TEXT,
    tax_no TEXT,
    preferred_language TEXT NOT NULL DEFAULT 'ar',
    legacy_ref TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'blocked')),
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Comments for documentation
COMMENT ON COLUMN public.parties.party_type IS 'Type of party: person or organization';
COMMENT ON COLUMN public.parties.status IS 'Party status: active, inactive, or blocked';

-- Enable RLS on parties
ALTER TABLE public.parties ENABLE ROW LEVEL SECURITY;

-- RLS Policies for parties
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'parties' 
        AND policyname = 'parties_select_authenticated'
    ) THEN
        CREATE POLICY parties_select_authenticated ON public.parties
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'parties' 
        AND policyname = 'parties_insert_authenticated'
    ) THEN
        CREATE POLICY parties_insert_authenticated ON public.parties
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'parties' 
        AND policyname = 'parties_update_authenticated'
    ) THEN
        CREATE POLICY parties_update_authenticated ON public.parties
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.parties TO authenticated;

-- Indexes for parties
CREATE INDEX IF NOT EXISTS idx_parties_primary_phone ON public.parties (primary_phone);
CREATE INDEX IF NOT EXISTS idx_parties_national_id_no ON public.parties (national_id_no) WHERE national_id_no IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_parties_passport_no ON public.parties (passport_no) WHERE passport_no IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_parties_primary_email ON public.parties (primary_email) WHERE primary_email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_parties_status ON public.parties (status);

-- Updated at trigger for parties
CREATE OR REPLACE FUNCTION update_parties_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger 
        WHERE tgname = 'parties_updated_at'
    ) THEN
        CREATE TRIGGER parties_updated_at
            BEFORE UPDATE ON public.parties
            FOR EACH ROW
            EXECUTE FUNCTION update_parties_updated_at();
    END IF;
END $$;

-- ============================================================================
-- UNIT_OWNERSHIPS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.unit_ownerships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    unit_id UUID NOT NULL REFERENCES public.units(id) ON DELETE RESTRICT,
    party_id UUID NOT NULL REFERENCES public.parties(id) ON DELETE RESTRICT,
    share_percentage NUMERIC(5,2) NOT NULL DEFAULT 100.00 CHECK (share_percentage > 0 AND share_percentage <= 100),
    effective_from DATE NOT NULL,
    effective_to DATE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Comment for documentation
COMMENT ON COLUMN public.unit_ownerships.status IS 'Ownership status: active or inactive';

-- Enable RLS on unit_ownerships
ALTER TABLE public.unit_ownerships ENABLE ROW LEVEL SECURITY;

-- RLS Policies for unit_ownerships
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'unit_ownerships' 
        AND policyname = 'unit_ownerships_select_authenticated'
    ) THEN
        CREATE POLICY unit_ownerships_select_authenticated ON public.unit_ownerships
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'unit_ownerships' 
        AND policyname = 'unit_ownerships_insert_authenticated'
    ) THEN
        CREATE POLICY unit_ownerships_insert_authenticated ON public.unit_ownerships
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'unit_ownerships' 
        AND policyname = 'unit_ownerships_update_authenticated'
    ) THEN
        CREATE POLICY unit_ownerships_update_authenticated ON public.unit_ownerships
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.unit_ownerships TO authenticated;

-- Indexes for unit_ownerships
CREATE INDEX IF NOT EXISTS idx_unit_ownerships_unit_id ON public.unit_ownerships (unit_id);
CREATE INDEX IF NOT EXISTS idx_unit_ownerships_party_id ON public.unit_ownerships (party_id);
CREATE INDEX IF NOT EXISTS idx_unit_ownerships_active ON public.unit_ownerships (unit_id) WHERE effective_to IS NULL;

-- Updated at trigger for unit_ownerships
CREATE OR REPLACE FUNCTION update_unit_ownerships_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger 
        WHERE tgname = 'unit_ownerships_updated_at'
    ) THEN
        CREATE TRIGGER unit_ownerships_updated_at
            BEFORE UPDATE ON public.unit_ownerships
            FOR EACH ROW
            EXECUTE FUNCTION update_unit_ownerships_updated_at();
    END IF;
END $$;

-- ============================================================================
-- RESPONSIBILITY_ASSIGNMENTS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.responsibility_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    unit_id UUID NOT NULL REFERENCES public.units(id) ON DELETE RESTRICT,
    party_id UUID NOT NULL REFERENCES public.parties(id) ON DELETE RESTRICT,
    responsibility_type TEXT NOT NULL CHECK (responsibility_type IN ('service_charge', 'electricity', 'water', 'gas', 'generator', 'all_utilities', 'general')),
    effective_from DATE NOT NULL,
    effective_to DATE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Comment for documentation
COMMENT ON COLUMN public.responsibility_assignments.status IS 'Assignment status: active or inactive';

-- Enable RLS on responsibility_assignments
ALTER TABLE public.responsibility_assignments ENABLE ROW LEVEL SECURITY;

-- RLS Policies for responsibility_assignments
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'responsibility_assignments' 
        AND policyname = 'responsibility_assignments_select_authenticated'
    ) THEN
        CREATE POLICY responsibility_assignments_select_authenticated ON public.responsibility_assignments
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'responsibility_assignments' 
        AND policyname = 'responsibility_assignments_insert_authenticated'
    ) THEN
        CREATE POLICY responsibility_assignments_insert_authenticated ON public.responsibility_assignments
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'responsibility_assignments' 
        AND policyname = 'responsibility_assignments_update_authenticated'
    ) THEN
        CREATE POLICY responsibility_assignments_update_authenticated ON public.responsibility_assignments
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.responsibility_assignments TO authenticated;

-- Indexes for responsibility_assignments
CREATE INDEX IF NOT EXISTS idx_responsibility_assignments_unit_type ON public.responsibility_assignments (unit_id, responsibility_type);
CREATE INDEX IF NOT EXISTS idx_responsibility_assignments_party_id ON public.responsibility_assignments (party_id);
CREATE INDEX IF NOT EXISTS idx_responsibility_assignments_unit_type_from ON public.responsibility_assignments (unit_id, responsibility_type, effective_from);

-- Updated at trigger for responsibility_assignments
CREATE OR REPLACE FUNCTION update_responsibility_assignments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger 
        WHERE tgname = 'responsibility_assignments_updated_at'
    ) THEN
        CREATE TRIGGER responsibility_assignments_updated_at
            BEFORE UPDATE ON public.responsibility_assignments
            FOR EACH ROW
            EXECUTE FUNCTION update_responsibility_assignments_updated_at();
    END IF;
END $$;

-- Notify PostgREST to reload schema
NOTIFY pgrst, 'reload schema';
