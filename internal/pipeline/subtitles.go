package pipeline

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// quantizeToFrameRate rounds time to nearest frame boundary
func quantizeToFrameRate(seconds float64, fps int) float64 {
	if fps <= 0 {
		fps = 24
	}
	frameDuration := 1.0 / float64(fps)
	frames := seconds / frameDuration
	return math.Round(frames) * frameDuration
}

// GenerateSubtitles creates chunked SRT from transcript with strict contiguity
func GenerateSubtitles(wd *workdir.WorkDir, cfg *config.Config, transcript *types.Transcript, timeline *types.Timeline, force bool) (string, error) {
	srtPath := wd.Path("subtitles.srt")

	if !force && wd.Exists("subtitles.srt") {
		fmt.Println("==> Subtitles (cached)")
		return srtPath, nil
	}

	fmt.Println("==> Generating subtitles")

	var cues []types.SRTCue
	cueIndex := 1
	minDuration := 1.0 / float64(cfg.FPS)

	// Process transcript segments sequentially (not timeline entries)
	for _, seg := range transcript.Segments {
		words := strings.Fields(seg.Text)
		if len(words) == 0 {
			continue
		}

		segDuration := seg.End - seg.Start
		wordsPerChunk := cfg.MaxWords

		// Split segment into chunks
		for i := 0; i < len(words); i += wordsPerChunk {
			end := i + wordsPerChunk
			if end > len(words) {
				end = len(words)
			}

			chunkWords := words[i:end]
			chunkText := strings.Join(chunkWords, " ")

			// Calculate timing
			startRatio := float64(i) / float64(len(words))
			endRatio := float64(end) / float64(len(words))

			cueStart := seg.Start + (segDuration * startRatio)
			cueEnd := seg.Start + (segDuration * endRatio)

			// Ensure minimum duration
			if cueEnd-cueStart < minDuration {
				cueEnd = cueStart + minDuration
			}

			// Clamp to segment bounds
			if cueStart < seg.Start {
				cueStart = seg.Start
			}
			if cueEnd > seg.End {
				cueEnd = seg.End
			}

			// Quantize to frame boundaries
			cueStart = quantizeToFrameRate(cueStart, cfg.FPS)
			cueEnd = quantizeToFrameRate(cueEnd, cfg.FPS)

			// Force contiguity: ensure no gaps or overlaps with previous cue
			if len(cues) > 0 {
				prevCue := &cues[len(cues)-1]
				// If current start is before previous end, adjust it
				if cueStart < prevCue.End {
					cueStart = prevCue.End
				}
				// If there's a tiny gap, close it
				if cueStart > prevCue.End && cueStart-prevCue.End < minDuration*2 {
					cueStart = prevCue.End
				}
			}

			// Skip if duration is too small after adjustments
			if cueEnd-cueStart < minDuration/2 {
				continue
			}

			cues = append(cues, types.SRTCue{
				Index: cueIndex,
				Start: cueStart,
				End:   cueEnd,
				Text:  chunkText,
			})
			cueIndex++
		}
	}

	// Write SRT file
	var sb strings.Builder
	for _, cue := range cues {
		sb.WriteString(fmt.Sprintf("%d\n", cue.Index))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", formatSRTTime(cue.Start), formatSRTTime(cue.End)))
		sb.WriteString(fmt.Sprintf("%s\n\n", cue.Text))
	}

	if err := os.WriteFile(srtPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write SRT: %w", err)
	}

	fmt.Printf("  ✓ Generated %d subtitle cues\n", len(cues))
	return srtPath, nil
}

func formatSRTTime(seconds float64) string {
	// Add epsilon to prevent rounding errors
	seconds += 0.000001

	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
