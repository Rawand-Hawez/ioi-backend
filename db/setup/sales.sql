-- =============================================================================
-- Sales Domain
-- =============================================================================
-- Entities: reservations, sales_contracts, payment_plan_templates,
--           installment_schedule_lines, sales_contract_parties, ownership_transfers
-- Dependencies: business_entities, branches, projects, units, parties, payments,
--               receivables, approval_requests
-- =============================================================================

-- =============================================================================
-- 1. Sequences for Document Number Generation
-- =============================================================================

CREATE SEQUENCE IF NOT EXISTS public.reservation_no_seq;
CREATE SEQUENCE IF NOT EXISTS public.contract_no_seq;

-- Helper function to generate reservation_no (format: RES-YYYY-00001)
CREATE OR REPLACE FUNCTION generate_reservation_no()
RETURNS TEXT AS $$
DECLARE
    v_year TEXT := TO_CHAR(CURRENT_DATE, 'YYYY');
    v_seq BIGINT;
BEGIN
    SELECT nextval('public.reservation_no_seq') INTO v_seq;
    RETURN 'RES-' || v_year || '-' || LPAD(v_seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- Helper function to generate contract_no (format: CON-YYYY-00001)
CREATE OR REPLACE FUNCTION generate_contract_no()
RETURNS TEXT AS $$
DECLARE
    v_year TEXT := TO_CHAR(CURRENT_DATE, 'YYYY');
    v_seq BIGINT;
BEGIN
    SELECT nextval('public.contract_no_seq') INTO v_seq;
    RETURN 'CON-' || v_year || '-' || LPAD(v_seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 2. Tables
-- =============================================================================

-- Reservations: Unit reservations before contract signing
CREATE TABLE IF NOT EXISTS public.reservations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id  UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id           UUID NOT NULL REFERENCES public.branches(id),
    project_id          UUID NOT NULL REFERENCES public.projects(id),
    unit_id             UUID NOT NULL REFERENCES public.units(id),
    customer_id         UUID NOT NULL REFERENCES public.parties(id),
    reservation_no      TEXT NOT NULL UNIQUE DEFAULT generate_reservation_no(),
    status              TEXT NOT NULL DEFAULT 'active',
    reserved_at         TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    expires_at          TIMESTAMPTZ NOT NULL,
    deposit_amount      NUMERIC(18,2) NOT NULL DEFAULT 0,
    deposit_currency    TEXT NOT NULL DEFAULT 'USD',
    deposit_payment_id  UUID REFERENCES public.payments(id),
    quoted_price_amount NUMERIC(18,2),
    discount_amount     NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes               TEXT,
    created_by_user_id  UUID NOT NULL REFERENCES auth.users(id),
    approved_by_user_id UUID REFERENCES auth.users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_reservation_status CHECK (status IN ('active', 'expired', 'cancelled', 'converted')),
    CONSTRAINT chk_reservation_expires_after_reserved CHECK (expires_at > reserved_at),
    CONSTRAINT chk_reservation_deposit_amount_nonnegative CHECK (deposit_amount >= 0),
    CONSTRAINT chk_reservation_discount_amount_nonnegative CHECK (discount_amount >= 0)
);

COMMENT ON COLUMN public.reservations.status IS 'active, converted, expired, cancelled';

-- Payment Plan Templates: Reusable installment plan definitions
CREATE TABLE IF NOT EXISTS public.payment_plan_templates (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id   UUID NOT NULL REFERENCES public.business_entities(id),
    project_id           UUID REFERENCES public.projects(id),
    code                 TEXT NOT NULL,
    name                 TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'active',
    frequency_type       TEXT NOT NULL,
    installment_count    INTEGER NOT NULL,
    generation_rule_json JSONB NOT NULL,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    UNIQUE (business_entity_id, code),
    CHECK (installment_count > 0)
);

COMMENT ON COLUMN public.payment_plan_templates.frequency_type IS 'monthly, quarterly, semi_annual, annual, custom';

-- Sales Contracts: Final sales agreements
CREATE TABLE IF NOT EXISTS public.sales_contracts (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id       UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id                UUID NOT NULL REFERENCES public.branches(id),
    project_id               UUID NOT NULL REFERENCES public.projects(id),
    unit_id                  UUID NOT NULL REFERENCES public.units(id),
    primary_buyer_id         UUID NOT NULL REFERENCES public.parties(id),
    source_reservation_id    UUID REFERENCES public.reservations(id),
    contract_no              TEXT NOT NULL UNIQUE DEFAULT generate_contract_no(),
    status                   TEXT NOT NULL DEFAULT 'draft',
    contract_date            DATE NOT NULL,
    effective_date           DATE NOT NULL,
    sale_price_amount        NUMERIC(18,2) NOT NULL,
    sale_price_currency      TEXT NOT NULL DEFAULT 'USD',
    discount_amount          NUMERIC(18,2) NOT NULL DEFAULT 0,
    net_contract_amount      NUMERIC(18,2) NOT NULL,
    down_payment_amount      NUMERIC(18,2) NOT NULL DEFAULT 0,
    financed_amount          NUMERIC(18,2) NOT NULL DEFAULT 0,
    payment_plan_template_id UUID REFERENCES public.payment_plan_templates(id),
    handover_date_planned    DATE,
    handover_date_actual     DATE,
    notes                    TEXT,
    created_by_user_id       UUID NOT NULL REFERENCES auth.users(id),
    approved_by_user_id      UUID REFERENCES auth.users(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_sales_contract_status CHECK (status IN ('draft', 'active', 'cancelled', 'completed', 'terminated', 'defaulted')),
    CONSTRAINT chk_sales_contract_amounts_nonnegative CHECK (
        sale_price_amount >= 0
        AND discount_amount >= 0
        AND net_contract_amount >= 0
        AND down_payment_amount >= 0
        AND financed_amount >= 0
    ),
    CONSTRAINT chk_sales_contract_net_amount CHECK (
        net_contract_amount = sale_price_amount - discount_amount
    ),
    CONSTRAINT chk_sales_contract_financed_amount CHECK (
        financed_amount = net_contract_amount - down_payment_amount
    )
);

COMMENT ON COLUMN public.sales_contracts.status IS 'draft, active, completed, cancelled, terminated, defaulted';

-- Installment Schedule Lines: Individual payment installments for a contract
CREATE TABLE IF NOT EXISTS public.installment_schedule_lines (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sales_contract_id       UUID NOT NULL REFERENCES public.sales_contracts(id),
    receivable_id           UUID REFERENCES public.receivables(id),
    line_no                 SMALLINT NOT NULL,
    due_date                DATE NOT NULL,
    line_type               TEXT NOT NULL,
    description             TEXT,
    principal_amount        NUMERIC(18,2) NOT NULL,
    penalty_amount_accrued  NUMERIC(18,2) NOT NULL DEFAULT 0,
    discount_amount_applied NUMERIC(18,2) NOT NULL DEFAULT 0,
    amount_paid             NUMERIC(18,2) NOT NULL DEFAULT 0,
    amount_outstanding      NUMERIC(18,2) GENERATED ALWAYS AS
                              (principal_amount + penalty_amount_accrued - discount_amount_applied - amount_paid) STORED,
    status                  TEXT NOT NULL DEFAULT 'scheduled',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    UNIQUE (sales_contract_id, line_no),
    CONSTRAINT chk_installment_schedule_line_type CHECK (line_type IN ('down_payment', 'installment', 'milestone', 'final', 'handover', 'adjustment')),
    CONSTRAINT chk_installment_schedule_line_status CHECK (status IN ('pending', 'scheduled', 'due', 'partially_paid', 'paid', 'waived', 'restructured', 'void')),
    CONSTRAINT chk_installment_schedule_line_principal_nonnegative CHECK (principal_amount >= 0),
    CONSTRAINT chk_installment_schedule_line_penalty_nonnegative CHECK (penalty_amount_accrued >= 0),
    CONSTRAINT chk_installment_schedule_line_discount_nonnegative CHECK (discount_amount_applied >= 0),
    CONSTRAINT chk_installment_schedule_line_amount_paid_nonnegative CHECK (amount_paid >= 0)
);

COMMENT ON COLUMN public.installment_schedule_lines.status IS 'scheduled, due, partially_paid, paid, waived, restructured';
COMMENT ON COLUMN public.installment_schedule_lines.line_type IS 'down_payment, installment, milestone, final';

-- Sales Contract Parties: Effective-dated party relationships for contracts
CREATE TABLE IF NOT EXISTS public.sales_contract_parties (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sales_contract_id UUID NOT NULL REFERENCES public.sales_contracts(id),
    party_id          UUID NOT NULL REFERENCES public.parties(id),
    role              TEXT NOT NULL,
    is_primary        BOOLEAN NOT NULL DEFAULT false,
    effective_from    DATE NOT NULL,
    effective_to      DATE,
    status            TEXT NOT NULL DEFAULT 'active',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

COMMENT ON COLUMN public.sales_contract_parties.role IS 'primary_buyer, co_buyer, guarantor';

-- Ownership Transfers: Buyer replacement or administrative corrections
CREATE TABLE IF NOT EXISTS public.ownership_transfers (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id   UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id            UUID NOT NULL REFERENCES public.branches(id),
    project_id           UUID NOT NULL REFERENCES public.projects(id),
    unit_id              UUID NOT NULL REFERENCES public.units(id),
    sales_contract_id    UUID NOT NULL REFERENCES public.sales_contracts(id),
    approval_request_id  UUID REFERENCES public.approval_requests(id),
    transfer_type        TEXT NOT NULL,
    from_party_id        UUID NOT NULL REFERENCES public.parties(id),
    to_party_id          UUID NOT NULL REFERENCES public.parties(id),
    effective_date       DATE NOT NULL,
    financial_treatment  TEXT NOT NULL DEFAULT 'no_change',
    transfer_fee_amount  NUMERIC(18,2) NOT NULL DEFAULT 0,
    transfer_fee_currency TEXT NOT NULL DEFAULT 'USD',
    notes                TEXT,
    status               TEXT NOT NULL DEFAULT 'pending',
    requested_by_user_id UUID NOT NULL REFERENCES auth.users(id),
    approved_by_user_id  UUID REFERENCES auth.users(id),
    completed_by_user_id UUID REFERENCES auth.users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT chk_ownership_transfer_status CHECK (status IN ('pending', 'pending_approval', 'approved', 'completed', 'rejected', 'cancelled')),
    CONSTRAINT chk_ownership_transfer_distinct_parties CHECK (from_party_id <> to_party_id),
    CONSTRAINT chk_ownership_transfer_fee_nonnegative CHECK (transfer_fee_amount >= 0)
);

COMMENT ON COLUMN public.ownership_transfers.transfer_type IS 'buyer_replacement, administrative_correction';
COMMENT ON COLUMN public.ownership_transfers.financial_treatment IS 'no_change, restructure, settle_and_restart';
COMMENT ON COLUMN public.ownership_transfers.status IS 'pending, approved, rejected, completed, cancelled';

-- Add named constraints to existing databases without requiring destructive reset.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_reservation_status') THEN
        ALTER TABLE public.reservations
            ADD CONSTRAINT chk_reservation_status
            CHECK (status IN ('active', 'expired', 'cancelled', 'converted')) NOT VALID;
    END IF;
END $$;

-- Phase 7 review: tighten chk_sales_contract_status to only include statuses the code path actually
-- transitions to. Cancellation/termination stay 'active' through the approval gate; intermediate
-- pending_approval values were never set by handlers. Drop-and-recreate to migrate older databases.
DO $$
BEGIN
    ALTER TABLE public.sales_contracts DROP CONSTRAINT IF EXISTS chk_sales_contract_status;
    ALTER TABLE public.sales_contracts
        ADD CONSTRAINT chk_sales_contract_status
        CHECK (status IN ('draft', 'active', 'cancelled', 'completed', 'terminated', 'defaulted')) NOT VALID;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_sales_contract_amounts_nonnegative') THEN
        ALTER TABLE public.sales_contracts
            ADD CONSTRAINT chk_sales_contract_amounts_nonnegative
            CHECK (
                sale_price_amount >= 0
                AND discount_amount >= 0
                AND net_contract_amount >= 0
                AND down_payment_amount >= 0
                AND financed_amount >= 0
            ) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_sales_contract_net_amount') THEN
        ALTER TABLE public.sales_contracts
            ADD CONSTRAINT chk_sales_contract_net_amount
            CHECK (net_contract_amount = sale_price_amount - discount_amount) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_sales_contract_financed_amount') THEN
        ALTER TABLE public.sales_contracts
            ADD CONSTRAINT chk_sales_contract_financed_amount
            CHECK (financed_amount = net_contract_amount - down_payment_amount) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_installment_schedule_line_type') THEN
        ALTER TABLE public.installment_schedule_lines
            ADD CONSTRAINT chk_installment_schedule_line_type
            CHECK (line_type IN ('down_payment', 'installment', 'milestone', 'final', 'handover', 'adjustment')) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_installment_schedule_line_status') THEN
        ALTER TABLE public.installment_schedule_lines
            ADD CONSTRAINT chk_installment_schedule_line_status
            CHECK (status IN ('pending', 'scheduled', 'due', 'partially_paid', 'paid', 'waived', 'restructured', 'void')) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_ownership_transfer_status') THEN
        ALTER TABLE public.ownership_transfers
            ADD CONSTRAINT chk_ownership_transfer_status
            CHECK (status IN ('pending', 'pending_approval', 'approved', 'completed', 'rejected', 'cancelled')) NOT VALID;
    END IF;
END $$;

-- =============================================================================
-- 3. Row Level Security
-- =============================================================================

ALTER TABLE public.reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.payment_plan_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sales_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.installment_schedule_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sales_contract_parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.ownership_transfers ENABLE ROW LEVEL SECURITY;

GRANT SELECT, INSERT, UPDATE ON public.reservations TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.payment_plan_templates TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.sales_contracts TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.installment_schedule_lines TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.sales_contract_parties TO authenticated;
GRANT SELECT, INSERT, UPDATE ON public.ownership_transfers TO authenticated;

-- Reservations RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'reservations'
        AND policyname = 'reservations_select_authenticated'
    ) THEN
        CREATE POLICY reservations_select_authenticated ON public.reservations
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'reservations'
        AND policyname = 'reservations_insert_authenticated'
    ) THEN
        CREATE POLICY reservations_insert_authenticated ON public.reservations
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'reservations'
        AND policyname = 'reservations_update_authenticated'
    ) THEN
        CREATE POLICY reservations_update_authenticated ON public.reservations
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Payment Plan Templates RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'payment_plan_templates'
        AND policyname = 'payment_plan_templates_select_authenticated'
    ) THEN
        CREATE POLICY payment_plan_templates_select_authenticated ON public.payment_plan_templates
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'payment_plan_templates'
        AND policyname = 'payment_plan_templates_insert_authenticated'
    ) THEN
        CREATE POLICY payment_plan_templates_insert_authenticated ON public.payment_plan_templates
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'payment_plan_templates'
        AND policyname = 'payment_plan_templates_update_authenticated'
    ) THEN
        CREATE POLICY payment_plan_templates_update_authenticated ON public.payment_plan_templates
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Sales Contracts RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contracts'
        AND policyname = 'sales_contracts_select_authenticated'
    ) THEN
        CREATE POLICY sales_contracts_select_authenticated ON public.sales_contracts
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contracts'
        AND policyname = 'sales_contracts_insert_authenticated'
    ) THEN
        CREATE POLICY sales_contracts_insert_authenticated ON public.sales_contracts
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contracts'
        AND policyname = 'sales_contracts_update_authenticated'
    ) THEN
        CREATE POLICY sales_contracts_update_authenticated ON public.sales_contracts
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Installment Schedule Lines RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'installment_schedule_lines'
        AND policyname = 'installment_schedule_lines_select_authenticated'
    ) THEN
        CREATE POLICY installment_schedule_lines_select_authenticated ON public.installment_schedule_lines
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'installment_schedule_lines'
        AND policyname = 'installment_schedule_lines_insert_authenticated'
    ) THEN
        CREATE POLICY installment_schedule_lines_insert_authenticated ON public.installment_schedule_lines
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'installment_schedule_lines'
        AND policyname = 'installment_schedule_lines_update_authenticated'
    ) THEN
        CREATE POLICY installment_schedule_lines_update_authenticated ON public.installment_schedule_lines
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Sales Contract Parties RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contract_parties'
        AND policyname = 'sales_contract_parties_select_authenticated'
    ) THEN
        CREATE POLICY sales_contract_parties_select_authenticated ON public.sales_contract_parties
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contract_parties'
        AND policyname = 'sales_contract_parties_insert_authenticated'
    ) THEN
        CREATE POLICY sales_contract_parties_insert_authenticated ON public.sales_contract_parties
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'sales_contract_parties'
        AND policyname = 'sales_contract_parties_update_authenticated'
    ) THEN
        CREATE POLICY sales_contract_parties_update_authenticated ON public.sales_contract_parties
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Ownership Transfers RLS Policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'ownership_transfers'
        AND policyname = 'ownership_transfers_select_authenticated'
    ) THEN
        CREATE POLICY ownership_transfers_select_authenticated ON public.ownership_transfers
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'ownership_transfers'
        AND policyname = 'ownership_transfers_insert_authenticated'
    ) THEN
        CREATE POLICY ownership_transfers_insert_authenticated ON public.ownership_transfers
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'ownership_transfers'
        AND policyname = 'ownership_transfers_update_authenticated'
    ) THEN
        CREATE POLICY ownership_transfers_update_authenticated ON public.ownership_transfers
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- =============================================================================
-- 4. Indexes
-- =============================================================================

