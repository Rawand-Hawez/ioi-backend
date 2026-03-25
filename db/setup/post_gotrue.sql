-- =============================================================================
-- Post-GoTrue Setup
-- =============================================================================
-- Run AFTER GoTrue has booted and created auth.users.
-- Command: make db-setup
--
-- Why this exists: PostgreSQL init scripts run BEFORE GoTrue starts.
-- Since GoTrue creates auth.users on first boot, anything that references
-- auth.users (profiles, FK constraints, triggers) must run after GoTrue.
-- =============================================================================

-- =============================================================================
-- 1. Profiles (mirrors auth.users for application logic)
-- =============================================================================
CREATE TABLE IF NOT EXISTS public.profiles (
    id UUID REFERENCES auth.users (id) ON DELETE CASCADE PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    role TEXT DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone ('utc'::text, now()) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT timezone ('utc'::text, now()) NOT NULL
);

ALTER TABLE public.profiles ENABLE ROW LEVEL SECURITY;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.profiles TO authenticated;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can view own profile' AND tablename = 'profiles') THEN
        CREATE POLICY "Users can view own profile" ON public.profiles FOR SELECT USING (auth.uid() = id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can update own profile' AND tablename = 'profiles') THEN
        CREATE POLICY "Users can update own profile" ON public.profiles FOR UPDATE USING (auth.uid() = id);
    END IF;
END $$;

-- Trigger: Mirror GoTrue signups into public.profiles
CREATE OR REPLACE FUNCTION public.handle_new_user()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO public.profiles (id, email, role)
  VALUES (new.id, new.email, 'user');
  RETURN new;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'on_auth_user_created') THEN
        CREATE TRIGGER on_auth_user_created
            AFTER INSERT ON auth.users
            FOR EACH ROW EXECUTE PROCEDURE public.handle_new_user();
    END IF;
END $$;

-- =============================================================================
-- 2. Application Schema (Domain-specific tables go here)
-- =============================================================================

-- Todos table
CREATE TABLE IF NOT EXISTS public.todos (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id UUID DEFAULT auth.uid() REFERENCES public.profiles(id) ON DELETE CASCADE NOT NULL,
    task TEXT NOT NULL,
    is_complete BOOLEAN DEFAULT false NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL
);

ALTER TABLE public.todos ENABLE ROW LEVEL SECURITY;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.todos TO authenticated;
CREATE INDEX IF NOT EXISTS idx_todos_user_id ON public.todos(user_id);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can only see their own todos' AND tablename = 'todos') THEN
        CREATE POLICY "Users can only see their own todos" ON public.todos FOR SELECT USING (auth.uid() = user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can insert their own todos' AND tablename = 'todos') THEN
        CREATE POLICY "Users can insert their own todos" ON public.todos FOR INSERT WITH CHECK (auth.uid() = user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can update their own todos' AND tablename = 'todos') THEN
        CREATE POLICY "Users can update their own todos" ON public.todos FOR UPDATE USING (auth.uid() = user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'Users can delete their own todos' AND tablename = 'todos') THEN
        CREATE POLICY "Users can delete their own todos" ON public.todos FOR DELETE USING (auth.uid() = user_id);
    END IF;
END $$;

-- =============================================================================
-- 3. Realtime Notifications
-- =============================================================================
CREATE OR REPLACE FUNCTION notify_realtime()
RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify(
    'realtime_events',
    json_build_object(
      'table', TG_TABLE_NAME,
      'action', TG_OP,
      'data', CASE WHEN TG_OP = 'DELETE' THEN row_to_json(OLD) ELSE row_to_json(NEW) END
    )::text
  );
  RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'todos_realtime') THEN
        CREATE TRIGGER todos_realtime
            AFTER INSERT OR UPDATE OR DELETE ON todos
            FOR EACH ROW EXECUTE FUNCTION notify_realtime();
    END IF;
END $$;

-- =============================================================================
-- 4. Notify PostgREST to reload schema cache
-- =============================================================================
NOTIFY pgrst, 'reload schema';
