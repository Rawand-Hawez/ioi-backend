-- Authorization Domain Schema
-- This file creates the roles, permissions, role_permissions, and user_role_scope_assignments tables

-- ============================================================================
-- ROLES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Enable RLS on roles
ALTER TABLE public.roles ENABLE ROW LEVEL SECURITY;

-- RLS Policies for roles
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'roles' 
        AND policyname = 'roles_select_authenticated'
    ) THEN
        CREATE POLICY roles_select_authenticated ON public.roles
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'roles' 
        AND policyname = 'roles_insert_authenticated'
    ) THEN
        CREATE POLICY roles_insert_authenticated ON public.roles
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'roles' 
        AND policyname = 'roles_update_authenticated'
    ) THEN
        CREATE POLICY roles_update_authenticated ON public.roles
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.roles TO authenticated;

-- Indexes for roles
CREATE INDEX IF NOT EXISTS idx_roles_is_active ON public.roles (is_active);

-- Updated at trigger for roles
CREATE OR REPLACE FUNCTION update_roles_updated_at()
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
        WHERE tgname = 'roles_updated_at'
    ) THEN
        CREATE TRIGGER roles_updated_at
            BEFORE UPDATE ON public.roles
            FOR EACH ROW
            EXECUTE FUNCTION update_roles_updated_at();
    END IF;
END $$;

-- ============================================================================
-- PERMISSIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL UNIQUE,
    module TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Comments for documentation
COMMENT ON COLUMN public.permissions.key IS 'Machine-readable permission identifier, e.g. sales.contract.activate';
COMMENT ON COLUMN public.permissions.module IS 'Domain classifier: inventory, sales, rentals, finance, sc, utility, approvals, admin';

-- Enable RLS on permissions
ALTER TABLE public.permissions ENABLE ROW LEVEL SECURITY;

-- RLS Policies for permissions
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'permissions' 
        AND policyname = 'permissions_select_authenticated'
    ) THEN
        CREATE POLICY permissions_select_authenticated ON public.permissions
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'permissions' 
        AND policyname = 'permissions_insert_authenticated'
    ) THEN
        CREATE POLICY permissions_insert_authenticated ON public.permissions
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'permissions' 
        AND policyname = 'permissions_update_authenticated'
    ) THEN
        CREATE POLICY permissions_update_authenticated ON public.permissions
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.permissions TO authenticated;

-- Indexes for permissions
CREATE INDEX IF NOT EXISTS idx_permissions_module ON public.permissions (module);

-- ============================================================================
-- ROLE_PERMISSIONS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES public.roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES public.permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    UNIQUE (role_id, permission_id)
);

-- Enable RLS on role_permissions
ALTER TABLE public.role_permissions ENABLE ROW LEVEL SECURITY;

-- RLS Policies for role_permissions
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'role_permissions' 
        AND policyname = 'role_permissions_select_authenticated'
    ) THEN
        CREATE POLICY role_permissions_select_authenticated ON public.role_permissions
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'role_permissions' 
        AND policyname = 'role_permissions_insert_authenticated'
    ) THEN
        CREATE POLICY role_permissions_insert_authenticated ON public.role_permissions
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'role_permissions' 
        AND policyname = 'role_permissions_update_authenticated'
    ) THEN
        CREATE POLICY role_permissions_update_authenticated ON public.role_permissions
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE ON public.role_permissions TO authenticated;

-- Indexes for role_permissions
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON public.role_permissions (permission_id);

-- ============================================================================
-- USER_ROLE_SCOPE_ASSIGNMENTS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS public.user_role_scope_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES public.roles(id) ON DELETE RESTRICT,
    scope_type TEXT NOT NULL CHECK (scope_type IN ('deployment', 'business_entity', 'branch', 'project')),
    scope_id UUID,
    created_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT timezone('utc', now()) NOT NULL
);

-- Enable RLS on user_role_scope_assignments
ALTER TABLE public.user_role_scope_assignments ENABLE ROW LEVEL SECURITY;

-- RLS Policies for user_role_scope_assignments
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'user_role_scope_assignments' 
        AND policyname = 'user_role_scope_assignments_select_authenticated'
    ) THEN
        CREATE POLICY user_role_scope_assignments_select_authenticated ON public.user_role_scope_assignments
            FOR SELECT TO authenticated
            USING (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'user_role_scope_assignments' 
        AND policyname = 'user_role_scope_assignments_insert_authenticated'
    ) THEN
        CREATE POLICY user_role_scope_assignments_insert_authenticated ON public.user_role_scope_assignments
            FOR INSERT TO authenticated
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'user_role_scope_assignments' 
        AND policyname = 'user_role_scope_assignments_update_authenticated'
    ) THEN
        CREATE POLICY user_role_scope_assignments_update_authenticated ON public.user_role_scope_assignments
            FOR UPDATE TO authenticated
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE tablename = 'user_role_scope_assignments' 
        AND policyname = 'user_role_scope_assignments_delete_authenticated'
    ) THEN
        CREATE POLICY user_role_scope_assignments_delete_authenticated ON public.user_role_scope_assignments
            FOR DELETE TO authenticated
            USING (true);
    END IF;
