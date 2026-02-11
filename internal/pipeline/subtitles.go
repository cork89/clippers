// ./internal/pipeline/subtitles.go
package pipeline

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cork89/clippers/assets"
	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ------------------------------------------------------------
// USER SETTINGS
// ------------------------------------------------------------

const (
	BoxPadding          = 3
	CornerRadius        = 8
	VerticalPositionPct = 0.7
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

// ------------------------------------------------------------
// Font handling
// ------------------------------------------------------------

type FontMeasurer struct {
	font *sfnt.Font
	buf  sfnt.Buffer
	ppem fixed.Int26_6
}

func NewFontMeasurer(size float64) (*FontMeasurer, error) {
	f, err := sfnt.Parse(assets.AsapCondensedMedium)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded font: %w", err)
	}

	return &FontMeasurer{
		font: f,
		ppem: fixed.I(int(size)),
	}, nil
}

func (fm *FontMeasurer) TextWidth(text string, fontSize int) int {
	if fm == nil || fm.font == nil {
		return len([]rune(text)) * fontSize / 3
	}

	var w fixed.Int26_6
	for _, r := range text {
		idx, err := fm.font.GlyphIndex(&fm.buf, r)
		if err != nil {
			continue
		}
		adv, err := fm.font.GlyphAdvance(&fm.buf, idx, fm.ppem, font.HintingNone)
		if err != nil {
			continue
		}
		w += adv
	}

	return int(float64(w) * float64(fontSize) / 64 / float64(fm.ppem))
}

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

var tagRegex = regexp.MustCompile(`\{[^}]*\}`)

func removeTags(s string) string {
	return tagRegex.ReplaceAllString(s, "")
}

// ------------------------------------------------------------
// ASS rounded rectangle drawing
// ------------------------------------------------------------

func roundedRectPath(w, h, r int) string {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}

	c := int(float64(r) * 0.5522847498)

	halfW := w / 2
	halfH := h / 2

	var b strings.Builder
	fmt.Fprintf(&b, "m %d %d ", -halfW+r, -halfH)
	fmt.Fprintf(&b, "l %d %d ", halfW-r, -halfH)
	fmt.Fprintf(&b, "b %d %d %d %d %d %d ", halfW-r+c, -halfH, halfW, -halfH+r-c, halfW, -halfH+r)
	fmt.Fprintf(&b, "l %d %d ", halfW, halfH-r)
	fmt.Fprintf(&b, "b %d %d %d %d %d %d ", halfW, halfH-r+c, halfW-r+c, halfH, halfW-r, halfH)
	fmt.Fprintf(&b, "l %d %d ", -halfW+r, halfH)
	fmt.Fprintf(&b, "b %d %d %d %d %d %d ", -halfW+r-c, halfH, -halfW, halfH-r+c, -halfW, halfH-r)
	fmt.Fprintf(&b, "l %d %d ", -halfW, -halfH+r)
	fmt.Fprintf(&b, "b %d %d %d %d %d %d", -halfW, -halfH+r-c, -halfW+r-c, -halfH, -halfW+r, -halfH)

	return b.String()
}

// ------------------------------------------------------------
// Main processing
// ------------------------------------------------------------

