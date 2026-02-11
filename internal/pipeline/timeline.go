// ./internal/pipeline/timeline.go
package pipeline

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// BuildTimeline creates a simple placeholder timeline cycling through images
func BuildTimeline(wd *workdir.WorkDir, cfg *config.Config, transcript *types.Transcript, force bool) (*types.Timeline, error) {
	if !force && wd.Exists("timeline.json") {
		fmt.Println("==> Timeline (cached)")
		var t types.Timeline
		if err := wd.ReadJSON("timeline.json", &t); err != nil {
			return nil, fmt.Errorf("failed to read timeline: %w", err)
		}
		return &t, nil
	}

	fmt.Println("==> Building timeline (placeholder)")

	// Get list of images
	images, err := listImages(cfg.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	// Sort for deterministic order
	sort.Strings(images)

	duration := transcript.DurationSec
	shotDuration := cfg.MinShotSec

	var entries []types.TimelineEntry
	currentTime := 0.0
	imageIndex := 0

	for currentTime < duration {
		end := currentTime + shotDuration
		if end > duration {
			end = duration
		}

		imagePath := images[imageIndex%len(images)]
		entries = append(entries, types.TimelineEntry{
			Start:   currentTime,
			End:     end,
			ImageID: filepath.Base(imagePath),
			Image:   imagePath,
		})

		currentTime = end
		imageIndex++
	}

	timeline := &types.Timeline{Entries: entries}

	if err := wd.WriteJSON("timeline.json", timeline); err != nil {
		return nil, fmt.Errorf("failed to write timeline: %w", err)
	}

	fmt.Printf("  ✓ Created %d shots (%.1fs each)\n", len(entries), shotDuration)
	return timeline, nil
}
