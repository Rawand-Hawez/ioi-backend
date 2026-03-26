-- Finance Domain Schema
-- This file creates the receivables, payments, payment_allocations, credit_balances, and financial_adjustments tables

-- ============================================================================
-- RECEIVABLES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.receivables (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id  UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id           UUID NOT NULL REFERENCES public.branches(id),
    party_id            UUID NOT NULL REFERENCES public.parties(id),
    unit_id             UUID REFERENCES public.units(id),
    source_module       TEXT NOT NULL,
    source_record_type  TEXT NOT NULL,
    source_record_id    UUID NOT NULL,
    receivable_no       TEXT UNIQUE,
    receivable_date     DATE NOT NULL,
    due_date            DATE NOT NULL,
    currency_code       TEXT NOT NULL DEFAULT 'USD',
    original_amount     NUMERIC(18,2) NOT NULL,
    adjusted_amount     NUMERIC(18,2) NOT NULL DEFAULT 0,
    paid_amount         NUMERIC(18,2) NOT NULL DEFAULT 0,
    credited_amount     NUMERIC(18,2) NOT NULL DEFAULT 0,
    outstanding_amount  NUMERIC(18,2) GENERATED ALWAYS AS
                          (original_amount + adjusted_amount - paid_amount - credited_amount) STORED,
    status              TEXT NOT NULL DEFAULT 'open',
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT receivables_source_module_check CHECK (source_module IN ('sales', 'rentals', 'service_charges', 'utilities', 'manual')),
    CONSTRAINT receivables_source_record_type_check CHECK (source_record_type IN ('installment_schedule_line', 'lease_bill', 'service_charge_bill', 'utility_bill', 'manual')),
    CONSTRAINT receivables_status_check CHECK (status IN ('open', 'partially_paid', 'paid', 'voided', 'written_off')),
    CONSTRAINT receivables_original_amount_positive CHECK (original_amount > 0)
);

-- Enable RLS on receivables
ALTER TABLE public.receivables ENABLE ROW LEVEL SECURITY;

-- RLS Policies for receivables
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'receivables' 
        AND policyname = 'receivables_select_authenticated'
    ) THEN
        CREATE POLICY receivables_select_authenticated ON public.receivables
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'receivables' 
        AND policyname = 'receivables_insert_authenticated'
    ) THEN
        CREATE POLICY receivables_insert_authenticated ON public.receivables
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'receivables' 
        AND policyname = 'receivables_update_authenticated'
    ) THEN
        CREATE POLICY receivables_update_authenticated ON public.receivables
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.receivables TO authenticated;

-- Indexes for receivables
CREATE INDEX IF NOT EXISTS idx_receivables_business_entity_id ON public.receivables(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_receivables_branch_id ON public.receivables(branch_id);
CREATE INDEX IF NOT EXISTS idx_receivables_party_id ON public.receivables(party_id);
CREATE INDEX IF NOT EXISTS idx_receivables_unit_id ON public.receivables(unit_id) WHERE unit_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receivables_due_date ON public.receivables(due_date);
CREATE INDEX IF NOT EXISTS idx_receivables_status ON public.receivables(status);
CREATE INDEX IF NOT EXISTS idx_receivables_source ON public.receivables(source_module, source_record_id);

-- Updated at trigger for receivables
CREATE OR REPLACE FUNCTION update_receivables_updated_at()
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
        WHERE tgname = 'receivables_updated_at'
    ) THEN
        CREATE TRIGGER receivables_updated_at
            BEFORE UPDATE ON public.receivables
            FOR EACH ROW
            EXECUTE FUNCTION update_receivables_updated_at();
    END IF;
END $$;

