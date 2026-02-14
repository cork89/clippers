# Project Settings Plan

## Overview

Move all project settings from JSON string storage to a dedicated database table. This includes moving the shader selection from `timeline_entries` to project-level settings.

## Current State

### Database Schema
```sql
-- projects table (current)
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    audio_path TEXT NOT NULL,
    images_dir TEXT NOT NULL,
    output_dir TEXT NOT NULL,
    audio_hash TEXT NOT NULL,
    images_hash TEXT NOT NULL,
    settings TEXT,                    -- JSON string (problematic)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- timeline_entries table (current)
CREATE TABLE timeline_entries (
    ...
    shader TEXT,                      -- per-entry shader (to be removed)
    ...
);
```

### Current Settings Flow
- Settings stored as JSON string in `projects.settings` column
- Shader stored per-timeline-entry in `timeline_entries.shader`
- No type safety, hard to query, error-prone

## Target State

### New Database Schema
```sql
-- projects table (simplified)
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    audio_path TEXT NOT NULL,
    images_dir TEXT NOT NULL,
    output_dir TEXT NOT NULL,
    audio_hash TEXT NOT NULL,
    images_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- project_settings table (new)
CREATE TABLE project_settings (
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

-- timeline_entries table (simplified)
CREATE TABLE timeline_entries (
    ...
    shader TEXT,                      -- REMOVED
    ...
);
```

### Benefits
1. **Type safety** - Each setting has proper column type
2. **Queryable** - Can filter projects by settings
3. **Indexed** - Can add indexes for frequently queried fields
4. **Schema evolution** - Easy to add/remove/migrate settings
5. **Shader at project level** - Single shader for entire project

## Implementation Steps

### Phase 1: Database Changes

1. **Create migration script**
   - Create new `project_settings` table
   - Copy any existing settings from JSON to new table
   - Add default values for missing fields

2. **Generate SQLC code**
   - Update schema.sql
   - Run `go generate ./internal/database/...`
   - Add queries for CRUD on project_settings

### Phase 2: Backend Changes

3. **Update types**
   - Create `ProjectSettings` struct in `types/types.go`
   - Remove `Shader` field from `TimelineEntry`

4. **Update database layer**
   - Add `GetProjectSettings(projectID)` function
   - Add `UpdateProjectSettings(projectID, *ProjectSettings)` function

5. **Update config loading**
   - Load project settings from DB instead of JSON
   - Merge with CLI/env defaults (CLI/env takes precedence)

6. **Update server handlers**
   - Remove shader endpoints from `/api/segment/*/shader`
   - Add `/api/project/settings` GET/PUT endpoints

7. **Update render pipeline**
   - Read shader from project settings instead of config

### Phase 3: Frontend Changes

8. **Update segment editor**
   - Remove shader selector from segment panel
   - Remove shader preview JavaScript (or repurpose)

9. **Add project settings UI**
   - Create new settings panel/modal
   - Add shader selector at project level

10. **Update API**
    - `GET /api/project/settings` - get all settings
    - `PUT /api/project/settings` - update settings

## Files to Modify

### Database
- `internal/database/schema.sql` - add project_settings table
- `internal/database/queries/projects.sql` - add settings queries
- `internal/database/sqlc/*.go` - regenerate

### Types
- `internal/types/types.go` - add ProjectSettings struct

### Backend
- `internal/database/db.go` - add settings load/save functions
- `internal/webserver/server.go` - add settings handlers
- `internal/config/config.go` - merge project settings with defaults
- `internal/pipeline/render.go` - read shader from project settings

### Frontend
- `internal/views/segment.templ` - remove shader selector
- `internal/views/static/shader.js` - remove or repurpose
- `internal/views/layout.templ` - add settings panel

## Migration Strategy

1. **Backward compatible start**
   - New table populated from existing JSON on first access
   - Old JSON column remains until full migration

2. **Gradual rollout**
   - Read from new table first, fall back to JSON
   - Write to both during transition

3. **Cleanup**
   - Remove JSON column after verification
   - Delete migration code

## Testing Plan

1. Unit tests for settings serialization
2. Manual testing of settings save/load
3. Verify shader applied correctly in render
4. Test migration path from existing projects
