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
	return err
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
	if err := db.Queries.SaveTranscript(ctx, sqlc.SaveTranscriptParams{
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
		return err
	}

	for i, seg := range t.Segments {
		if err := db.Queries.SaveSegment(ctx, sqlc.SaveSegmentParams{
			ProjectID: projectID,
			Start:     seg.Start,
			End:       seg.End,
			Text:      seg.Text,
			Ordinal:   int64(i),
		}); err != nil {
			return err
		}
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
	for _, img := range catalog.Images {
		tagsJSON, _ := json.Marshal(img.Tags)
		if err := db.Queries.SaveImage(ctx, sqlc.SaveImageParams{
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
			return err
		}
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
	for i, w := range windows {
		if err := db.Queries.SaveWindow(ctx, sqlc.SaveWindowParams{
			ProjectID: projectID,
			Start:     w.Start,
			End:       w.End,
			Text: sql.NullString{
				String: w.Text,
				Valid:  w.Text != "",
			},
			Ordinal: int64(i),
		}); err != nil {
			return err
		}
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
	for i, e := range timeline.Entries {
		if err := db.Queries.SaveTimelineEntry(ctx, sqlc.SaveTimelineEntryParams{
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
			return err
		}
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