func addRoundedBackground(input string, subtitleAspect types.SubtitleAspect, aspectCfg config.AspectConfig) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	fm, err := NewFontMeasurer(float64(aspectCfg.FontSize))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load font (%v), using fallback estimation\n", err)
	}

	lines := strings.Split(string(data), "\n")
	var out []string

	inEvents := false
	styleInjected := false
	foundScriptInfo := false
	playResWritten := false

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		// Case-insensitive check for [Script Info]
		if strings.EqualFold(trim, "[Script Info]") {
			out = append(out, line)
			out = append(out, fmt.Sprintf("PlayResX: %d", aspectCfg.Width))
			out = append(out, fmt.Sprintf("PlayResY: %d", aspectCfg.Height))
			foundScriptInfo = true
			playResWritten = true
			continue
		}

		// Skip any existing PlayResX/PlayResY lines to avoid duplicates
		if foundScriptInfo && (strings.HasPrefix(strings.ToLower(trim), "playresx:") ||
			strings.HasPrefix(strings.ToLower(trim), "playresy:")) {
			continue
		}

		// Case-insensitive check for [V4+ Styles]
		if strings.EqualFold(trim, "[V4+ Styles]") || strings.EqualFold(trim, "[V4 Styles]") {
			// If we haven't written PlayRes yet, we need to add [Script Info] first
			if !playResWritten {
				out = append(out, "[Script Info]")
				out = append(out, fmt.Sprintf("PlayResX: %d", aspectCfg.Width))
				out = append(out, fmt.Sprintf("PlayResY: %d", aspectCfg.Height))
				out = append(out, "")
				playResWritten = true
			}
			out = append(out, line)
			styleInjected = true
			continue
		}

		if styleInjected && strings.HasPrefix(trim, "Format:") {
			out = append(out, line)
			out = append(out,
				"Style: RoundedBox,Asap Condensed Medium,"+strconv.Itoa(aspectCfg.FontSize)+
					",&H30000000,&H00000000,&H00000000,&H00000000,"+
					"0,0,0,0,100,100,0,0,1,0,0,5,10,10,10,1",
			)
			out = append(out,
				"Style: SubText,Asap Condensed Medium,"+strconv.Itoa(aspectCfg.FontSize)+
					",&H00FFFFFF,&H00000000,&H00000000,&H00000000,"+
					"1,0,0,0,100,100,0,0,1,0,0,5,10,10,10,1",
			)
			styleInjected = false
			continue
		}

		if strings.EqualFold(trim, "[Events]") {
			inEvents = true
			out = append(out, line)
			continue
		}

		if inEvents && strings.HasPrefix(trim, "Dialogue:") {
			fields := strings.SplitN(line, ",", 10)
			if len(fields) < 10 {
				out = append(out, line)
				continue
			}

			plain := removeTags(fields[9])
			boxW := fm.TextWidth(plain, aspectCfg.FontSize) + BoxPadding*2 - 20
			boxH := aspectCfg.FontSize + BoxPadding*2
			path := roundedRectPath(boxW, boxH, CornerRadius)

			posX := aspectCfg.Width / 2
			posY := int(float64(aspectCfg.Height) * VerticalPositionPct)

			// posTag := fmt.Sprintf("{\\an5\\pos(%d,%d)}", posX, posY)

			// Background - offset X position
			bgPosTag := fmt.Sprintf("{\\an5\\pos(%d,%d)}", posX+int(boxW/2), posY+int(boxH/2))

			// Text
			txtPosTag := fmt.Sprintf("{\\an5\\pos(%d,%d)}", posX, posY)
			// Background
			bg := make([]string, len(fields))
			copy(bg, fields)
			bg[0] = "Dialogue: 0"
			bg[3] = "RoundedBox"
			bg[9] = bgPosTag + "{\\p1}" + path + "{\\p0}"

			// Text
			txt := make([]string, len(fields))
			copy(txt, fields)
			txt[0] = "Dialogue: 1"
			txt[3] = "SubText"
			txt[9] = txtPosTag + plain

			out = append(out, strings.Join(bg, ","))
			out = append(out, strings.Join(txt, ","))
			continue
		}

		out = append(out, line)
	}

	// Final safety check: if we still haven't written PlayRes, prepend it
	if !playResWritten {
		header := []string{
			"[Script Info]",
			fmt.Sprintf("PlayResX: %d", aspectCfg.Width),
			fmt.Sprintf("PlayResY: %d", aspectCfg.Height),
			"",
		}
		out = append(header, out...)
	}

	return os.WriteFile(subtitleAspect.Path, []byte(strings.Join(out, "\n")), 0644)
}
