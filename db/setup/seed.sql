-- seed.sql
-- !! DEV ONLY — This user and password must NOT exist in production !!
-- Seeds development users into GoTrue's database table

-- Ensure pgcrypto is enabled so we can natively hash the passwords with bcrypt
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Insert dev users into GoTrue's auth.users table
-- We use fixed UUIDs for users to ensure consistency
DO $$
DECLARE
  rawand_user_id UUID := 'e9fe80dd-56a0-4afb-a824-f5c62098509a';
  e2e_user_id UUID := 'f8a91b2e-3d4c-5e6f-7a8b-9c0d1e2f3a4b';
BEGIN
  -- Create rawand@ioi.dev user
  IF NOT EXISTS (SELECT 1 FROM auth.users WHERE email = 'rawand@ioi.dev') THEN
    INSERT INTO auth.users (
        instance_id, id, aud, role, email, encrypted_password, email_confirmed_at,
        raw_app_meta_data, raw_user_meta_data, created_at, updated_at
    ) VALUES (
        '00000000-0000-0000-0000-000000000000', rawand_user_id, 'authenticated', 'authenticated',
        'rawand@ioi.dev', crypt('4buvjbg1uzwcaoTx', gen_salt('bf')), timezone('utc'::text, now()),
        '{"provider":"email","providers":["email"]}', '{}', timezone('utc'::text, now()), timezone('utc'::text, now())
    );

    INSERT INTO auth.identities (
        id, user_id, identity_data, provider, provider_id, last_sign_in_at, created_at, updated_at
    ) VALUES (
        gen_random_uuid(), rawand_user_id, 
        format('{"sub":"%s","email":"rawand@ioi.dev"}', rawand_user_id)::jsonb,
        'email', rawand_user_id, timezone('utc'::text, now()), timezone('utc'::text, now()), timezone('utc'::text, now())
    );
  END IF;

  -- Create e2e@ioi.dev user (used by tests)
  IF NOT EXISTS (SELECT 1 FROM auth.users WHERE email = 'e2e@ioi.dev') THEN
    INSERT INTO auth.users (
        instance_id, id, aud, role, email, encrypted_password, email_confirmed_at,
        raw_app_meta_data, raw_user_meta_data, created_at, updated_at
    ) VALUES (
        '00000000-0000-0000-0000-000000000000', e2e_user_id, 'authenticated', 'authenticated',
        'e2e@ioi.dev', crypt('testing12345!', gen_salt('bf')), timezone('utc'::text, now()),
        '{"provider":"email","providers":["email"]}', '{}', timezone('utc'::text, now()), timezone('utc'::text, now())
    );

    INSERT INTO auth.identities (
        id, user_id, identity_data, provider, provider_id, last_sign_in_at, created_at, updated_at
    ) VALUES (
        gen_random_uuid(), e2e_user_id, 
        format('{"sub":"%s","email":"e2e@ioi.dev"}', e2e_user_id)::jsonb,
        'email', e2e_user_id, timezone('utc'::text, now()), timezone('utc'::text, now()), timezone('utc'::text, now())
    );
  END IF;
END $$;

-- Assign system_admin role to rawand@ioi.dev for testing
INSERT INTO public.user_role_scope_assignments (user_id, role_id, scope_type, scope_id)
SELECT 
    u.id,
    r.id,
    'deployment',
    NULL
FROM auth.users u
CROSS JOIN public.roles r
WHERE u.email = 'rawand@ioi.dev'
  AND r.code = 'system_admin'
ON CONFLICT (user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000')) DO NOTHING;

-- Assign system_admin role to e2e@ioi.dev for testing
INSERT INTO public.user_role_scope_assignments (user_id, role_id, scope_type, scope_id)
SELECT 
    u.id,
    r.id,
    'deployment',
    NULL
FROM auth.users u
CROSS JOIN public.roles r
WHERE u.email = 'e2e@ioi.dev'
  AND r.code = 'system_admin'
ON CONFLICT (user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000')) DO NOTHING;
