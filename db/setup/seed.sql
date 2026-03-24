-- 02_seed.sql
-- This scripts securely seeds the development user into GoTrue's database table

-- Ensure pgcrypto is enabled so we can natively hash the passwords with bcrypt
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Insert the test user into GoTrue's auth.users table
-- We use a fixed UUID for the user to ensure consistency
DO $$
DECLARE
  user_id UUID := 'e9fe80dd-56a0-4afb-a824-f5c62098509a';
BEGIN
  IF NOT EXISTS (SELECT 1 FROM auth.users WHERE email = 'rawand@ioi.dev') THEN
    INSERT INTO auth.users (
        instance_id, id, aud, role, email, encrypted_password, email_confirmed_at,
        raw_app_meta_data, raw_user_meta_data, created_at, updated_at
    ) VALUES (
        '00000000-0000-0000-0000-000000000000', user_id, 'authenticated', 'authenticated',
        'rawand@ioi.dev', crypt('4buvjbg1uzwcaoTx', gen_salt('bf')), timezone('utc'::text, now()),
        '{"provider":"email","providers":["email"]}', '{}', timezone('utc'::text, now()), timezone('utc'::text, now())
    );

    -- Also insert into auth.identities to satisfy GoTrue's internal integrity checks
    INSERT INTO auth.identities (
        id, user_id, identity_data, provider, provider_id, last_sign_in_at, created_at, updated_at
    ) VALUES (
        gen_random_uuid(), user_id, 
        format('{"sub":"%s","email":"rawand@ioi.dev"}', user_id)::jsonb,
        'email', user_id, timezone('utc'::text, now()), timezone('utc'::text, now()), timezone('utc'::text, now())
    );
  END IF;
END $$;
