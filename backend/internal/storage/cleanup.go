package storage

import (
	"fmt"
	"os"
)

// Removes the job directory and all its contents from disk.
// Used for failed or cancelled jobs to reclaim disk space.
func PurgeJob(storageRoot, documentID, elementID, jobID string) error {
	paths := Layout(storageRoot, documentID, elementID, jobID)
	if err := os.RemoveAll(paths.JobDir); err != nil {
		return fmt.Errorf("PurgeJob: %w", err)
	}
	return nil
}

// Creates the frames directory (and all parents) for a job.
// Must be called before writing any frame files.
func EnsureJobDirs(paths Paths) error {
	if err := os.MkdirAll(paths.FramesDir, 0o755); err != nil {
		return fmt.Errorf("EnsureJobDirs: %w", err)
	}
	return nil
}
