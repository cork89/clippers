package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/pipeline"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/workdir"
	"github.com/spf13/cobra"
)

var cfg = config.DefaultConfig()
var aspectsFlag string

var rootCmd = &cobra.Command{
	Use:   "clippers",
	Short: "Audio Clip to Video converter",
	Long:  "Convert audio clips to videos with transcribed subtitles and AI-selected images",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full conversion pipeline",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required")
		}
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required")
		}
		if cfg.OutputDir == "" {
			cfg.OutputDir = "output"
		}

		if aspectsFlag != "" {
			cfg.Aspects = parseAspects(aspectsFlag)
		}

		for _, aspect := range cfg.Aspects {
			if !isValidAspect(aspect) {
				return fmt.Errorf("invalid aspect ratio: %s (valid: 1x1, 16x9, 9x16)", aspect)
			}
		}

		return pipeline.Run(cfg)
	},
}

var captionCmd = &cobra.Command{
	Use:   "caption-images",
	Short: "Caption images only (for debugging)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required")
		}
		if cfg.AudioPath == "" {
			cfg.AudioPath = "dummy"
		}

		wd, err := workdir.NewForImages(cfg)
		if err != nil {
			return err
		}

		catalog, err := pipeline.CaptionImages(wd, cfg, cfg.Force)
		if err != nil {
			return err
		}

		fmt.Println("\n==> Captions:")
		for _, img := range catalog.Images {
			marker := ""
			if img.IsDefault {
				marker = " [DEFAULT]"
			}
			fmt.Printf("\n%s%s:\n  %s\n  Tags: %v\n", img.ID, marker, img.Caption, img.Tags)
		}

		return nil
	},
}

var transcribeCmd = &cobra.Command{
	Use:   "transcribe",
	Short: "Transcribe audio only (for debugging)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required")
		}
		if cfg.ImagesDir == "" {
			cfg.ImagesDir = "."
		}

		wd, err := workdir.New(cfg)
		if err != nil {
			return err
		}

		if _, err := pipeline.NormalizeAudio(wd, cfg.AudioPath, cfg.Force); err != nil {
			return err
		}

		transcript, err := pipeline.Transcribe(wd, cfg, cfg.Force)
		if err != nil {
			return err
		}

		fmt.Println("\n==> Transcript:")
		for _, seg := range transcript.Segments {
			fmt.Printf("[%.1fs - %.1fs] %s\n", seg.Start, seg.End, seg.Text)
		}

		return nil
	},
}

var previewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Show planned timeline without rendering",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required")
		}
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required")
		}

		wd, err := workdir.New(cfg)
		if err != nil {
			return err
		}

		if !wd.Exists("timeline.json") {
			return fmt.Errorf("no timeline found. Run 'clippers run' first")
		}

		var timeline types.Timeline
		if err := wd.ReadJSON("timeline.json", &timeline); err != nil {
			return err
		}

		// Detect default image for highlighting
		defaultImage := pipeline.DetectDefaultImage(cfg.ImagesDir)
		defaultName := ""
		if defaultImage != "" {
			defaultName = filepath.Base(defaultImage)
		}

		fmt.Println("\n==> Timeline Preview:")
		if cfg.Title != "" {
			fmt.Printf("Title: %q\n", cfg.Title)
		}
		fmt.Println(strings.Repeat("-", 60))

		totalDuration := 0.0
		defaultCount := 0

		for i, entry := range timeline.Entries {
			duration := entry.End - entry.Start
			totalDuration = entry.End

			marker := ""
			if entry.ImageID == defaultName {
				marker = " [DEFAULT]"
				defaultCount++
			}

			confStr := ""
			if entry.Confidence > 0 {
				confStr = fmt.Sprintf(" (%.0f%%)", entry.Confidence*100)
			}

			fmt.Printf("  [%2d] %6.1fs - %6.1fs (%4.1fs): %s%s%s\n",
				i+1, entry.Start, entry.End, duration, entry.ImageID, confStr, marker)
		}

		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("  Total: %.1fs | Shots: %d", totalDuration, len(timeline.Entries))
		if defaultCount > 0 {
			fmt.Printf(" | Default used: %d (%.0f%%)", defaultCount, float64(defaultCount)/float64(len(timeline.Entries))*100)
		}
		fmt.Println()

		return nil
	},
}

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Re-render videos from existing timeline",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required")
		}
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required")
		}
		if cfg.OutputDir == "" {
			cfg.OutputDir = "output"
		}

		if aspectsFlag != "" {
			cfg.Aspects = parseAspects(aspectsFlag)
		}

		wd, err := workdir.New(cfg)
		if err != nil {
			return err
		}

		if !wd.Exists("timeline.json") {
			return fmt.Errorf("no timeline found. Run 'clippers run' first")
		}
		if !wd.Exists("subtitles.srt") {
			return fmt.Errorf("no subtitles found. Run 'clippers run' first")
		}
		if !wd.Exists("audio.wav") {
			return fmt.Errorf("no normalized audio found. Run 'clippers run' first")
		}

		var timeline types.Timeline
		if err := wd.ReadJSON("timeline.json", &timeline); err != nil {
			return err
		}

		srtPath := wd.Path("subtitles.srt")

		outputs, err := pipeline.RenderAll(wd, cfg, &timeline, srtPath)
		if err != nil {
			return err
		}

		fmt.Println("\n==> Rendered:")
		for _, output := range outputs {
			fmt.Printf("    • %s\n", output)
		}

		return nil
	},
}

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Run only the planning stage (caption + timeline)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required")
		}
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required")
		}

		// Run preflight for Ollama
		if err := pipeline.Preflight(cfg); err != nil {
			return err
		}

		wd, err := workdir.New(cfg)
		if err != nil {
			return err
		}

		// Normalize and transcribe
		if _, err := pipeline.NormalizeAudio(wd, cfg.AudioPath, cfg.Force); err != nil {
			return err
		}

		transcript, err := pipeline.Transcribe(wd, cfg, cfg.Force)
		if err != nil {
			return err
		}

		// Caption images
		catalog, err := pipeline.CaptionImages(wd, cfg, cfg.Force)
		if err != nil {
			return err
		}

		// Build windows
		windows, err := pipeline.BuildTextWindows(wd, cfg, transcript, cfg.Force)
		if err != nil {
			return err
		}

		// Plan timeline
		timeline, err := pipeline.PlanTimeline(wd, cfg, windows, catalog, cfg.Force)
		if err != nil {
			return err
		}

		fmt.Printf("\n==> Timeline planned: %d shots\n", len(timeline.Entries))
		fmt.Println("Run 'clippers preview' to see details or 'clippers render' to generate videos.")

		return nil
	},
}

