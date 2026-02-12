-- name: SaveWindow :exec
INSERT INTO text_windows (project_id, start, "end", text, ordinal)
VALUES (?, ?, ?, ?, ?);

-- name: GetWindows :many
SELECT id, project_id, start, "end", text, ordinal FROM text_windows 
WHERE project_id = ? ORDER BY ordinal;

-- name: WindowsExist :one
SELECT EXISTS(SELECT 1 FROM text_windows WHERE project_id = ?);

-- name: ClearWindows :exec
DELETE FROM text_windows WHERE project_id = ?;
