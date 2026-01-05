// ./internal/pipeline/subtitle_processor.go
package pipeline

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/types"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ------------------------------------------------------------
// USER SETTINGS
// ------------------------------------------------------------

const (
	FontSize            = 48
	BoxPadding          = 3
	CornerRadius        = 8
	VerticalPositionPct = 0.7
)

func defaultFontPath() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\Windows\Fonts\AsapCondensed-Medium.ttf`
	case "darwin":
		return "/Library/Fonts/AsapCondensed-Medium.ttf"
	default:
		return "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	}
}

// ------------------------------------------------------------
// Font handling
// ------------------------------------------------------------

type FontMeasurer struct {
	font *sfnt.Font
	buf  sfnt.Buffer
	ppem fixed.Int26_6
}

func NewFontMeasurer(path string, size float64) (*FontMeasurer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}

	return &FontMeasurer{
		font: f,
		ppem: fixed.I(int(size)),
	}, nil
}

func (fm *FontMeasurer) TextWidth(text string) int {
	if fm == nil || fm.font == nil {
		return len([]rune(text)) * FontSize / 3
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

	return int(float64(w) * float64(FontSize) / 64 / float64(fm.ppem))
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

	fm, err := NewFontMeasurer(defaultFontPath(), FontSize)
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
				"Style: RoundedBox,Asap Condensed Medium,"+strconv.Itoa(FontSize)+
					",&H30000000,&H00000000,&H00000000,&H00000000,"+
					"0,0,0,0,100,100,0,0,1,0,0,5,10,10,10,1",
			)
			out = append(out,
				"Style: SubText,Asap Condensed Medium,"+strconv.Itoa(FontSize)+
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
			boxW := fm.TextWidth(plain) + BoxPadding*2 - 20
			boxH := FontSize + BoxPadding*2
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
