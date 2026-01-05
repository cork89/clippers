package types

// Segment represents a transcribed segment from whisper
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Transcript holds the full transcription result
type Transcript struct {
	Language    string    `json:"language"`
	DurationSec float64   `json:"duration_sec"`
	Segments    []Segment `json:"segments"`
}

// TimelineEntry represents a single shot in the video timeline
type TimelineEntry struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	ImageID    string  `json:"image_id"`
	Image      string  `json:"image_path"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

// Timeline is the full video timeline
type Timeline struct {
	Entries []TimelineEntry `json:"entries"`
}

// Project stores the project metadata
type Project struct {
	AudioPath  string            `json:"audio_path"`
	ImagesDir  string            `json:"images_dir"`
	OutputDir  string            `json:"output_dir"`
	AudioHash  string            `json:"audio_hash"`
	ImagesHash string            `json:"images_hash"`
	Settings   map[string]string `json:"settings"`
	CreatedAt  string            `json:"created_at"`
}

// SRTCue represents a single subtitle cue
type SRTCue struct {
	Index int
	Start float64
	End   float64
	Text  string
}

type SubtitleAspect struct {
	Aspect string
	Path   string
}
