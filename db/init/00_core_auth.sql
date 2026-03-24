-- Create a schema for our custom auth helpers if it doesn't exist
CREATE SCHEMA IF NOT EXISTS auth;

-- GoTrue automatically creates the auth.users table.
-- We create a public.profiles table that mirrors it for our application logic.
CREATE TABLE public.profiles (
    id UUID REFERENCES auth.users (id) ON DELETE CASCADE PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    role TEXT DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone ('utc'::text, now()) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT timezone ('utc'::text, now()) NOT NULL
);

-- Enable Row Level Security on the profiles table
ALTER TABLE public.profiles ENABLE ROW LEVEL SECURITY;

-- Helper function: Extract the User ID from the JWT injected by pREST/Fiber
CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
  SELECT current_setting('request.jwt.claim.sub', true)::UUID;
$$ LANGUAGE sql STABLE;

-- Helper function: Extract the Role from the JWT injected by pREST/Fiber
CREATE OR REPLACE FUNCTION auth.role() RETURNS TEXT AS $$
  SELECT current_setting('request.jwt.claim.role', true)::TEXT;
$$ LANGUAGE sql STABLE;

-- Trigger Function: Automatically create a profile when a new GoTrue user signs up
CREATE OR REPLACE FUNCTION public.handle_new_user() 
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO public.profiles (id, email, role)
  VALUES (new.id, new.email, COALESCE(new.raw_app_meta_data->>'role', 'user'));
  RETURN new;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Bind the trigger to GoTrue's auth.users table
CREATE TRIGGER on_auth_user_created
  AFTER INSERT ON auth.users
  FOR EACH ROW EXECUTE PROCEDURE public.handle_new_user();

-- Core RLS Policy: Users can only read and update their own profile
CREATE POLICY "Users can view own profile" ON public.profiles FOR
SELECT USING (auth.uid () = id);

CREATE POLICY "Users can update own profile" ON public.profiles
FOR UPDATE
    USING (auth.uid () = id);