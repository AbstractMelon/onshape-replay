// Package storage owns the on-disk layout for rendered output.
// No other package should construct storage paths directly.
package storage

import (
	"fmt"
	"path/filepath"
)

// Paths returns all derived paths for a given job.
type Paths struct {
	JobDir    string
	FramesDir string
	OutputMP4 string
	OutputGIF string
	OutputZIP string
	Manifest  string
}

// Layout returns the Paths for a given job, rooted at storageRoot.
func Layout(storageRoot, documentID, elementID, jobID string) Paths {
	jobDir := filepath.Join(storageRoot, documentID, elementID, jobID)
	return Paths{
		JobDir:    jobDir,
		FramesDir: filepath.Join(jobDir, "frames"),
		OutputMP4: filepath.Join(jobDir, "output.mp4"),
		OutputGIF: filepath.Join(jobDir, "output.gif"),
		OutputZIP: filepath.Join(jobDir, "output.zip"),
		Manifest:  filepath.Join(jobDir, "manifest.json"),
	}
}

// FrameName returns the zero-padded filename for a frame at the given index.
func FrameName(index int) string {
	return fmt.Sprintf("frame_%04d.png", index)
}

// FramePath returns the full path for frame at index.
func FramePath(paths Paths, index int) string {
	return filepath.Join(paths.FramesDir, FrameName(index))
}

// FrameGlob returns the glob pattern for ffmpeg frame input.
func FrameGlob(paths Paths) string {
	return filepath.Join(paths.FramesDir, "frame_%04d.png")
}
