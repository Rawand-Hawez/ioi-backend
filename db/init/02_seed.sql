-- 02_seed.sql
-- This scripts securely seeds the development user into GoTrue's database table

-- Ensure pgcrypto is enabled so we can natively hash the passwords with bcrypt
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Insert the test user into GoTrue's auth.users table
-- GoTrue will recognize this user natively since the password is mathematically hashed.
INSERT INTO auth.users (
    instance_id,
    id,
    aud,
    role,
    email,
    encrypted_password,
    email_confirmed_at,
    raw_app_meta_data,
    raw_user_meta_data,
    created_at,
    updated_at
) VALUES (
    '00000000-0000-0000-0000-000000000000',
    gen_random_uuid(),
    'authenticated',
    'authenticated',
    'rawand@ioi.dev',
    crypt('4buvjbg1uzwcaoTx', gen_salt('bf')),
    timezone('utc'::text, now()),
    '{"provider":"email","providers":["email"]}',
    '{}',
    timezone('utc'::text, now()),
    timezone('utc'::text, now())
);
