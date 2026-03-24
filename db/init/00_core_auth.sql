-- =============================================================================
-- PostgREST Roles
-- =============================================================================
-- authenticator: PostgREST logs in as this role, then switches per JWT
-- web_anon: unauthenticated requests (no table access by default)
-- authenticated: verified JWT holders (RLS governs row-level access)
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'authenticator') THEN
        CREATE ROLE authenticator NOINHERIT LOGIN PASSWORD 'postgrest_dev_password';
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'web_anon') THEN
        CREATE ROLE web_anon NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'authenticated') THEN
        CREATE ROLE authenticated NOLOGIN;
    END IF;
END
$$;

GRANT web_anon TO authenticator;
GRANT authenticated TO authenticator;

-- Grant schema access
GRANT USAGE ON SCHEMA public TO web_anon, authenticated;

-- Auto-grant table/sequence privileges to authenticated for all future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO authenticated;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE ON SEQUENCES TO authenticated;

-- =============================================================================
-- Auth Schema & Helpers
-- =============================================================================
-- GoTrue creates auth.users when it first connects (AFTER postgres init).
-- We create the schema and helper functions here. The profiles table and
-- trigger that depend on auth.users are created via `make db-setup`.
-- =============================================================================
CREATE SCHEMA IF NOT EXISTS auth;

-- Helper function: Extract the User ID from the JWT
-- Both Fiber (GUC injection) and PostgREST set request.jwt.claim.sub
CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
  SELECT NULLIF(current_setting('request.jwt.claim.sub', true), '')::UUID;
$$ LANGUAGE sql STABLE;

-- Helper function: Extract the Role from the JWT
CREATE OR REPLACE FUNCTION auth.role() RETURNS TEXT AS $$
  SELECT COALESCE(NULLIF(current_setting('request.jwt.claim.role', true), ''), 'anon');
$$ LANGUAGE sql STABLE;