-- Immutability trigger: receivables
-- Prevent modification of original_amount, source_record_id, party_id when status != 'open'
CREATE OR REPLACE FUNCTION prevent_receivable_mutation() RETURNS trigger AS $$
BEGIN
    IF OLD.status != 'open' THEN
        IF NEW.original_amount != OLD.original_amount THEN
            RAISE EXCEPTION 'cannot modify original_amount on non-open receivable';
        END IF;
        IF NEW.source_record_id != OLD.source_record_id THEN
            RAISE EXCEPTION 'cannot modify source_record_id on non-open receivable';
        END IF;
        IF NEW.party_id != OLD.party_id THEN
            RAISE EXCEPTION 'cannot modify party_id on non-open receivable';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_prevent_receivable_mutation'
    ) THEN
        CREATE TRIGGER trg_prevent_receivable_mutation
            BEFORE UPDATE ON public.receivables
            FOR EACH ROW EXECUTE FUNCTION prevent_receivable_mutation();
    END IF;
END $$;

-- ============================================================================
-- PAYMENTS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id  UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id           UUID NOT NULL REFERENCES public.branches(id),
    party_id            UUID NOT NULL REFERENCES public.parties(id),
    payment_no          TEXT NOT NULL UNIQUE,
    receipt_no          TEXT,
    payment_date        DATE NOT NULL,
    payment_method      TEXT NOT NULL,
    currency_code       TEXT NOT NULL DEFAULT 'USD',
    amount_received     NUMERIC(18,2) NOT NULL,
    unapplied_amount    NUMERIC(18,2) NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'draft',
    reference_no        TEXT,
    notes               TEXT,
    received_by_user_id UUID NOT NULL REFERENCES auth.users(id),
    posted_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT payments_payment_method_check CHECK (payment_method IN ('cash', 'bank_transfer', 'cheque', 'card', 'credit_balance')),
    CONSTRAINT payments_status_check CHECK (status IN ('draft', 'posted', 'voided')),
    CONSTRAINT payments_amount_received_positive CHECK (amount_received > 0)
);

-- Enable RLS on payments
ALTER TABLE public.payments ENABLE ROW LEVEL SECURITY;

-- RLS Policies for payments
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'payments' 
        AND policyname = 'payments_select_authenticated'
    ) THEN
        CREATE POLICY payments_select_authenticated ON public.payments
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'payments' 
        AND policyname = 'payments_insert_authenticated'
    ) THEN
        CREATE POLICY payments_insert_authenticated ON public.payments
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'payments' 
        AND policyname = 'payments_update_authenticated'
    ) THEN
        CREATE POLICY payments_update_authenticated ON public.payments
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.payments TO authenticated;

-- Indexes for payments
CREATE INDEX IF NOT EXISTS idx_payments_business_entity_id ON public.payments(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_payments_branch_id ON public.payments(branch_id);
CREATE INDEX IF NOT EXISTS idx_payments_party_id ON public.payments(party_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON public.payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_payment_date ON public.payments(payment_date);
CREATE INDEX IF NOT EXISTS idx_payments_received_by_user_id ON public.payments(received_by_user_id);

-- Updated at trigger for payments
CREATE OR REPLACE FUNCTION update_payments_updated_at()
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
        WHERE tgname = 'payments_updated_at'
    ) THEN
        CREATE TRIGGER payments_updated_at
            BEFORE UPDATE ON public.payments
            FOR EACH ROW
            EXECUTE FUNCTION update_payments_updated_at();
    END IF;
END $$;

-- Immutability trigger: payments
-- Prevent modification of amount_received, party_id, payment_date when status = 'posted'
CREATE OR REPLACE FUNCTION prevent_payment_mutation() RETURNS trigger AS $$
BEGIN
    IF OLD.status = 'posted' THEN
        IF NEW.amount_received != OLD.amount_received THEN
            RAISE EXCEPTION 'cannot modify amount_received on posted payment';
        END IF;
        IF NEW.party_id != OLD.party_id THEN
            RAISE EXCEPTION 'cannot modify party_id on posted payment';
        END IF;
        IF NEW.payment_date != OLD.payment_date THEN
            RAISE EXCEPTION 'cannot modify payment_date on posted payment';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_prevent_payment_mutation'
    ) THEN
        CREATE TRIGGER trg_prevent_payment_mutation
            BEFORE UPDATE ON public.payments
            FOR EACH ROW EXECUTE FUNCTION prevent_payment_mutation();
    END IF;
