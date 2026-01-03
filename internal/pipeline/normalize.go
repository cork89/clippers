package pipeline

import (
	"fmt"
	"os/exec"

	"github.com/cork89/clippers/internal/workdir"
)

// NormalizeAudio converts input audio to whisper-friendly format
func NormalizeAudio(wd *workdir.WorkDir, inputPath string, force bool) (string, error) {
	outputPath := wd.Path("audio.wav")

	if !force && wd.Exists("audio.wav") {
		fmt.Println("==> Audio normalization (cached)")
		return outputPath, nil
	}

	fmt.Println("==> Normalizing audio")

	// ffmpeg -y -i "input.mp3" -ac 1 -ar 16000 -c:a pcm_s16le "audio.wav"
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	fmt.Printf("  ✓ Normalized audio: %s\n", outputPath)
	return outputPath, nil
}
