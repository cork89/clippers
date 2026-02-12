package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cork89/clippers/internal/config"
	"github.com/cork89/clippers/internal/database"
	"github.com/cork89/clippers/internal/pipeline"
	"github.com/cork89/clippers/internal/types"
	"github.com/cork89/clippers/internal/webserver"
	"github.com/cork89/clippers/internal/workdir"
	"github.com/spf13/cobra"
)

var cfg = config.DefaultConfig()
var aspectsFlag string
var shaderFlag string

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

		if shaderFlag != "" {
			if !config.IsValidShader(shaderFlag) {
				return fmt.Errorf("invalid shader: %s (valid: %s)", shaderFlag, validShadersStr())
			}
			cfg.Shader = config.ShaderType(shaderFlag)
		}

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		return pipeline.Run(context.Background(), cfg, db)
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

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		wd, err := workdir.NewForImages(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		catalog, err := pipeline.CaptionImages(context.Background(), wd, cfg, db, cfg.Force)
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

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		wd, err := workdir.New(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		if _, err := pipeline.NormalizeAudio(wd, cfg.AudioPath, cfg.Force); err != nil {
			return err
		}

		transcript, err := pipeline.Transcribe(context.Background(), wd, cfg, db, cfg.Force)
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

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		wd, err := workdir.New(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		exists, _ := db.Queries.TimelineExists(context.Background(), wd.ProjectID())
		if exists != 1 {
			return fmt.Errorf("no timeline found. Run 'clippers run' first")
		}

		timeline, err := db.GetTimeline(context.Background(), wd.ProjectID())
		if err != nil {
			return err
		}

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

		if shaderFlag != "" {
			if !config.IsValidShader(shaderFlag) {
				return fmt.Errorf("invalid shader: %s (valid: %s)", shaderFlag, validShadersStr())
			}
			cfg.Shader = config.ShaderType(shaderFlag)
		}

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		wd, err := workdir.New(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		exists, _ := db.Queries.TimelineExists(context.Background(), wd.ProjectID())
		if exists != 1 {
			return fmt.Errorf("no timeline found. Run 'clippers run' first")
		}
		if !wd.Exists("audio.wav") {
			return fmt.Errorf("no normalized audio found. Run 'clippers run' first")
		}

		timeline, err := db.GetTimeline(context.Background(), wd.ProjectID())
		if err != nil {
			return err
		}

		subtitleAspects := make([]types.SubtitleAspect, 0)

		for _, aspect := range cfg.Aspects {
			if wd.Exists(fmt.Sprintf("subtitles_%s.ass", aspect)) {
				subtitleAspects = append(subtitleAspects, types.SubtitleAspect{Aspect: aspect, Path: wd.Path(fmt.Sprintf("subtitles_%s.ass", aspect))})
			} else if wd.Exists("subtitles.srt") {
				subtitleAspects = append(subtitleAspects, types.SubtitleAspect{Aspect: aspect, Path: wd.Path("subtitles.srt")})
			} else {
				return fmt.Errorf("no subtitles found. Run 'clippers run' first")
			}
		}

		outputs, err := pipeline.RenderAll(wd, cfg, timeline, subtitleAspects)
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

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		wd, err := workdir.New(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		if err := pipeline.Preflight(cfg, wd); err != nil {
			return err
		}

		if _, err := pipeline.NormalizeAudio(wd, cfg.AudioPath, cfg.Force); err != nil {
			return err
		}

		transcript, err := pipeline.Transcribe(context.Background(), wd, cfg, db, cfg.Force)
		if err != nil {
			return err
		}

		catalog, err := pipeline.CaptionImages(context.Background(), wd, cfg, db, cfg.Force)
		if err != nil {
			return err
		}

		windows, err := pipeline.BuildTextWindows(context.Background(), wd, cfg, db, transcript, cfg.Force)
		if err != nil {
			return err
		}

		timeline, err := pipeline.PlanTimeline(context.Background(), wd, cfg, db, windows, catalog, cfg.Force)
		if err != nil {
			return err
		}

		fmt.Printf("\n==> Timeline planned: %d shots\n", len(timeline.Entries))
		fmt.Println("Run 'clippers preview' to see details or 'clippers render' to generate videos.")

		return nil
	},
}

var shadersCmd = &cobra.Command{
	Use:   "shaders",
	Short: "List available shader effects",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available shaders:")
		fmt.Println()
		for _, shader := range config.ValidShaders() {
			desc := shaderDescription(shader)
			fmt.Printf("  %-15s %s\n", shader, desc)
		}
		fmt.Println()
		fmt.Println("Use --shader=<name> with 'run' or 'render' commands")
	},
}

var serverPort int
var projectsDir string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web UI server",
	Long:  "Start a local web server for the interactive timeline editor. If --audio and --images are not provided, shows a project selector using --projects-dir.",
	RunE: func(cmd *cobra.Command, args []string) error {
		useProjectSelector := projectsDir != "" || (cfg.AudioPath == "" && cfg.ImagesDir == "")

		db, err := database.Open(".clippers.db")
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		if useProjectSelector {
			if projectsDir == "" {
				projectsDir = "projects"
			}

			if _, err := os.Stat(projectsDir); err != nil {
				return fmt.Errorf("projects directory not found: %s", projectsDir)
			}

			fmt.Printf("Starting server with project selector (projects dir: %s)\n", projectsDir)
			fmt.Printf("Open http://localhost:%d in your browser\n\n", serverPort)

			server := webserver.NewServer(cfg, nil, db, serverPort, projectsDir)
			return server.Start()
		}

		if cfg.AudioPath == "" {
			return fmt.Errorf("--audio is required when not using project selector")
		}
		if cfg.ImagesDir == "" {
			return fmt.Errorf("--images is required when not using project selector")
		}

		wd, err := workdir.New(context.Background(), cfg, db)
		if err != nil {
			return err
		}

		exists, _ := db.Queries.TimelineExists(context.Background(), wd.ProjectID())
		if exists != 1 {
			fmt.Println("No existing timeline found. Running planning stage first...")

			if err := pipeline.Preflight(cfg, wd); err != nil {
				return err
			}

			if _, err := pipeline.NormalizeAudio(wd, cfg.AudioPath, cfg.Force); err != nil {
				return err
			}

			transcript, err := pipeline.Transcribe(context.Background(), wd, cfg, db, cfg.Force)
			if err != nil {
				return err
			}

			catalog, err := pipeline.CaptionImages(context.Background(), wd, cfg, db, cfg.Force)
			if err != nil {
				return err
			}

			windows, err := pipeline.BuildTextWindows(context.Background(), wd, cfg, db, transcript, cfg.Force)
			if err != nil {
				return err
			}

			_, err = pipeline.PlanTimeline(context.Background(), wd, cfg, db, windows, catalog, cfg.Force)
			if err != nil {
				return err
			}

			fmt.Printf("Timeline created successfully!\n\n")
		}

		server := webserver.NewServer(cfg, wd, db, serverPort, "")
		return server.Start()
	},
}

func shaderDescription(s config.ShaderType) string {
	switch s {
	case config.ShaderNone:
		return "(no effect)"
	case config.ShaderWaveDisplace:
		return "RGB channel wave displacement with chromatic aberration"
	case config.ShaderEdgeGlow:
		return "Neon edge detection with cyan/pink pulse"
	case config.ShaderLiquidFlow:
		return "Liquid pastel marble warping effect"
	case config.ShaderPixelMelt:
		return "Digital pixel melting based on brightness"
	case config.ShaderRetro:
		return "VHS tape effect with tracking, grain, and scanlines"
	case config.ShaderVoronoi:
		return "Geometric mosaic with soft sweeping shine"
	default:
		return ""
	}
}

func init() {
	persistentFlags := rootCmd.PersistentFlags()
	persistentFlags.StringVar(&cfg.WorkDir, "workdir", ".work", "Working directory")
	persistentFlags.StringVar(&cfg.OllamaHost, "ollama-host", "http://localhost:11434", "Ollama API host")
	persistentFlags.StringVar(&cfg.VisionModel, "vision-model", "llava", "Vision model for captioning")
	persistentFlags.StringVar(&cfg.SelectModel, "select-model", "gemma3:4b-it-qat", "Model for image selection")
	persistentFlags.BoolVar(&cfg.Force, "force", false, "Force recompute all stages")
	persistentFlags.StringVar(&cfg.ShadersDir, "shaders-dir", "shaders", "Directory containing shader files")

	runCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	runCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	runCmd.Flags().StringVarP(&cfg.OutputDir, "out", "o", "output", "Output directory")
	runCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title (used for image selection context)")
	runCmd.Flags().StringVar(&aspectsFlag, "aspects", "1x1,16x9,9x16", "Aspect ratios to render")
	runCmd.Flags().StringVar(&shaderFlag, "shader", "none", "Shader effect to apply (use 'clippers shaders' to list)")
	runCmd.Flags().Float64Var(&cfg.MinShotSec, "min-shot", cfg.MinShotSec, "Minimum shot duration in seconds")
	runCmd.Flags().IntVar(&cfg.MaxWords, "max-words", cfg.MaxWords, "Maximum words per subtitle chunk")
	runCmd.Flags().IntVar(&cfg.BlurStrength, "blur", cfg.BlurStrength, "Background blur strength")
	runCmd.Flags().IntVar(&cfg.FontSize, "font-size", cfg.FontSize, "Base subtitle font size")
	runCmd.Flags().Float64Var(&cfg.DefaultImageWeight, "default-threshold", 0.5, "Confidence threshold below which default image is used")
	runCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	captionCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")

	transcribeCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	transcribeCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	previewCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	previewCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	previewCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title")

	renderCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	renderCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	renderCmd.Flags().StringVarP(&cfg.OutputDir, "out", "o", "output", "Output directory")
	renderCmd.Flags().StringVar(&aspectsFlag, "aspects", "1x1,16x9,9x16", "Aspect ratios to render")
	renderCmd.Flags().StringVar(&shaderFlag, "shader", "none", "Shader effect to apply")
	renderCmd.Flags().IntVar(&cfg.BlurStrength, "blur", cfg.BlurStrength, "Background blur strength")
	renderCmd.Flags().IntVar(&cfg.FontSize, "font-size", cfg.FontSize, "Base subtitle font size")

	planCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (required)")
	planCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (required)")
	planCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title (used for image selection context)")
	planCmd.Flags().Float64Var(&cfg.MinShotSec, "min-shot", 5, "Minimum shot duration in seconds")
	planCmd.Flags().Float64Var(&cfg.DefaultImageWeight, "default-threshold", 0.5, "Confidence threshold for default image")
	planCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	serverCmd.Flags().StringVarP(&cfg.AudioPath, "audio", "a", "", "Path to audio file (optional if using --projects-dir)")
	serverCmd.Flags().StringVarP(&cfg.ImagesDir, "images", "i", "", "Path to images directory (optional if using --projects-dir)")
	serverCmd.Flags().StringVar(&projectsDir, "projects-dir", "", "Directory containing project folders (default: 'projects' if audio/images not specified)")
	serverCmd.Flags().StringVarP(&cfg.Title, "title", "t", "", "Video title")
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "Server port")
	serverCmd.Flags().Float64Var(&cfg.MinShotSec, "min-shot", 5, "Minimum shot duration in seconds")
	serverCmd.Flags().Float64Var(&cfg.DefaultImageWeight, "default-threshold", 0.5, "Confidence threshold for default image")
	serverCmd.Flags().StringVar(&cfg.WhisperModel, "whisper-model", "medium.en", "Whisper model name")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(captionCmd)
	rootCmd.AddCommand(transcribeCmd)
	rootCmd.AddCommand(previewCmd)
	rootCmd.AddCommand(renderCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(shadersCmd)
	rootCmd.AddCommand(serverCmd)
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

func validShadersStr() string {
	var names []string
	for _, s := range config.ValidShaders() {
		names = append(names, string(s))
	}
	return strings.Join(names, ", ")
}