END $$;

-- Grant permissions to authenticated role
GRANT SELECT, INSERT, UPDATE, DELETE ON public.user_role_scope_assignments TO authenticated;

-- Indexes for user_role_scope_assignments
CREATE INDEX IF NOT EXISTS idx_user_role_scope_assignments_role_id ON public.user_role_scope_assignments (role_id);
CREATE INDEX IF NOT EXISTS idx_user_role_scope_assignments_scope ON public.user_role_scope_assignments (scope_type, scope_id) WHERE scope_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_role_scope_unique ON public.user_role_scope_assignments (user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'));

-- Updated at trigger for user_role_scope_assignments
CREATE OR REPLACE FUNCTION update_user_role_scope_assignments_updated_at()
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
        WHERE tgname = 'user_role_scope_assignments_updated_at'
    ) THEN
        CREATE TRIGGER user_role_scope_assignments_updated_at
            BEFORE UPDATE ON public.user_role_scope_assignments
            FOR EACH ROW
            EXECUTE FUNCTION update_user_role_scope_assignments_updated_at();
    END IF;
END $$;

-- ============================================================================
-- SEED DATA: ROLES
-- ============================================================================
INSERT INTO public.roles (code, name, description) VALUES
    ('system_admin', 'System Administrator', 'Full system access across all modules'),
    ('business_entity_admin', 'Business Entity Admin', 'Full access within one business entity'),
    ('branch_manager', 'Branch Manager', 'Operational management within one branch'),
    ('sales_officer', 'Sales Officer', 'Create and manage sales transactions'),
    ('sales_approver', 'Sales Approver', 'Approve sales transfers and cancellations'),
    ('leasing_officer', 'Leasing Officer', 'Create and manage lease contracts'),
    ('leasing_approver', 'Leasing Approver', 'Approve lease terminations and deposit refunds'),
    ('cashier', 'Cashier', 'Record and post payments'),
    ('finance_officer', 'Finance Officer', 'Create adjustments and manual receivables'),
    ('finance_approver', 'Finance Approver', 'Approve financial adjustments and payment voids'),
    ('property_service_manager', 'Property Service Manager', 'Manage service charge rules and utility billing'),
    ('reporting_user', 'Reporting User', 'Read-only reporting access')
ON CONFLICT (code) DO NOTHING;

-- ============================================================================
-- SEED DATA: PERMISSIONS
-- ============================================================================
INSERT INTO public.permissions (key, module, description) VALUES
    -- Inventory
    ('inventory.unit.view', 'inventory', 'View units'),
    ('inventory.unit.create', 'inventory', 'Create units'),
    ('inventory.unit.edit', 'inventory', 'Edit unit attributes'),
    ('inventory.unit.edit_code', 'inventory', 'Edit unit code (elevated)'),
    -- Sales
    ('sales.reservation.create', 'sales', 'Create reservations'),
    ('sales.contract.create', 'sales', 'Create sales contracts'),
    ('sales.contract.activate', 'sales', 'Activate sales contracts'),
    ('sales.contract.cancel', 'sales', 'Cancel sales contracts'),
    ('sales.transfer.request', 'sales', 'Request ownership transfers'),
    ('sales.transfer.approve', 'sales', 'Approve ownership transfers'),
    ('sales.reservation.cancel', 'sales', 'Cancel reservations'),
    ('sales.reservation.convert', 'sales', 'Convert reservations into contracts'),
    ('sales.contract.edit', 'sales', 'Edit draft sales contracts'),
    ('sales.contract.terminate', 'sales', 'Request or apply contract termination'),
    ('sales.contract.complete', 'sales', 'Complete active sales contracts'),
    ('sales.contract.mark_default', 'sales', 'Mark active sales contracts as defaulted'),
    ('sales.schedule.generate', 'sales', 'Generate contract installment schedules'),
    ('sales.schedule.edit', 'sales', 'Edit draft schedules or submit restructure requests'),
    ('sales.ownership.transfer', 'sales', 'Request ownership transfers'),
    ('sales.ownership.transfer.complete', 'sales', 'Complete ownership transfers'),
    ('sales.payment_plan.create', 'sales', 'Create payment plan templates'),
    -- Rentals
    ('rentals.lease.create', 'rentals', 'Create lease contracts'),
    ('rentals.lease.activate', 'rentals', 'Activate lease contracts'),
    ('rentals.lease.terminate', 'rentals', 'Terminate lease contracts'),
    -- Finance
    ('finance.payment.create', 'finance', 'Create payments'),
    ('finance.payment.post', 'finance', 'Post payments'),
    ('finance.payment.void', 'finance', 'Void posted payments'),
    ('finance.receivable.manual', 'finance', 'Create manual receivables'),
    ('finance.adjustment.create', 'finance', 'Create financial adjustments'),
    ('finance.adjustment.approve', 'finance', 'Approve financial adjustments'),
    ('finance.credit.apply', 'finance', 'Apply credit balances'),
    -- Service Charges
    ('sc.rule.create', 'sc', 'Create service charge rules'),
    ('sc.rule.edit', 'sc', 'Edit service charge rules'),
    ('sc.bills.generate', 'sc', 'Generate service charge bills'),
    -- Utilities
    ('utility.service.create', 'utility', 'Create utility services'),
    ('utility.prepaid.topup', 'utility', 'Top up prepaid utility balances'),
    ('utility.prepaid.adjust', 'utility', 'Adjust prepaid utility balances'),
    -- Approvals
    ('approvals.request.decide', 'approvals', 'Decide on approval requests'),
    ('approvals.policy.manage', 'approvals', 'Manage approval policies'),
    -- Admin
    ('admin.users.manage', 'admin', 'Manage user accounts'),
    ('admin.roles.assign', 'admin', 'Assign roles to users'),
    -- Audit
    ('audit.log.view', 'audit', 'View audit logs')