-- Reservations indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_reservations_unique_active ON public.reservations (unit_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_reservations_business_entity_id ON public.reservations (business_entity_id);
CREATE INDEX IF NOT EXISTS idx_reservations_branch_id ON public.reservations (branch_id);
CREATE INDEX IF NOT EXISTS idx_reservations_project_id ON public.reservations (project_id);
CREATE INDEX IF NOT EXISTS idx_reservations_unit_status ON public.reservations (unit_id, status);
CREATE INDEX IF NOT EXISTS idx_reservations_customer_id ON public.reservations (customer_id);
CREATE INDEX IF NOT EXISTS idx_reservations_created_by_user_id ON public.reservations (created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_reservations_deposit_payment_id ON public.reservations (deposit_payment_id) WHERE deposit_payment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_reservations_expires_active ON public.reservations (expires_at) WHERE status = 'active';

-- Payment Plan Templates indexes
CREATE INDEX IF NOT EXISTS idx_payment_plan_templates_business_entity_id ON public.payment_plan_templates (business_entity_id);
CREATE INDEX IF NOT EXISTS idx_payment_plan_templates_project_id ON public.payment_plan_templates (project_id) WHERE project_id IS NOT NULL;

-- Sales Contracts indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_contracts_unique_active ON public.sales_contracts (unit_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_sales_contracts_business_entity_id ON public.sales_contracts (business_entity_id);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_branch_id ON public.sales_contracts (branch_id);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_project_id ON public.sales_contracts (project_id);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_unit_status ON public.sales_contracts (unit_id, status);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_primary_buyer_id ON public.sales_contracts (primary_buyer_id);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_source_reservation_id ON public.sales_contracts (source_reservation_id) WHERE source_reservation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sales_contracts_payment_plan_template_id ON public.sales_contracts (payment_plan_template_id) WHERE payment_plan_template_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sales_contracts_created_by_user_id ON public.sales_contracts (created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_sales_contracts_approved_by_user_id ON public.sales_contracts (approved_by_user_id) WHERE approved_by_user_id IS NOT NULL;

-- Installment Schedule Lines indexes
CREATE INDEX IF NOT EXISTS idx_installment_schedule_lines_sales_contract_id ON public.installment_schedule_lines (sales_contract_id);
CREATE INDEX IF NOT EXISTS idx_installment_schedule_lines_due_date_status ON public.installment_schedule_lines (due_date, status);
CREATE INDEX IF NOT EXISTS idx_installment_schedule_lines_receivable_id ON public.installment_schedule_lines (receivable_id) WHERE receivable_id IS NOT NULL;

-- Sales Contract Parties indexes
CREATE INDEX IF NOT EXISTS idx_sales_contract_parties_sales_contract_id ON public.sales_contract_parties (sales_contract_id);
CREATE INDEX IF NOT EXISTS idx_sales_contract_parties_party_id ON public.sales_contract_parties (party_id);
CREATE INDEX IF NOT EXISTS idx_sales_contract_parties_active ON public.sales_contract_parties (sales_contract_id) WHERE effective_to IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_contract_parties_unique_primary ON public.sales_contract_parties (sales_contract_id) WHERE is_primary = true AND effective_to IS NULL;

-- Ownership Transfers indexes
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_business_entity_id ON public.ownership_transfers (business_entity_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_branch_id ON public.ownership_transfers (branch_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_project_id ON public.ownership_transfers (project_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_unit_id ON public.ownership_transfers (unit_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_sales_contract_id ON public.ownership_transfers (sales_contract_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_approval_request_id ON public.ownership_transfers (approval_request_id) WHERE approval_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_from_party_id ON public.ownership_transfers (from_party_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_to_party_id ON public.ownership_transfers (to_party_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_requested_by_user_id ON public.ownership_transfers (requested_by_user_id);
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_approved_by_user_id ON public.ownership_transfers (approved_by_user_id) WHERE approved_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_completed_by_user_id ON public.ownership_transfers (completed_by_user_id) WHERE completed_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ownership_transfers_status ON public.ownership_transfers (status);

-- =============================================================================
-- 5. Triggers
-- =============================================================================

-- Updated_at trigger for reservations
CREATE OR REPLACE FUNCTION update_reservations_updated_at()
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
        WHERE tgname = 'reservations_updated_at'
    ) THEN
        CREATE TRIGGER reservations_updated_at
            BEFORE UPDATE ON public.reservations
            FOR EACH ROW
            EXECUTE FUNCTION update_reservations_updated_at();
    END IF;
END $$;

-- Updated_at trigger for payment_plan_templates
CREATE OR REPLACE FUNCTION update_payment_plan_templates_updated_at()
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
        WHERE tgname = 'payment_plan_templates_updated_at'
    ) THEN
        CREATE TRIGGER payment_plan_templates_updated_at
            BEFORE UPDATE ON public.payment_plan_templates
            FOR EACH ROW
            EXECUTE FUNCTION update_payment_plan_templates_updated_at();
    END IF;
END $$;

-- Updated_at trigger for sales_contracts
CREATE OR REPLACE FUNCTION update_sales_contracts_updated_at()
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
        WHERE tgname = 'sales_contracts_updated_at'
    ) THEN
        CREATE TRIGGER sales_contracts_updated_at
            BEFORE UPDATE ON public.sales_contracts
            FOR EACH ROW
            EXECUTE FUNCTION update_sales_contracts_updated_at();
    END IF;
END $$;

-- Updated_at trigger for installment_schedule_lines
CREATE OR REPLACE FUNCTION update_installment_schedule_lines_updated_at()
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
        WHERE tgname = 'installment_schedule_lines_updated_at'
    ) THEN
        CREATE TRIGGER installment_schedule_lines_updated_at
            BEFORE UPDATE ON public.installment_schedule_lines
            FOR EACH ROW
            EXECUTE FUNCTION update_installment_schedule_lines_updated_at();
    END IF;
END $$;

-- Updated_at trigger for sales_contract_parties
CREATE OR REPLACE FUNCTION update_sales_contract_parties_updated_at()
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
        WHERE tgname = 'sales_contract_parties_updated_at'
    ) THEN
        CREATE TRIGGER sales_contract_parties_updated_at
            BEFORE UPDATE ON public.sales_contract_parties
            FOR EACH ROW
            EXECUTE FUNCTION update_sales_contract_parties_updated_at();
    END IF;
END $$;

-- Updated_at trigger for ownership_transfers
CREATE OR REPLACE FUNCTION update_ownership_transfers_updated_at()
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
        WHERE tgname = 'ownership_transfers_updated_at'
    ) THEN
        CREATE TRIGGER ownership_transfers_updated_at
            BEFORE UPDATE ON public.ownership_transfers
            FOR EACH ROW
            EXECUTE FUNCTION update_ownership_transfers_updated_at();
    END IF;
END $$;

-- Realtime notifications (only if notify_realtime function exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'notify_realtime') THEN
        -- Reservations realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'reservations_realtime') THEN
            CREATE TRIGGER reservations_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.reservations
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;

        -- Payment Plan Templates realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'payment_plan_templates_realtime') THEN
            CREATE TRIGGER payment_plan_templates_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.payment_plan_templates
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;

        -- Sales Contracts realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'sales_contracts_realtime') THEN
            CREATE TRIGGER sales_contracts_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.sales_contracts
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;

        -- Installment Schedule Lines realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'installment_schedule_lines_realtime') THEN
            CREATE TRIGGER installment_schedule_lines_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.installment_schedule_lines
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;

        -- Sales Contract Parties realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'sales_contract_parties_realtime') THEN
            CREATE TRIGGER sales_contract_parties_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.sales_contract_parties
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;

        -- Ownership Transfers realtime
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'ownership_transfers_realtime') THEN
            CREATE TRIGGER ownership_transfers_realtime
                AFTER INSERT OR UPDATE OR DELETE ON public.ownership_transfers
                FOR EACH ROW EXECUTE FUNCTION notify_realtime();
        END IF;
    END IF;
END $$;

-- =============================================================================
-- 5b. Immutability trigger: posted schedule lines (financial fields locked once
--     a receivable has been linked to the line — direct mutation must be
--     approval-gated and applied through a controlled flow)
-- =============================================================================
CREATE OR REPLACE FUNCTION prevent_posted_schedule_line_mutation()
RETURNS trigger AS $$
BEGIN
    IF OLD.receivable_id IS NOT NULL
       AND (
           NEW.principal_amount IS DISTINCT FROM OLD.principal_amount
           OR NEW.due_date IS DISTINCT FROM OLD.due_date
           OR NEW.line_type IS DISTINCT FROM OLD.line_type
       ) THEN
        RAISE EXCEPTION 'posted schedule lines are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_prevent_posted_schedule_line_mutation'
    ) THEN
        CREATE TRIGGER trg_prevent_posted_schedule_line_mutation
            BEFORE UPDATE ON public.installment_schedule_lines
            FOR EACH ROW
            EXECUTE FUNCTION prevent_posted_schedule_line_mutation();
    END IF;
END $$;

-- =============================================================================
-- 6. Notify PostgREST to reload schema
-- =============================================================================
NOTIFY pgrst, 'reload schema';
