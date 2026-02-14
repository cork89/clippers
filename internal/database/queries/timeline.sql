-- name: SaveTimelineEntry :exec
INSERT INTO timeline_entries (project_id, start, "end", image_id, image_path, confidence, reason, ordinal)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTimeline :many
SELECT id, project_id, start, "end", image_id, image_path, confidence, reason, ordinal 
FROM timeline_entries WHERE project_id = ? ORDER BY ordinal;

-- name: TimelineExists :one
SELECT EXISTS(SELECT 1 FROM timeline_entries WHERE project_id = ?);

-- name: ClearTimeline :exec
DELETE FROM timeline_entries WHERE project_id = ?;