func init() {
	// Common flags
	persistentFlags := rootCmd.PersistentFlags()
	persistentFlags.StringVar(&cfg.WorkDir, "workdir", ".work", "Working directory")
	persistentFlags.StringVar(&cfg.OllamaHost, "ollama-host", "http://localhost:11434", "Ollama API host")
	persistentFlags.StringVar(&cfg.VisionModel, "vision-model", "llava", "Vision model for captioning")
	persistentFlags.StringVar(&cfg.SelectModel, "select-model", "gemma3:4b-it-qat", "Model for image selection")
	persistentFlags.BoolVar(&cfg.Force, "force", false, "Force recompute all stages")

	// Run command flags
	runCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	runCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	runCmd.Flags().StringVarP(&cfg.OutputDir, "out", "o", "output", "Output directory")
	runCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title (used for image selection context)")
	runCmd.Flags().StringVar(&aspectsFlag, "aspects", "1x1,16x9,9x16", "Aspect ratios to render")
	runCmd.Flags().Float64Var(&cfg.MinShotSec, "min-shot", 2.5, "Minimum shot duration in seconds")
	runCmd.Flags().IntVar(&cfg.MaxWords, "max-words", 5, "Maximum words per subtitle chunk")
	runCmd.Flags().IntVar(&cfg.BlurStrength, "blur", 20, "Background blur strength")
	runCmd.Flags().IntVar(&cfg.FontSize, "font-size", 24, "Base subtitle font size")
	runCmd.Flags().Float64Var(&cfg.DefaultImageWeight, "default-threshold", 0.5, "Confidence threshold below which default image is used")
	runCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	// Caption command flags
	captionCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")

	// Transcribe command flags
	transcribeCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	transcribeCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	// Preview command flags
	previewCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	previewCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	previewCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title")

	// Render command flags
	renderCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	renderCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	renderCmd.Flags().StringVarP(&cfg.OutputDir, "out", "o", "output", "Output directory")
	renderCmd.Flags().StringVar(&aspectsFlag, "aspects", "1x1,16x9,9x16", "Aspect ratios to render")
	renderCmd.Flags().IntVar(&cfg.BlurStrength, "blur", 20, "Background blur strength")
	renderCmd.Flags().IntVar(&cfg.FontSize, "font-size", 24, "Base subtitle font size")

	// Plan command flags
	planCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	planCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	planCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title (used for image selection context)")
	planCmd.Flags().Float64Var(&cfg.MinShotSec, "min-shot", 2.5, "Minimum shot duration in seconds")
	planCmd.Flags().Float64Var(&cfg.DefaultImageWeight, "default-threshold", 0.5, "Confidence threshold for default image")
	planCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(captionCmd)
	rootCmd.AddCommand(transcribeCmd)
	rootCmd.AddCommand(previewCmd)
	rootCmd.AddCommand(renderCmd)
	rootCmd.AddCommand(planCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func parseAspects(s string) []string {
	parts := strings.Split(s, ",")
	var aspects []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			aspects = append(aspects, p)
		}
	}
	return aspects
}

func isValidAspect(aspect string) bool {
	switch aspect {
	case "1x1", "16x9", "9x16":
		return true
	default:
		return false
	}
}
