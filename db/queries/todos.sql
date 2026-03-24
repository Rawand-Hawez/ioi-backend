-- RLS-based queries (rely on auth.uid() via GUC variables)
-- These must be used within a transaction with GUC injection

-- name: GetTodosRLS :many
SELECT * FROM public.todos
ORDER BY created_at DESC;

-- name: CreateTodoRLS :one
INSERT INTO public.todos (task)
VALUES ($1)
RETURNING *;

-- name: ToggleTodoRLS :exec
UPDATE public.todos
SET is_complete = NOT is_complete
WHERE id = $1;

-- name: DeleteTodoRLS :exec
DELETE FROM public.todos
WHERE id = $1;

-- name: GetTodoByIDRLS :one
SELECT * FROM public.todos
WHERE id = $1;
