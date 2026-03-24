-- name: GetTodosForUser :many
SELECT * FROM public.todos
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateTodo :one
INSERT INTO public.todos (user_id, task)
VALUES ($1, $2)
RETURNING *;

-- name: ToggleTodo :exec
UPDATE public.todos
SET is_complete = NOT is_complete
WHERE id = $1 AND user_id = $2;
