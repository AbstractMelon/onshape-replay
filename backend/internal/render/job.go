// Package render owns the job lifecycle: creation, state transitions,
// progress tracking, and orchestrating the capture loop plus the FFmpeg step.
package render

import (
	"context"
	"time"

	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// Status represents the current state of a render job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Job holds all state for a single render job.
type Job struct {
	ID          string
	DocumentID  string
	WorkspaceID string
	ElementID   string

	Config storage.ExportConfig

	Status   Status
	ErrorMsg string

	Features []onshape.Feature

	// Progress tracking
	CurrentFeatureIndex int
	CurrentFeatureName  string
	TotalFeatures       int
	PercentComplete     float64
	EstimatedRemaining  time.Duration

	// Timestamps
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	// Output files populated on completion.
	Outputs []storage.OutputFile

	// cancel stops the pipeline goroutine.
	cancel context.CancelFunc

	// progress is the broadcaster for SSE subscribers.
	progress *ProgressBroadcaster
}

// ProgressSnapshot is a point-in-time copy of job progress safe to share
// across goroutines without holding a lock.
type ProgressSnapshot struct {
	JobID               string              `json:"jobId"`
	Status              Status              `json:"status"`
	CurrentFeatureIndex int                 `json:"currentFeatureIndex"`
	CurrentFeatureName  string              `json:"currentFeatureName"`
	TotalFeatures       int                 `json:"totalFeatures"`
	PercentComplete     float64             `json:"percentComplete"`
	EstimatedRemaining  time.Duration       `json:"estimatedRemaining"`
	ErrorMsg            string              `json:"errorMsg"`
	Outputs             []storage.OutputFile `json:"outputs,omitempty"`
}

// Snapshot returns a copy of the current progress state.
// Callers must hold the queue's read lock or call via Queue methods.
func (j *Job) Snapshot() ProgressSnapshot {
	return ProgressSnapshot{
		JobID:               j.ID,
		Status:              j.Status,
		CurrentFeatureIndex: j.CurrentFeatureIndex,
		CurrentFeatureName:  j.CurrentFeatureName,
		TotalFeatures:       j.TotalFeatures,
		PercentComplete:     j.PercentComplete,
		EstimatedRemaining:  j.EstimatedRemaining,
		ErrorMsg:            j.ErrorMsg,
		Outputs:             j.Outputs,
	}
}

// IsTerminal reports whether the job has reached a final state.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}
