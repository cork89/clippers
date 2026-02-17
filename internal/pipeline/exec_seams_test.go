// ./internal/pipeline/exec_seams_test.go
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/workdir"
)

func TestNormalizeAudio_ExecCalled(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.mp3")
	if err := os.WriteFile(inputPath, []byte("x"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	wd := &workdir.WorkDir{Root: tmpDir}

	origExec := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = origExec }()

	t.Setenv("CLIPPERS_TEST_MODE", "normalize")
	t.Setenv("CLIPPERS_TEST_INPUT", inputPath)
	t.Setenv("CLIPPERS_TEST_OUTPUT", wd.Path("audio.wav"))

	output, err := NormalizeAudio(wd, inputPath, true)
	if err != nil {
		t.Fatalf("NormalizeAudio error: %v", err)
	}
	if output != wd.Path("audio.wav") {
		t.Fatalf("unexpected output path: %q", output)
	}
}

func TestGetAudioDuration_ParsesOutput(t *testing.T) {
	origExec := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = origExec }()

	t.Setenv("CLIPPERS_TEST_MODE", "ffprobe")
	t.Setenv("CLIPPERS_TEST_DURATION", "12.34")

	value, err := getAudioDuration("/tmp/audio.wav")
	if err != nil {
		t.Fatalf("getAudioDuration error: %v", err)
	}
	if value != 12.34 {
		t.Fatalf("expected 12.34, got %.2f", value)
	}
}

func TestConvertSRTtoASS_UsesSubtitleEdit(t *testing.T) {
	tmpDir := t.TempDir()
	srtPath := filepath.Join(tmpDir, "subtitles.srt")
	if err := os.WriteFile(srtPath, []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n"), 0644); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	assPath := filepath.Join(tmpDir, "custom_output.ass")

	origExec := execCommand
	origLook := execLookPath
	execCommand = fakeExecCommand
	execLookPath = func(name string) (string, error) { return "SubtitleEdit", nil }
	defer func() {
		execCommand = origExec
		execLookPath = origLook
	}()

	t.Setenv("CLIPPERS_TEST_MODE", "subtitleedit")

	if err := convertSRTtoASS(srtPath, assPath); err != nil {
		t.Fatalf("convertSRTtoASS error: %v", err)
	}
	if _, err := os.Stat(assPath); err != nil {
		t.Fatalf("expected output file, got error: %v", err)
	}
}

func TestRenderFinalPass_AssFilter(t *testing.T) {
	tmpDir := t.TempDir()
	wd := &workdir.WorkDir{Root: tmpDir}
	if err := os.WriteFile(wd.Path("audio.wav"), []byte("x"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	subPath := filepath.Join(tmpDir, "subtitles.ass")
	if err := os.WriteFile(subPath, []byte(""), 0644); err != nil {
		t.Fatalf("write subs: %v", err)
	}

	origExec := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = origExec }()

	t.Setenv("CLIPPERS_TEST_MODE", "renderfinal")
	t.Setenv("CLIPPERS_TEST_FILTER_PREFIX", "ass=")

	if err := renderFinalPass(wd, "input.mp4", subPath, "out.mp4", dummyAspect()); err != nil {
		t.Fatalf("renderFinalPass error: %v", err)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := 0
	for sep < len(args) && args[sep] != "--" {
		sep++
	}
	if sep >= len(args)-1 {
		fmt.Fprintln(os.Stderr, "missing command")
		os.Exit(2)
	}

	cmd := args[sep+1]
	cmdArgs := args[sep+2:]
	mode := os.Getenv("CLIPPERS_TEST_MODE")

	switch cmd {
	case "ffmpeg":
		switch mode {
		case "normalize":
			input := os.Getenv("CLIPPERS_TEST_INPUT")
			output := os.Getenv("CLIPPERS_TEST_OUTPUT")
			if !containsArgPair(cmdArgs, "-i", input) || !contains(cmdArgs, output) {
				fmt.Fprintln(os.Stderr, "missing expected args")
				os.Exit(2)
			}
		case "renderfinal":
			prefix := os.Getenv("CLIPPERS_TEST_FILTER_PREFIX")
			if !argHasPrefix(cmdArgs, "-vf", prefix) {
				fmt.Fprintln(os.Stderr, "missing expected filter")
				os.Exit(2)
			}
		}
		os.Exit(0)
	case "ffprobe":
		if mode == "ffprobe" {
			duration := os.Getenv("CLIPPERS_TEST_DURATION")
			payload, _ := json.Marshal(map[string]map[string]string{"format": {"duration": duration}})
			fmt.Fprint(os.Stdout, string(payload))
			os.Exit(0)
		}
	case "SubtitleEdit":
		if mode == "subtitleedit" {
			outputDir := ""
			srtPath := ""
			for _, arg := range cmdArgs {
				if strings.HasPrefix(arg, "/outputfolder:") {
					outputDir = strings.TrimPrefix(arg, "/outputfolder:")
				}
			}
			if len(cmdArgs) > 1 {
				srtPath = cmdArgs[1]
			}
			if outputDir == "" || srtPath == "" {
				fmt.Fprintln(os.Stderr, "missing args")
				os.Exit(2)
			}
			base := strings.TrimSuffix(filepath.Base(srtPath), filepath.Ext(srtPath)) + ".ass"
			outPath := filepath.Join(outputDir, base)
			_ = os.WriteFile(outPath, []byte("ass"), 0644)
			os.Exit(0)
		}
	}

	fmt.Fprintln(os.Stderr, "unexpected command")
	os.Exit(2)
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperProcess", "--", command}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func containsArgPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func contains(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func argHasPrefix(args []string, flag, prefix string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && strings.HasPrefix(args[i+1], prefix) {
			return true
		}
	}
	return false
}

func dummyAspect() config.AspectConfig {
	return config.AspectConfig{
		Width:    1080,
		Height:   1080,
		FontSize: 40,
		MarginV:  40,
	}
}
