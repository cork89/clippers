-- Clippers database schema

CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    audio_path TEXT NOT NULL,
    images_dir TEXT NOT NULL,
    output_dir TEXT NOT NULL,
    audio_hash TEXT NOT NULL,
    images_hash TEXT NOT NULL,
    settings TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE transcripts (
    project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    language TEXT,
    duration_sec REAL
);

CREATE TABLE segments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    text TEXT NOT NULL,
    ordinal INTEGER NOT NULL
);

CREATE TABLE images (
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

CREATE TABLE text_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    text TEXT,
    ordinal INTEGER NOT NULL
);

CREATE TABLE timeline_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    start REAL NOT NULL,
    "end" REAL NOT NULL,
    image_id TEXT NOT NULL,
    image_path TEXT NOT NULL,
    confidence REAL,
    reason TEXT,
    shader TEXT,
    ordinal INTEGER NOT NULL
);

CREATE INDEX idx_segments_project ON segments(project_id);
CREATE INDEX idx_images_project ON images(project_id);
CREATE INDEX idx_windows_project ON text_windows(project_id);
CREATE INDEX idx_timeline_project ON timeline_entries(project_id);
