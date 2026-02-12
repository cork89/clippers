-- name: CreateProject :exec
INSERT OR IGNORE INTO projects (id, audio_path, images_dir, output_dir, audio_hash, images_hash, settings)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;

-- name: ProjectExists :one
SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?);

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = ?;

-- name: UpdateProject :exec
UPDATE projects SET audio_path = ?, images_dir = ?, output_dir = ?, settings = ? WHERE id = ?;