ON CONFLICT (key) DO NOTHING;

-- ============================================================================
-- SEED DATA: ROLE-PERMISSION MAPPINGS
-- ============================================================================

-- system_admin: ALL 30 permissions
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'system_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- business_entity_admin: All EXCEPT admin.users.manage, admin.roles.assign
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'business_entity_admin'
  AND p.key NOT IN ('admin.users.manage', 'admin.roles.assign')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- branch_manager: inventory.unit.*, sales.reservation.create, sales.reservation.cancel, sales.reservation.convert, sales.contract.create, sales.contract.edit, sales.schedule.generate, sales.schedule.edit, rentals.lease.create, finance.payment.create, finance.payment.post, sc.*, utility.*
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'branch_manager'
  AND (p.key LIKE 'inventory.unit.%' OR p.key IN ('sales.reservation.create', 'sales.reservation.cancel', 'sales.reservation.convert', 'sales.contract.create', 'sales.contract.edit', 'sales.schedule.generate', 'sales.schedule.edit', 'rentals.lease.create', 'finance.payment.create', 'finance.payment.post') OR p.key LIKE 'sc.%' OR p.key LIKE 'utility.%')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- sales_officer: inventory.unit.view, sales.reservation.create, sales.reservation.cancel, sales.reservation.convert, sales.contract.create, sales.contract.edit, sales.contract.activate, sales.transfer.request, sales.ownership.transfer, sales.schedule.generate, sales.schedule.edit, sales.payment_plan.create, finance.payment.create
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'sales_officer'
  AND p.key IN ('inventory.unit.view', 'sales.reservation.create', 'sales.reservation.cancel', 'sales.reservation.convert', 'sales.contract.create', 'sales.contract.edit', 'sales.contract.activate', 'sales.transfer.request', 'sales.ownership.transfer', 'sales.schedule.generate', 'sales.schedule.edit', 'sales.payment_plan.create', 'finance.payment.create')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- sales_approver: inventory.unit.view, sales.transfer.approve, sales.ownership.transfer.complete, sales.contract.cancel, sales.contract.terminate, approvals.request.decide
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'sales_approver'
  AND p.key IN ('inventory.unit.view', 'sales.transfer.approve', 'sales.ownership.transfer.complete', 'sales.contract.cancel', 'sales.contract.terminate', 'approvals.request.decide')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- leasing_officer: inventory.unit.view, rentals.lease.create, rentals.lease.activate, finance.payment.create
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'leasing_officer'
  AND p.key IN ('inventory.unit.view', 'rentals.lease.create', 'rentals.lease.activate', 'finance.payment.create')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- leasing_approver: inventory.unit.view, rentals.lease.terminate, approvals.request.decide
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'leasing_approver'
  AND p.key IN ('inventory.unit.view', 'rentals.lease.terminate', 'approvals.request.decide')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- cashier: inventory.unit.view, finance.payment.create, finance.payment.post
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'cashier'
  AND p.key IN ('inventory.unit.view', 'finance.payment.create', 'finance.payment.post')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- finance_officer: inventory.unit.view, sales.contract.complete, finance.payment.create, finance.payment.post, finance.receivable.manual, finance.adjustment.create, finance.credit.apply
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'finance_officer'
  AND p.key IN ('inventory.unit.view', 'sales.contract.complete', 'finance.payment.create', 'finance.payment.post', 'finance.receivable.manual', 'finance.adjustment.create', 'finance.credit.apply')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- finance_approver: inventory.unit.view, sales.contract.mark_default, finance.payment.void, finance.adjustment.approve, approvals.request.decide
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'finance_approver'
  AND p.key IN ('inventory.unit.view', 'sales.contract.mark_default', 'finance.payment.void', 'finance.adjustment.approve', 'approvals.request.decide')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- property_service_manager: inventory.unit.view, sc.*, utility.*
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'property_service_manager'
  AND (p.key = 'inventory.unit.view' OR p.key LIKE 'sc.%' OR p.key LIKE 'utility.%')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- reporting_user: inventory.unit.view only
INSERT INTO public.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM public.roles r, public.permissions p
WHERE r.code = 'reporting_user'
  AND p.key = 'inventory.unit.view'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Notify PostgREST to reload schema
NOTIFY pgrst, 'reload schema';
