-- Approvals + Audit Domain Schema
-- This file creates the approval_policies, approval_requests, approval_request_approvers, and audit_logs tables

-- ============================================================================
-- APPROVAL_POLICIES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.approval_policies (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id        UUID NOT NULL REFERENCES public.business_entities(id),
    code                      TEXT NOT NULL,
    name                      TEXT NOT NULL,
    module                    TEXT NOT NULL,
    request_type              TEXT NOT NULL,
    min_approvers             SMALLINT NOT NULL DEFAULT 1,
    prevent_self_approval     BOOLEAN NOT NULL DEFAULT true,
    approver_role_id          UUID REFERENCES public.roles(id),
    auto_approve_below_amount NUMERIC(18,2),
    expiry_hours              INTEGER,
    is_active                 BOOLEAN NOT NULL DEFAULT true,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT approval_policies_module_check CHECK (module IN ('sales', 'finance', 'rentals', 'service_charges', 'utilities')),
    CONSTRAINT approval_policies_request_type_check CHECK (request_type IN ('ownership_transfer', 'payment_void', 'deposit_refund', 'contract_cancellation', 'contract_termination', 'schedule_restructure', 'financial_adjustment', 'lease_termination', 'manual_override', 'prepaid_adjustment')),
    CONSTRAINT approval_policies_code_unique UNIQUE (business_entity_id, code)
);

-- Enable RLS on approval_policies
ALTER TABLE public.approval_policies ENABLE ROW LEVEL SECURITY;

-- RLS Policies for approval_policies
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_policies' 
        AND policyname = 'approval_policies_select_authenticated'
    ) THEN
        CREATE POLICY approval_policies_select_authenticated ON public.approval_policies
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_policies' 
        AND policyname = 'approval_policies_insert_authenticated'
    ) THEN
        CREATE POLICY approval_policies_insert_authenticated ON public.approval_policies
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_policies' 
        AND policyname = 'approval_policies_update_authenticated'
    ) THEN
        CREATE POLICY approval_policies_update_authenticated ON public.approval_policies
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.approval_policies TO authenticated;

-- Indexes for approval_policies
CREATE INDEX IF NOT EXISTS idx_approval_policies_business_entity_id ON public.approval_policies(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_approval_policies_approver_role_id ON public.approval_policies(approver_role_id) WHERE approver_role_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_policies_is_active ON public.approval_policies(is_active);

-- Updated at trigger for approval_policies
CREATE OR REPLACE FUNCTION update_approval_policies_updated_at()
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
        WHERE tgname = 'approval_policies_updated_at'
    ) THEN
        CREATE TRIGGER approval_policies_updated_at
            BEFORE UPDATE ON public.approval_policies
            FOR EACH ROW
            EXECUTE FUNCTION update_approval_policies_updated_at();
    END IF;
END $$;

-- ============================================================================
-- APPROVAL_REQUESTS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.approval_requests (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_entity_id    UUID NOT NULL REFERENCES public.business_entities(id),
    branch_id             UUID REFERENCES public.branches(id),
    approval_policy_id    UUID REFERENCES public.approval_policies(id),
    module                TEXT NOT NULL,
    request_type          TEXT NOT NULL,
    source_record_type    TEXT NOT NULL,
    source_record_id      UUID NOT NULL,
    requested_by_user_id  UUID NOT NULL REFERENCES auth.users(id),
    assigned_to_user_id   UUID REFERENCES auth.users(id),
    status                TEXT NOT NULL DEFAULT 'pending',
    submitted_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    decided_at            TIMESTAMPTZ,
    decision_reason       TEXT,
    payload_snapshot_json JSONB NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT approval_requests_status_check CHECK (status IN ('draft', 'pending', 'approved', 'rejected', 'cancelled', 'expired')),
    CONSTRAINT approval_requests_module_check CHECK (module IN ('sales', 'finance', 'rentals', 'service_charges', 'utilities')),
    CONSTRAINT approval_requests_request_type_check CHECK (request_type IN ('ownership_transfer', 'payment_void', 'deposit_refund', 'contract_cancellation', 'contract_termination', 'schedule_restructure', 'financial_adjustment', 'lease_termination', 'manual_override', 'prepaid_adjustment'))
);

-- Migration: Add CHECK constraints to approval_requests if they don't exist (for existing databases)
DO $$
BEGIN
    -- Add module check constraint if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'approval_requests_module_check' 
        AND conrelid = 'public.approval_requests'::regclass
    ) THEN
        ALTER TABLE public.approval_requests
        ADD CONSTRAINT approval_requests_module_check 
        CHECK (module IN ('sales', 'finance', 'rentals', 'service_charges', 'utilities'));
    END IF;
END $$;

DO $$
BEGIN
    -- Add request_type check constraint if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'approval_requests_request_type_check' 
        AND conrelid = 'public.approval_requests'::regclass
    ) THEN
        ALTER TABLE public.approval_requests
        ADD CONSTRAINT approval_requests_request_type_check 
        CHECK (request_type IN ('ownership_transfer', 'payment_void', 'deposit_refund', 'contract_cancellation', 'contract_termination', 'schedule_restructure', 'financial_adjustment', 'lease_termination', 'manual_override', 'prepaid_adjustment'));
    END IF;
END $$;

-- Enable RLS on approval_requests
ALTER TABLE public.approval_requests ENABLE ROW LEVEL SECURITY;

