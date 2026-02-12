-- name: SaveImage :exec
INSERT INTO images (id, project_id, path, caption, tags, style, notes, is_default)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id, project_id) DO UPDATE SET 
    caption = excluded.caption, 
    tags = excluded.tags,
    style = excluded.style,
    notes = excluded.notes,
    is_default = excluded.is_default;

-- name: GetImages :many
SELECT id, project_id, path, caption, tags, style, notes, is_default 
FROM images WHERE project_id = ?;

-- name: GetDefaultImage :one
SELECT id, path FROM images WHERE project_id = ? AND is_default = 1;

-- name: ImageCatalogExists :one
SELECT EXISTS(SELECT 1 FROM images WHERE project_id = ?);

-- name: ClearImages :exec
DELETE FROM images WHERE project_id = ?;
