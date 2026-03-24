-- 01_app_schema.sql: Define your custom domain here

-- 1. Create the `todos` table
CREATE TABLE public.todos (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id UUID REFERENCES public.profiles(id) ON DELETE CASCADE NOT NULL,
    task TEXT NOT NULL,
    is_complete BOOLEAN DEFAULT false NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT timezone('utc'::text, now()) NOT NULL
);

-- 2. Turn on Zero-Trust Security (Row-Level Security)
ALTER TABLE public.todos ENABLE ROW LEVEL SECURITY;

-- 3. Define EXACT access policies using the `auth.uid()` helper we created in 00_core_auth.sql
CREATE POLICY "Users can only see their own todos" 
    ON public.todos FOR SELECT 
    USING (auth.uid() = user_id);

CREATE POLICY "Users can insert their own todos" 
    ON public.todos FOR INSERT 
    WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update their own todos" 
    ON public.todos FOR UPDATE 
    USING (auth.uid() = user_id);

CREATE POLICY "Users can delete their own todos" 
    ON public.todos FOR DELETE 
    USING (auth.uid() = user_id);

-- REALTIME NOTIFICATIONS
-- This function sends a JSON payload to the 'realtime_events' channel
CREATE OR REPLACE FUNCTION notify_realtime()
RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify(
    'realtime_events',
    json_build_object(
      'table', TG_TABLE_NAME,
      'action', TG_OP,
      'data', row_to_json(NEW)
    )::text
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for the 'todos' table
CREATE TRIGGER todos_realtime
AFTER INSERT OR UPDATE OR DELETE ON todos
FOR EACH ROW EXECUTE FUNCTION notify_realtime();