-- RLS Policies for approval_requests
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_requests' 
        AND policyname = 'approval_requests_select_authenticated'
    ) THEN
        CREATE POLICY approval_requests_select_authenticated ON public.approval_requests
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_requests' 
        AND policyname = 'approval_requests_insert_authenticated'
    ) THEN
        CREATE POLICY approval_requests_insert_authenticated ON public.approval_requests
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_requests' 
        AND policyname = 'approval_requests_update_authenticated'
    ) THEN
        CREATE POLICY approval_requests_update_authenticated ON public.approval_requests
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.approval_requests TO authenticated;

-- Indexes for approval_requests
CREATE INDEX IF NOT EXISTS idx_approval_requests_business_entity_id ON public.approval_requests(business_entity_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_branch_id ON public.approval_requests(branch_id) WHERE branch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_requests_approval_policy_id ON public.approval_requests(approval_policy_id) WHERE approval_policy_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_requests_requested_by_user_id ON public.approval_requests(requested_by_user_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_assigned_to_user_id ON public.approval_requests(assigned_to_user_id) WHERE assigned_to_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_requests_status ON public.approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_requests_source ON public.approval_requests(source_record_type, source_record_id);

-- Updated at trigger for approval_requests
CREATE OR REPLACE FUNCTION update_approval_requests_updated_at()
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
        WHERE tgname = 'approval_requests_updated_at'
    ) THEN
        CREATE TRIGGER approval_requests_updated_at
            BEFORE UPDATE ON public.approval_requests
            FOR EACH ROW
            EXECUTE FUNCTION update_approval_requests_updated_at();
    END IF;
END $$;

-- ============================================================================
-- APPROVAL_REQUEST_APPROVERS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.approval_request_approvers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_request_id UUID NOT NULL REFERENCES public.approval_requests(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES auth.users(id),
    decision            TEXT,
    decided_at          TIMESTAMPTZ,
    decision_reason     TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT approval_request_approvers_decision_check CHECK (decision IS NULL OR decision IN ('approved', 'rejected')),
    CONSTRAINT approval_request_approvers_unique UNIQUE (approval_request_id, user_id)
);

-- Enable RLS on approval_request_approvers
ALTER TABLE public.approval_request_approvers ENABLE ROW LEVEL SECURITY;

-- RLS Policies for approval_request_approvers
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_request_approvers' 
        AND policyname = 'approval_request_approvers_select_authenticated'
    ) THEN
        CREATE POLICY approval_request_approvers_select_authenticated ON public.approval_request_approvers
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_request_approvers' 
        AND policyname = 'approval_request_approvers_insert_authenticated'
    ) THEN
        CREATE POLICY approval_request_approvers_insert_authenticated ON public.approval_request_approvers
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'approval_request_approvers' 
        AND policyname = 'approval_request_approvers_update_authenticated'
    ) THEN
        CREATE POLICY approval_request_approvers_update_authenticated ON public.approval_request_approvers
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (no DELETE)
GRANT SELECT, INSERT, UPDATE ON public.approval_request_approvers TO authenticated;

-- Indexes for approval_request_approvers
CREATE INDEX IF NOT EXISTS idx_approval_request_approvers_user_pending ON public.approval_request_approvers(user_id) WHERE decision IS NULL;

-- ============================================================================
-- AUDIT_LOGS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.audit_logs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_time                  TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    user_id                     UUID REFERENCES auth.users(id),
    module                      TEXT NOT NULL,
    action_type                 TEXT NOT NULL,
    entity_type                 TEXT NOT NULL,
    entity_id                   UUID NOT NULL,
    scope_type                  TEXT NOT NULL,
    scope_id                    UUID NOT NULL,
    result_status               TEXT NOT NULL,
    reason                      TEXT,
    summary_text                TEXT NOT NULL,
    before_snapshot_json        JSONB,
    after_snapshot_json         JSONB,
    related_approval_request_id UUID REFERENCES public.approval_requests(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),

    CONSTRAINT audit_logs_scope_type_check CHECK (scope_type IN ('deployment', 'business_entity', 'branch', 'project')),
    CONSTRAINT audit_logs_result_status_check CHECK (result_status IN ('success', 'failure', 'pending'))
);

-- Enable RLS on audit_logs
ALTER TABLE public.audit_logs ENABLE ROW LEVEL SECURITY;

-- RLS Policies for audit_logs
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'audit_logs' 
        AND policyname = 'audit_logs_select_authenticated'
    ) THEN
        CREATE POLICY audit_logs_select_authenticated ON public.audit_logs
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'audit_logs' 
        AND policyname = 'audit_logs_insert_authenticated'
    ) THEN
        CREATE POLICY audit_logs_insert_authenticated ON public.audit_logs
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role (SELECT + INSERT only, append-only)
GRANT SELECT, INSERT ON public.audit_logs TO authenticated;

-- Indexes for audit_logs
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON public.audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON public.audit_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_logs_event_time ON public.audit_logs(event_time);
CREATE INDEX IF NOT EXISTS idx_audit_logs_related_approval_request_id ON public.audit_logs(related_approval_request_id) WHERE related_approval_request_id IS NOT NULL;

-- Append-only trigger for audit_logs
CREATE OR REPLACE FUNCTION prevent_audit_log_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only: UPDATE and DELETE are not permitted';
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_prevent_audit_log_mutation'
    ) THEN
        CREATE TRIGGER trg_prevent_audit_log_mutation
            BEFORE UPDATE OR DELETE ON public.audit_logs
            FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();
    END IF;
END $$;

-- Notify PostgREST to reload schema
NOTIFY pgrst, 'reload schema';
