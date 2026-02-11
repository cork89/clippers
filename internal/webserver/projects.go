package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProjectInfo represents a discovered project in the projects directory
type ProjectInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	AudioFile  string    `json:"audio_file"`
	ImagesDir  string    `json:"images_dir"`
	HasImages  bool      `json:"has_images"`
	ImageCount int       `json:"image_count"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
}

// DiscoverProjects scans the projects directory and returns a list of valid projects
func DiscoverProjects(projectsDir string) ([]ProjectInfo, error) {
	var projects []ProjectInfo

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return projects, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(projectsDir, entry.Name())
		info, err := scanProject(projectPath, entry.Name())
		if err != nil {
			continue
		}

		projects = append(projects, info)
	}

	return projects, nil
}

// scanProject examines a single project directory
func scanProject(projectPath, name string) (ProjectInfo, error) {
	info := ProjectInfo{
		Name: name,
		Path: projectPath,
	}

	// Get directory stats
	stat, err := os.Stat(projectPath)
	if err != nil {
		return info, err
	}
	info.CreatedAt = stat.ModTime()
	info.ModifiedAt = stat.ModTime()

	// Look for audio file (MP3)
	dirEntries, err := os.ReadDir(projectPath)
	if err != nil {
		return info, err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".mp3" {
			info.AudioFile = entry.Name()
			info.ModifiedAt, _ = getModTime(filepath.Join(projectPath, entry.Name()))
			break
		}
	}

	if info.AudioFile == "" {
		return info, os.ErrNotExist
	}

	// Check for images directory
	imagesPath := filepath.Join(projectPath, "images")
	imagesStat, err := os.Stat(imagesPath)
	if err == nil && imagesStat.IsDir() {
		info.ImagesDir = imagesPath
		info.HasImages = true
		info.ImageCount = countImages(imagesPath)
		if imagesStat.ModTime().After(info.ModifiedAt) {
			info.ModifiedAt = imagesStat.ModTime()
		}
	}

	return info, nil
}

// getModTime returns the modification time of a file
func getModTime(path string) (time.Time, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return stat.ModTime(), nil
}

// countImages counts the number of image files in a directory
func countImages(dir string) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp":
			count++
		}
	}

	return count
}
