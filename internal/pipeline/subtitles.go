package pipeline

import (
	"fmt"
	"os"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
)

// GenerateSubtitles creates chunked SRT from transcript
func GenerateSubtitles(wd *workdir.WorkDir, cfg *config.Config, transcript *types.Transcript, force bool) (string, error) {
	srtPath := wd.Path("subtitles.srt")

	if !force && wd.Exists("subtitles.srt") {
		fmt.Println("==> Subtitles (cached)")
		return srtPath, nil
	}

	fmt.Println("==> Generating subtitles")

	var cues []types.SRTCue
	cueIndex := 1

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

			// Approximate timing within segment
			startRatio := float64(i) / float64(len(words))
			endRatio := float64(end) / float64(len(words))

			cueStart := seg.Start + (segDuration * startRatio)
			cueEnd := seg.Start + (segDuration * endRatio)

			// Ensure minimum duration
			if cueEnd-cueStart < 0.5 {
				cueEnd = cueStart + 0.5
			}
			if cueEnd > seg.End {
				cueEnd = seg.End
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
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
