# AGENTS.md

Agentic coding instructions for the Clippers repository.

## Overview

Clippers is a Go CLI tool that converts audio clips to videos with AI-driven image selection and subtitles. It uses Whisper.cpp for transcription, Ollama for vision LLM captioning and text LLM image selection, and FFmpeg for video rendering.

## Build Commands

```bash
# Build the CLI binary
go build -o clippers.exe ./cmd/clippers

# Build for release (optimized)
go build -ldflags "-s -w" -o clippers.exe ./cmd/clippers
```

## Test Commands

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./internal/pipeline

# Run a specific test function
go test ./internal/pipeline -run TestFunctionName

# Run tests with verbose output
go test -v ./...
```

## Lint Commands

```bash
# Run golangci-lint (preferred)
golangci-lint run

# Run standard Go tools
go vet ./...
gofmt -d .

# Check for unused code
go mod tidy
```

## Code Style Guidelines

### Imports

Group imports in this order, separated by blank lines:
1. Standard library packages
2. Third-party packages
3. Internal project packages

```go
import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/cobra"

    "github.com/cork89/clippers/internal/config"
    "github.com/cork89/clippers/internal/types"
)
```

### Formatting

- Use `gofmt` for all Go code
- No trailing whitespace
- End files with a single newline
- Use spaces (not tabs) for alignment in comments if needed

### Naming Conventions

- **Packages**: lowercase, single word (e.g., `pipeline`, `workdir`, `ollama`)
- **Exported identifiers**: PascalCase (e.g., `Transcribe`, `Config`, `Client`)
- **Unexported identifiers**: camelCase (e.g., `hashFile`, `audioHash`)
- **Constants**: Use const keyword, may be PascalCase if exported (e.g., `ShaderNone`)
- **Acronyms**: Keep uppercase (e.g., `SRTCue`, `HTTPClient`)
- **File names**: lowercase_with_underscores.go or lowercase.go

### Types

- Use struct tags for JSON serialization: ``json:"field_name"``
- Use `omitempty` for optional fields: ``json:"field,omitempty"``
- Define related types in the same file (e.g., request/response structs with their client)

### Error Handling

- Always wrap errors with context using `fmt.Errorf("...: %w", err)`
- Return errors rather than logging and continuing in most cases
- Use early returns to reduce nesting
- Check for specific errors when appropriate

```go
if err != nil {
    return fmt.Errorf("failed to create work directory: %w", err)
}
```

### Comments

- Use `//` for all comments
- Include a package comment at the top of each file: `// ./path/to/file.go`
- Document all exported functions, types, and constants
- Keep comments concise and describe the "why" not just the "what"

### Struct Organization

- Group related fields together
- Use pointers for optional configuration
- Embed smaller structs when appropriate

### CLI Commands

- Use Cobra for CLI commands
- Define commands in `internal/cli/root.go`
- Use `RunE` for commands that can fail (returns error)
- Use `Run` for simple commands that don't fail
- Register commands in the `init()` function

### Configuration

- Define default configuration in `config.DefaultConfig()`
- Use struct tags for CLI flag binding
- Validate configuration early in command handlers

## Project Structure

```
cmd/clippers/         # Entry point
internal/
  cli/                # Cobra CLI commands
  config/             # Configuration types and defaults
  ollama/             # Ollama API client
  pipeline/           # Video processing pipeline stages
  types/              # Shared type definitions
  workdir/            # Working directory management
assets/               # Embedded assets (fonts)
shaders/              # Shader files for video effects
```

## Dependencies

- **spf13/cobra**: CLI framework
- **golang.org/x/image**: Extended image support

## External Dependencies

The application requires these external binaries:
- `ffmpeg` and `ffprobe`: Video processing
- `whisper.cpp`: Audio transcription
- `ollama`: LLM inference server

## Common Tasks

```bash
# Run the full pipeline
./clippers.exe run -a audio.mp3 -i ./images -o ./output

# Preview the timeline without rendering
./clippers.exe preview -a audio.mp3 -i ./images

# Re-render from cached timeline
./clippers.exe render -a audio.mp3 -i ./images
```