END $$;

-- ============================================================================
-- PAYMENT_ALLOCATIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.payment_allocations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id       UUID NOT NULL REFERENCES public.payments(id),
    receivable_id    UUID NOT NULL REFERENCES public.receivables(id),
    allocated_amount NUMERIC(18,2) NOT NULL,
    allocation_date  DATE NOT NULL,
    allocation_order SMALLINT NOT NULL DEFAULT 1,
    notes            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT payment_allocations_payment_receivable_key UNIQUE (payment_id, receivable_id),
    CONSTRAINT payment_allocations_amount_positive CHECK (allocated_amount > 0)
);

-- Enable RLS on payment_allocations
ALTER TABLE public.payment_allocations ENABLE ROW LEVEL SECURITY;

-- RLS Policies for payment_allocations (SELECT + INSERT only, no UPDATE)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'payment_allocations' 
        AND policyname = 'payment_allocations_select_authenticated'
    ) THEN
        CREATE POLICY payment_allocations_select_authenticated ON public.payment_allocations
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'payment_allocations' 
        AND policyname = 'payment_allocations_insert_authenticated'
    ) THEN
        CREATE POLICY payment_allocations_insert_authenticated ON public.payment_allocations
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (SELECT + INSERT only, append-only)
GRANT SELECT, INSERT ON public.payment_allocations TO authenticated;

-- Indexes for payment_allocations
CREATE INDEX IF NOT EXISTS idx_payment_allocations_receivable_id ON public.payment_allocations(receivable_id);

-- ============================================================================
-- FINANCIAL_ADJUSTMENTS TABLE (must be before credit_balances due to FK)
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.financial_adjustments (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id   UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id            UUID NOT NULL REFERENCES public.branches(id),
    party_id             UUID REFERENCES public.parties(id),
    source_module        TEXT NOT NULL,
    source_record_type   TEXT NOT NULL,
    source_record_id     UUID NOT NULL,
    adjustment_type      TEXT NOT NULL,
    amount               NUMERIC(18,2) NOT NULL,
    currency_code        TEXT NOT NULL DEFAULT 'USD',
    effective_date       DATE NOT NULL,
    status               TEXT NOT NULL DEFAULT 'pending',
    reason               TEXT NOT NULL,
    requested_by_user_id UUID NOT NULL REFERENCES auth.users(id),
    approved_by_user_id  UUID REFERENCES auth.users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT financial_adjustments_adjustment_type_check CHECK (adjustment_type IN ('debit', 'credit', 'waiver', 'penalty', 'correction')),
    CONSTRAINT financial_adjustments_status_check CHECK (status IN ('pending', 'approved', 'applied', 'rejected')),
    CONSTRAINT financial_adjustments_amount_positive CHECK (amount > 0)
);

-- Enable RLS on financial_adjustments
ALTER TABLE public.financial_adjustments ENABLE ROW LEVEL SECURITY;

-- RLS Policies for financial_adjustments
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'financial_adjustments' 
        AND policyname = 'financial_adjustments_select_authenticated'
    ) THEN
        CREATE POLICY financial_adjustments_select_authenticated ON public.financial_adjustments
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'financial_adjustments' 
        AND policyname = 'financial_adjustments_insert_authenticated'
    ) THEN
        CREATE POLICY financial_adjustments_insert_authenticated ON public.financial_adjustments
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'financial_adjustments' 
        AND policyname = 'financial_adjustments_update_authenticated'
    ) THEN
        CREATE POLICY financial_adjustments_update_authenticated ON public.financial_adjustments
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.financial_adjustments TO authenticated;

