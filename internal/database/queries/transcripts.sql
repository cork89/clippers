-- name: SaveTranscript :exec
INSERT INTO transcripts (project_id, language, duration_sec)
VALUES (?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET language = excluded.language, duration_sec = excluded.duration_sec;

-- name: GetTranscript :one
SELECT * FROM transcripts WHERE project_id = ?;

-- name: TranscriptExists :one
SELECT EXISTS(SELECT 1 FROM transcripts WHERE project_id = ?);

-- name: SaveSegment :exec
INSERT INTO segments (project_id, start, "end", text, ordinal)
VALUES (?, ?, ?, ?, ?);

-- name: GetSegments :many
SELECT id, project_id, start, "end", text, ordinal FROM segments 
WHERE project_id = ? ORDER BY ordinal;

-- name: ClearSegments :exec
DELETE FROM segments WHERE project_id = ?;
