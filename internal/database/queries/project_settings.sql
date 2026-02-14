-- name: CreateProjectSettings :exec
INSERT INTO project_settings (project_id) VALUES (?);

-- name: GetProjectSettings :one
SELECT * FROM project_settings WHERE project_id = ?;

-- name: UpdateProjectSettings :exec
UPDATE project_settings SET 
    shader = ?,
    fps = ?,
    aspects = ?,
    font_size = ?,
    subtitle_margin = ?,
    min_shot_sec = ?,
    max_words = ?,
    default_image_weight = ?,
    title_weight = ?,
    blur_strength = ?,
    whisper_model = ?,
    vision_model = ?,
    select_model = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ?;

-- name: UpsertProjectSettings :exec
INSERT INTO project_settings (
    project_id, shader, fps, aspects, font_size, subtitle_margin,
    min_shot_sec, max_words, default_image_weight, title_weight, blur_strength,
    whisper_model, vision_model, select_model
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
    shader = excluded.shader,
    fps = excluded.fps,
    aspects = excluded.aspects,
    font_size = excluded.font_size,
    subtitle_margin = excluded.subtitle_margin,
    min_shot_sec = excluded.min_shot_sec,
    max_words = excluded.max_words,
    default_image_weight = excluded.default_image_weight,
    title_weight = excluded.title_weight,
    blur_strength = excluded.blur_strength,
    whisper_model = excluded.whisper_model,
    vision_model = excluded.vision_model,
    select_model = excluded.select_model,
    updated_at = CURRENT_TIMESTAMP;