-- Indexes for financial_adjustments
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_business_entity_id ON public.financial_adjustments(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_branch_id ON public.financial_adjustments(branch_id);
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_party_id ON public.financial_adjustments(party_id) WHERE party_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_status ON public.financial_adjustments(status);
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_requested_by ON public.financial_adjustments(requested_by_user_id);
CREATE INDEX IF NOT EXISTS idx_financial_adjustments_approved_by ON public.financial_adjustments(approved_by_user_id) WHERE approved_by_user_id IS NOT NULL;

-- Updated at trigger for financial_adjustments
CREATE OR REPLACE FUNCTION update_financial_adjustments_updated_at()
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
        WHERE tgname = 'financial_adjustments_updated_at'
    ) THEN
        CREATE TRIGGER financial_adjustments_updated_at
            BEFORE UPDATE ON public.financial_adjustments
            FOR EACH ROW
            EXECUTE FUNCTION update_financial_adjustments_updated_at();
    END IF;
END $$;

-- ============================================================================
-- CREDIT_BALANCES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.credit_balances (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id   UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id            UUID NOT NULL REFERENCES public.branches(id),
    party_id             UUID NOT NULL REFERENCES public.parties(id),
    source_module        TEXT,
    source_record_type   TEXT,
    source_record_id     UUID,
    origin_payment_id    UUID REFERENCES public.payments(id),
    origin_adjustment_id UUID REFERENCES public.financial_adjustments(id),
    currency_code        TEXT NOT NULL DEFAULT 'USD',
    amount_total         NUMERIC(18,2) NOT NULL,
    amount_used          NUMERIC(18,2) NOT NULL DEFAULT 0,
    amount_remaining     NUMERIC(18,2) GENERATED ALWAYS AS (amount_total - amount_used) STORED,
    status               TEXT NOT NULL DEFAULT 'available',
    reason               TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT credit_balances_status_check CHECK (status IN ('available', 'exhausted', 'cancelled')),
    CONSTRAINT credit_balances_amount_total_positive CHECK (amount_total > 0)
);

-- Enable RLS on credit_balances
ALTER TABLE public.credit_balances ENABLE ROW LEVEL SECURITY;

-- RLS Policies for credit_balances
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'credit_balances' 
        AND policyname = 'credit_balances_select_authenticated'
    ) THEN
        CREATE POLICY credit_balances_select_authenticated ON public.credit_balances
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'credit_balances' 
        AND policyname = 'credit_balances_insert_authenticated'
    ) THEN
        CREATE POLICY credit_balances_insert_authenticated ON public.credit_balances
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'credit_balances' 
        AND policyname = 'credit_balances_update_authenticated'
    ) THEN
        CREATE POLICY credit_balances_update_authenticated ON public.credit_balances
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.credit_balances TO authenticated;

-- Indexes for credit_balances
CREATE INDEX IF NOT EXISTS idx_credit_balances_business_entity_id ON public.credit_balances(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_credit_balances_branch_id ON public.credit_balances(branch_id);
CREATE INDEX IF NOT EXISTS idx_credit_balances_party_status ON public.credit_balances(party_id, status);
CREATE INDEX IF NOT EXISTS idx_credit_balances_origin_payment_id ON public.credit_balances(origin_payment_id) WHERE origin_payment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_credit_balances_origin_adjustment_id ON public.credit_balances(origin_adjustment_id) WHERE origin_adjustment_id IS NOT NULL;

-- Updated at trigger for credit_balances
CREATE OR REPLACE FUNCTION update_credit_balances_updated_at()
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
        WHERE tgname = 'credit_balances_updated_at'
    ) THEN
        CREATE TRIGGER credit_balances_updated_at
            BEFORE UPDATE ON public.credit_balances
            FOR EACH ROW
            EXECUTE FUNCTION update_credit_balances_updated_at();
    END IF;
END $$;

-- Notify PostgREST to reload schema
NOTIFY pgrst, 'reload schema';
