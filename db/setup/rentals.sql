-- =============================================================================
-- Rentals Domain
-- =============================================================================
-- Lease contracts, effective-dated lease parties, and recurring lease bills.
-- Issued lease bills produce explicit receivables in the shared finance module
-- (source_module = 'rentals', source_record_type = 'lease_bill').
--
-- Lease termination follows the Phase 7 sales pattern: the lease stays at
-- 'active' through the approval gate. There is no intermediate
-- termination_pending_approval status; the gate looks up the latest approval
-- request via (source_record_type, source_record_id, request_type).

CREATE SEQUENCE IF NOT EXISTS public.lease_no_seq;

CREATE OR REPLACE FUNCTION public.generate_lease_no()
RETURNS TEXT AS $$
DECLARE
    v_year TEXT := TO_CHAR(CURRENT_DATE, 'YYYY');
    v_seq BIGINT;
BEGIN
    SELECT nextval('public.lease_no_seq') INTO v_seq;
    RETURN 'LSE-' || v_year || '-' || LPAD(v_seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 1. Tables
-- =============================================================================

CREATE TABLE IF NOT EXISTS public.lease_contracts (
    id                             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id             UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id                      UUID NOT NULL REFERENCES public.branches(id),
    project_id                     UUID NOT NULL REFERENCES public.projects(id),
    unit_id                        UUID NOT NULL REFERENCES public.units(id),
    primary_tenant_id              UUID NOT NULL REFERENCES public.parties(id),
    renewed_from_lease_contract_id UUID REFERENCES public.lease_contracts(id),
    lease_no                       TEXT NOT NULL UNIQUE DEFAULT public.generate_lease_no(),
    lease_type                     TEXT NOT NULL,
    status                         TEXT NOT NULL DEFAULT 'draft',
    start_date                     DATE NOT NULL,
    end_date                       DATE NOT NULL,
    rent_pricing_basis             TEXT NOT NULL,
    area_basis_sqm                 NUMERIC(10,2),
    rate_per_sqm                   NUMERIC(10,4),
    contractual_rent_amount        NUMERIC(18,2) NOT NULL,
    billing_interval_value         SMALLINT NOT NULL DEFAULT 1,
    billing_interval_unit          TEXT NOT NULL DEFAULT 'month',
    billing_anchor_date            DATE NOT NULL,
    security_deposit_amount        NUMERIC(18,2) NOT NULL DEFAULT 0,
    advance_rent_amount            NUMERIC(18,2) NOT NULL DEFAULT 0,
    currency_code                  TEXT NOT NULL DEFAULT 'USD',
    notice_period_days             INTEGER,
    purpose_of_use                 TEXT,
    notes                          TEXT,
    created_by_user_id             UUID NOT NULL REFERENCES auth.users(id),
    approved_by_user_id            UUID REFERENCES auth.users(id),
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at                     TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_lease_contract_type CHECK (lease_type IN ('residential', 'commercial')),
    CONSTRAINT chk_lease_contract_status CHECK (status IN ('draft', 'active', 'expired', 'renewed', 'terminated')),
    CONSTRAINT chk_lease_contract_dates CHECK (end_date >= start_date),
    CONSTRAINT chk_lease_contract_rent_basis CHECK (rent_pricing_basis IN ('fixed_amount', 'area_based')),
    CONSTRAINT chk_lease_contract_interval_unit CHECK (billing_interval_unit IN ('month', 'quarter', 'semi_year', 'year')),
    CONSTRAINT chk_lease_contract_interval_positive CHECK (billing_interval_value > 0),
    CONSTRAINT chk_lease_contract_amounts_nonnegative CHECK (
        contractual_rent_amount >= 0
        AND COALESCE(area_basis_sqm, 0) >= 0
        AND COALESCE(rate_per_sqm, 0) >= 0
        AND security_deposit_amount >= 0
        AND advance_rent_amount >= 0
    ),
    CONSTRAINT chk_lease_contract_area_pricing CHECK (
        rent_pricing_basis <> 'area_based'
        OR (area_basis_sqm IS NOT NULL AND rate_per_sqm IS NOT NULL)
    )
);

COMMENT ON COLUMN public.lease_contracts.status IS 'draft, active, expired, renewed, terminated. Termination is approval-gated; lease stays active during pending approval (Phase 7 pattern).';

CREATE TABLE IF NOT EXISTS public.lease_parties (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lease_contract_id UUID NOT NULL REFERENCES public.lease_contracts(id),
    party_id          UUID NOT NULL REFERENCES public.parties(id),
    role              TEXT NOT NULL,
    is_primary        BOOLEAN NOT NULL DEFAULT false,
    effective_from    DATE NOT NULL,
    effective_to      DATE,
    status            TEXT NOT NULL DEFAULT 'active',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_lease_party_role CHECK (role IN ('primary_tenant', 'co_tenant', 'guarantor')),
    CONSTRAINT chk_lease_party_status CHECK (status IN ('active', 'inactive', 'closed')),
    CONSTRAINT chk_lease_party_dates CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE IF NOT EXISTS public.lease_bills (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id     UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id              UUID NOT NULL REFERENCES public.branches(id),
    lease_contract_id      UUID NOT NULL REFERENCES public.lease_contracts(id),
    unit_id                UUID NOT NULL REFERENCES public.units(id),
    responsible_party_id   UUID NOT NULL REFERENCES public.parties(id),
    receivable_id          UUID REFERENCES public.receivables(id),
    billing_period_start   DATE NOT NULL,
    billing_period_end     DATE NOT NULL,
    due_date               DATE NOT NULL,
    billing_interval_value SMALLINT NOT NULL,
    billing_interval_unit  TEXT NOT NULL,
    billed_amount          NUMERIC(18,2) NOT NULL,
    currency_code          TEXT NOT NULL DEFAULT 'USD',
    is_advance             BOOLEAN NOT NULL DEFAULT false,
    status                 TEXT NOT NULL DEFAULT 'draft',
    notes                  TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_lease_bill_period CHECK (billing_period_end >= billing_period_start),
    CONSTRAINT chk_lease_bill_interval_unit CHECK (billing_interval_unit IN ('month', 'quarter', 'semi_year', 'year')),
    CONSTRAINT chk_lease_bill_status CHECK (status IN ('draft', 'issued', 'partially_paid', 'paid', 'voided')),
    CONSTRAINT chk_lease_bill_amount_nonnegative CHECK (billed_amount >= 0),
    CONSTRAINT unique_lease_bill_period UNIQUE (lease_contract_id, billing_period_start, billing_period_end, is_advance)
);

ALTER TABLE public.lease_bills
    DROP CONSTRAINT IF EXISTS unique_lease_bill_period;
ALTER TABLE public.lease_bills
    ADD CONSTRAINT unique_lease_bill_period UNIQUE (lease_contract_id, billing_period_start, billing_period_end, is_advance);

-- =============================================================================
-- 2. Indexes
-- =============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS unique_active_lease_per_unit
    ON public.lease_contracts(unit_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_lease_contracts_business_entity_id ON public.lease_contracts(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_branch_id ON public.lease_contracts(branch_id);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_project_id ON public.lease_contracts(project_id);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_unit_status ON public.lease_contracts(unit_id, status);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_primary_tenant_id ON public.lease_contracts(primary_tenant_id);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_renewed_from ON public.lease_contracts(renewed_from_lease_contract_id) WHERE renewed_from_lease_contract_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_lease_contracts_created_by ON public.lease_contracts(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_lease_contracts_approved_by ON public.lease_contracts(approved_by_user_id) WHERE approved_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_lease_contracts_active_end_date ON public.lease_contracts(end_date) WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_lease_parties_lease_contract_id ON public.lease_parties(lease_contract_id);
CREATE INDEX IF NOT EXISTS idx_lease_parties_party_id ON public.lease_parties(party_id);
CREATE INDEX IF NOT EXISTS idx_lease_parties_active_lease ON public.lease_parties(lease_contract_id) WHERE effective_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_lease_bills_business_entity_id ON public.lease_bills(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_lease_bills_branch_id ON public.lease_bills(branch_id);
CREATE INDEX IF NOT EXISTS idx_lease_bills_lease_contract_id ON public.lease_bills(lease_contract_id);
CREATE INDEX IF NOT EXISTS idx_lease_bills_unit_id ON public.lease_bills(unit_id);
CREATE INDEX IF NOT EXISTS idx_lease_bills_responsible_party_id ON public.lease_bills(responsible_party_id);
CREATE INDEX IF NOT EXISTS idx_lease_bills_receivable_id ON public.lease_bills(receivable_id) WHERE receivable_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_lease_bills_billing_period_start ON public.lease_bills(billing_period_start);
CREATE INDEX IF NOT EXISTS idx_lease_bills_status ON public.lease_bills(status);

-- =============================================================================
-- 3. Row Level Security
-- =============================================================================

ALTER TABLE public.lease_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.lease_parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.lease_bills ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON public.lease_contracts TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.lease_parties TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.lease_bills TO authenticated;

DO $$
BEGIN
    DROP POLICY IF EXISTS lease_contracts_authenticated_all ON public.lease_contracts;
    DROP POLICY IF EXISTS lease_parties_authenticated_all ON public.lease_parties;
    DROP POLICY IF EXISTS lease_bills_authenticated_all ON public.lease_bills;

    DROP POLICY IF EXISTS lease_contracts_select_authenticated ON public.lease_contracts;
    DROP POLICY IF EXISTS lease_contracts_write_via_fiber ON public.lease_contracts;
    DROP POLICY IF EXISTS lease_contracts_update_via_fiber ON public.lease_contracts;
    DROP POLICY IF EXISTS lease_parties_select_authenticated ON public.lease_parties;
    DROP POLICY IF EXISTS lease_parties_write_via_fiber ON public.lease_parties;
    DROP POLICY IF EXISTS lease_parties_update_via_fiber ON public.lease_parties;
    DROP POLICY IF EXISTS lease_bills_select_authenticated ON public.lease_bills;
    DROP POLICY IF EXISTS lease_bills_write_via_fiber ON public.lease_bills;
    DROP POLICY IF EXISTS lease_bills_update_via_fiber ON public.lease_bills;

    CREATE POLICY lease_contracts_select_authenticated ON public.lease_contracts
        FOR SELECT TO authenticated USING (true);
    CREATE POLICY lease_contracts_write_via_fiber ON public.lease_contracts
        FOR INSERT TO authenticated WITH CHECK (current_setting('app.request_origin', true) = 'fiber');
    CREATE POLICY lease_contracts_update_via_fiber ON public.lease_contracts
        FOR UPDATE TO authenticated USING (current_setting('app.request_origin', true) = 'fiber')
        WITH CHECK (current_setting('app.request_origin', true) = 'fiber');

    CREATE POLICY lease_parties_select_authenticated ON public.lease_parties
        FOR SELECT TO authenticated USING (true);
    CREATE POLICY lease_parties_write_via_fiber ON public.lease_parties
        FOR INSERT TO authenticated WITH CHECK (current_setting('app.request_origin', true) = 'fiber');
    CREATE POLICY lease_parties_update_via_fiber ON public.lease_parties
        FOR UPDATE TO authenticated USING (current_setting('app.request_origin', true) = 'fiber')
        WITH CHECK (current_setting('app.request_origin', true) = 'fiber');

    CREATE POLICY lease_bills_select_authenticated ON public.lease_bills
        FOR SELECT TO authenticated USING (true);
    CREATE POLICY lease_bills_write_via_fiber ON public.lease_bills
        FOR INSERT TO authenticated WITH CHECK (current_setting('app.request_origin', true) = 'fiber');
    CREATE POLICY lease_bills_update_via_fiber ON public.lease_bills
        FOR UPDATE TO authenticated USING (current_setting('app.request_origin', true) = 'fiber')
        WITH CHECK (current_setting('app.request_origin', true) = 'fiber');
END $$;

-- =============================================================================
-- 4. updated_at triggers
-- =============================================================================

CREATE OR REPLACE FUNCTION public.update_rentals_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = timezone('utc', now());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'lease_contracts_updated_at') THEN
        CREATE TRIGGER lease_contracts_updated_at
            BEFORE UPDATE ON public.lease_contracts
            FOR EACH ROW EXECUTE FUNCTION public.update_rentals_updated_at();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'lease_parties_updated_at') THEN
        CREATE TRIGGER lease_parties_updated_at
            BEFORE UPDATE ON public.lease_parties
            FOR EACH ROW EXECUTE FUNCTION public.update_rentals_updated_at();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'lease_bills_updated_at') THEN
        CREATE TRIGGER lease_bills_updated_at
            BEFORE UPDATE ON public.lease_bills
            FOR EACH ROW EXECUTE FUNCTION public.update_rentals_updated_at();
    END IF;
END $$;

NOTIFY pgrst, 'reload schema';
