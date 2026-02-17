package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/cork89/clippers/internal/database/sqlc"
	"github.com/cork89/clippers/internal/types"
)

type DB struct {
	db      *sql.DB
	Queries *sqlc.Queries
}

func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{
		db:      db,
		Queries: sqlc.New(db),
	}, nil
}

func (db *DB) Close() error {
	return db.db.Close()
}

func runMigrations(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    audio_path TEXT NOT NULL,
    images_dir TEXT NOT NULL,
    output_dir TEXT NOT NULL,
    audio_hash TEXT NOT NULL,
    images_hash TEXT NOT NULL,
    settings TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transcripts (
    project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    language TEXT,
    duration_sec REAL
);

CREATE TABLE IF NOT EXISTS segments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    text TEXT NOT NULL,
    ordinal INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS images (
    id TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    caption TEXT,
    tags TEXT,
    style INTEGER,
    notes TEXT,
    is_default INTEGER DEFAULT 0,
    PRIMARY KEY (id, project_id)
);

CREATE TABLE IF NOT EXISTS text_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    text TEXT,
    ordinal INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS project_settings (
    project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    
    -- Video settings
    shader TEXT DEFAULT 'none',
    fps INTEGER DEFAULT 24,
    aspects TEXT DEFAULT '1x1,16x9,9x16',
    
    -- Subtitle settings
    font_size INTEGER DEFAULT 60,
    subtitle_margin INTEGER DEFAULT 20,
    
    -- Planning settings
    min_shot_sec REAL DEFAULT 5.0,
    max_words INTEGER DEFAULT 5,
    default_image_weight REAL DEFAULT 0.5,
    title_weight TEXT DEFAULT 'high',
    blur_strength INTEGER DEFAULT 20,
    
    -- LLM settings
    whisper_model TEXT DEFAULT 'distil-medium.en',
    vision_model TEXT,
    select_model TEXT,
    
    -- Metadata
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS timeline_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    image_id TEXT NOT NULL,
    image_path TEXT NOT NULL,
    confidence REAL,
    reason TEXT,
    ordinal INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_segments_project ON segments(project_id);
CREATE INDEX IF NOT EXISTS idx_images_project ON images(project_id);
CREATE INDEX IF NOT EXISTS idx_windows_project ON text_windows(project_id);
CREATE INDEX IF NOT EXISTS idx_timeline_project ON timeline_entries(project_id);
`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	return runMigrationsFromHistory(db)
}

type migration struct {
	name string
	up   string
}

var migrations = []migration{
	{
		name: "add_timeline_shader",
		up:   "ALTER TABLE timeline_entries ADD COLUMN shader TEXT;",
	},
	{
		name: "dedupe_project_rows_by_ordinal",
		up: `
DELETE FROM segments
WHERE id NOT IN (
	SELECT MAX(id) FROM segments GROUP BY project_id, ordinal
);

DELETE FROM text_windows
WHERE id NOT IN (
	SELECT MAX(id) FROM text_windows GROUP BY project_id, ordinal
);

DELETE FROM timeline_entries
WHERE id NOT IN (
	SELECT MAX(id) FROM timeline_entries GROUP BY project_id, ordinal
);`,
	},
}

func runMigrationsFromHistory(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM migrations WHERE name = ?)", m.name).Scan(&exists)
		if err != nil {
			return err
		}

		if !exists {
			_, err := db.Exec(m.up)
			if err != nil {
				return fmt.Errorf("migration %s failed: %w", m.name, err)
			}

			_, err = db.Exec("INSERT INTO migrations (name) VALUES (?)", m.name)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (db *DB) CreateProject(ctx context.Context, id, audioPath, imagesDir, outputDir, audioHash, imagesHash string, settings map[string]string) error {
	settingsJSON, _ := json.Marshal(settings)
	return db.Queries.CreateProject(ctx, sqlc.CreateProjectParams{
		ID:         id,
		AudioPath:  audioPath,
		ImagesDir:  imagesDir,
		OutputDir:  outputDir,
		AudioHash:  audioHash,
		ImagesHash: imagesHash,
		Settings: sql.NullString{
			String: string(settingsJSON),
			Valid:  len(settingsJSON) > 0,
		},
	})
}

func (db *DB) SaveFullTranscript(ctx context.Context, projectID string, t *types.Transcript) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transcript transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.Queries.WithTx(tx)

	if err := qtx.SaveTranscript(ctx, sqlc.SaveTranscriptParams{
		ProjectID: projectID,
		Language: sql.NullString{
			String: t.Language,
			Valid:  t.Language != "",
		},
		DurationSec: sql.NullFloat64{
			Float64: t.DurationSec,
			Valid:   t.DurationSec > 0,
		},
	}); err != nil {
		return fmt.Errorf("failed to save transcript row: %w", err)
	}

	if err := qtx.ClearSegments(ctx, projectID); err != nil {
		return fmt.Errorf("failed to clear existing segments: %w", err)
	}

	for i, seg := range t.Segments {
		if err := qtx.SaveSegment(ctx, sqlc.SaveSegmentParams{
			ProjectID: projectID,
			Start:     seg.Start,
			End:       seg.End,
			Text:      seg.Text,
			Ordinal:   int64(i),
		}); err != nil {
			return fmt.Errorf("failed to save segment %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transcript transaction: %w", err)
	}

	return nil
}

func (db *DB) GetFullTranscript(ctx context.Context, projectID string) (*types.Transcript, error) {
	t, err := db.Queries.GetTranscript(ctx, projectID)
	if err != nil {
		return nil, err
	}

	segments, err := db.Queries.GetSegments(ctx, projectID)
	if err != nil {
		return nil, err
	}

	result := &types.Transcript{
		Language:    t.Language.String,
		DurationSec: t.DurationSec.Float64,
		Segments:    make([]types.Segment, len(segments)),
	}

	for i, s := range segments {
		result.Segments[i] = types.Segment{
			Start: s.Start,
			End:   s.End,
			Text:  s.Text,
		}
	}

	return result, nil
}

type ImageCaption struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Caption   string   `json:"caption"`
	Tags      []string `json:"tags"`
	Style     int      `json:"style"`
	Notes     string   `json:"notes,omitempty"`
	IsDefault bool     `json:"is_default,omitempty"`
}

type ImageCatalog struct {
	Images       []ImageCaption `json:"images"`
	DefaultImage string         `json:"default_image,omitempty"`
}

type TextWindowData struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func (db *DB) SaveImageCatalog(ctx context.Context, projectID string, catalog *ImageCatalog) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin image catalog transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.Queries.WithTx(tx)
	if err := qtx.ClearImages(ctx, projectID); err != nil {
		return fmt.Errorf("failed to clear existing images: %w", err)
	}

	for _, img := range catalog.Images {
		tagsJSON, _ := json.Marshal(img.Tags)
		if err := qtx.SaveImage(ctx, sqlc.SaveImageParams{
			ID:        img.ID,
			ProjectID: projectID,
			Path:      img.Path,
			Caption: sql.NullString{
				String: img.Caption,
				Valid:  img.Caption != "",
			},
			Tags: sql.NullString{
				String: string(tagsJSON),
				Valid:  len(tagsJSON) > 0,
			},
			Style: sql.NullInt64{
				Int64: int64(img.Style),
				Valid: true,
			},
			Notes: sql.NullString{
				String: img.Notes,
				Valid:  img.Notes != "",
			},
			IsDefault: sql.NullInt64{
				Int64: boolToInt64(img.IsDefault),
				Valid: true,
			},
		}); err != nil {
			return fmt.Errorf("failed to save image %q: %w", img.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit image catalog transaction: %w", err)
	}

	return nil
}

func (db *DB) GetImageCatalog(ctx context.Context, projectID string) (*ImageCatalog, error) {
	images, err := db.Queries.GetImages(ctx, projectID)
	if err != nil {
		return nil, err
	}

	catalog := &ImageCatalog{
		Images: make([]ImageCaption, len(images)),
	}

	for i, img := range images {
		var tags []string
		if img.Tags.Valid {
			if err := json.Unmarshal([]byte(img.Tags.String), &tags); err != nil {
				tags = []string{}
			}
		}

		catalog.Images[i] = ImageCaption{
			ID:        img.ID,
			Path:      img.Path,
			Caption:   img.Caption.String,
			Tags:      tags,
			Style:     int(img.Style.Int64),
			Notes:     img.Notes.String,
			IsDefault: img.IsDefault.Int64 == 1,
		}

		if img.IsDefault.Int64 == 1 {
			catalog.DefaultImage = img.Path
		}
	}

	return catalog, nil
}

func (db *DB) SaveTextWindows(ctx context.Context, projectID string, windows []TextWindowData) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin windows transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.Queries.WithTx(tx)
	if err := qtx.ClearWindows(ctx, projectID); err != nil {
		return fmt.Errorf("failed to clear existing windows: %w", err)
	}

	for i, w := range windows {
		if err := qtx.SaveWindow(ctx, sqlc.SaveWindowParams{
			ProjectID: projectID,
			Start:     w.Start,
			End:       w.End,
			Text: sql.NullString{
				String: w.Text,
				Valid:  w.Text != "",
			},
			Ordinal: int64(i),
		}); err != nil {
			return fmt.Errorf("failed to save window %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit windows transaction: %w", err)
	}

	return nil
}

func (db *DB) GetTextWindows(ctx context.Context, projectID string) ([]TextWindowData, error) {
	windows, err := db.Queries.GetWindows(ctx, projectID)
	if err != nil {
		return nil, err
	}

	result := make([]TextWindowData, len(windows))
	for i, w := range windows {
		result[i] = TextWindowData{
			Start: w.Start,
			End:   w.End,
			Text:  w.Text.String,
		}
	}
	return result, nil
}

func (db *DB) SaveTimeline(ctx context.Context, projectID string, timeline *types.Timeline) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin timeline transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.Queries.WithTx(tx)
	if err := qtx.ClearTimeline(ctx, projectID); err != nil {
		return fmt.Errorf("failed to clear existing timeline entries: %w", err)
	}

	for i, e := range timeline.Entries {
		if err := qtx.SaveTimelineEntry(ctx, sqlc.SaveTimelineEntryParams{
			ProjectID: projectID,
			Start:     e.Start,
			End:       e.End,
			ImageID:   e.ImageID,
			ImagePath: e.Image,
			Confidence: sql.NullFloat64{
				Float64: e.Confidence,
				Valid:   e.Confidence > 0,
			},
			Reason: sql.NullString{
				String: e.Reason,
				Valid:  e.Reason != "",
			},
			Ordinal: int64(i),
		}); err != nil {
			return fmt.Errorf("failed to save timeline entry %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit timeline transaction: %w", err)
	}

	return nil
}

func (db *DB) GetTimeline(ctx context.Context, projectID string) (*types.Timeline, error) {
	entries, err := db.Queries.GetTimeline(ctx, projectID)
	if err != nil {
		return nil, err
	}

	result := &types.Timeline{
		Entries: make([]types.TimelineEntry, len(entries)),
	}

	for i, e := range entries {
		result.Entries[i] = types.TimelineEntry{
			Start:      e.Start,
			End:        e.End,
			ImageID:    e.ImageID,
			Image:      e.ImagePath,
			Confidence: e.Confidence.Float64,
			Reason:     e.Reason.String,
		}
	}

	return result, nil
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (db *DB) GetProjectSettings(ctx context.Context, projectID string) (*types.ProjectSettings, error) {
	settings, err := db.Queries.GetProjectSettings(ctx, projectID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	result := &types.ProjectSettings{
		ProjectID:          settings.ProjectID,
		Shader:             settings.Shader.String,
		FPS:                int(settings.Fps.Int64),
		Aspects:            settings.Aspects.String,
		FontSize:           int(settings.FontSize.Int64),
		SubtitleMargin:     int(settings.SubtitleMargin.Int64),
		MinShotSec:         settings.MinShotSec.Float64,
		MaxWords:           int(settings.MaxWords.Int64),
		DefaultImageWeight: settings.DefaultImageWeight.Float64,
		TitleWeight:        settings.TitleWeight.String,
		BlurStrength:       int(settings.BlurStrength.Int64),
		WhisperModel:       settings.WhisperModel.String,
		VisionModel:        settings.VisionModel.String,
		SelectModel:        settings.SelectModel.String,
	}

	return result, nil
}

func (db *DB) SaveProjectSettings(ctx context.Context, settings *types.ProjectSettings) error {
	return db.Queries.UpsertProjectSettings(ctx, sqlc.UpsertProjectSettingsParams{
		ProjectID: settings.ProjectID,
		Shader: sql.NullString{
			String: settings.Shader,
			Valid:  settings.Shader != "",
		},
		Fps: sql.NullInt64{
			Int64: int64(settings.FPS),
			Valid: settings.FPS > 0,
		},
		Aspects: sql.NullString{
			String: settings.Aspects,
			Valid:  settings.Aspects != "",
		},
		FontSize: sql.NullInt64{
			Int64: int64(settings.FontSize),
			Valid: settings.FontSize > 0,
		},
		SubtitleMargin: sql.NullInt64{
			Int64: int64(settings.SubtitleMargin),
			Valid: settings.SubtitleMargin > 0,
		},
		MinShotSec: sql.NullFloat64{
			Float64: settings.MinShotSec,
			Valid:   settings.MinShotSec > 0,
		},
		MaxWords: sql.NullInt64{
			Int64: int64(settings.MaxWords),
			Valid: settings.MaxWords > 0,
		},
		DefaultImageWeight: sql.NullFloat64{
			Float64: settings.DefaultImageWeight,
			Valid:   settings.DefaultImageWeight > 0,
		},
		TitleWeight: sql.NullString{
			String: settings.TitleWeight,
			Valid:  settings.TitleWeight != "",
		},
		BlurStrength: sql.NullInt64{
			Int64: int64(settings.BlurStrength),
			Valid: settings.BlurStrength > 0,
		},
		WhisperModel: sql.NullString{
			String: settings.WhisperModel,
			Valid:  settings.WhisperModel != "",
		},
		VisionModel: sql.NullString{
			String: settings.VisionModel,
			Valid:  settings.VisionModel != "",
		},
		SelectModel: sql.NullString{
			String: settings.SelectModel,
			Valid:  settings.SelectModel != "",
		},
	})
}
