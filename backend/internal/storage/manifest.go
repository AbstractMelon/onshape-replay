package storage

import (
	"encoding/json"
	"os"
	"time"
)

// JobStatus mirrors the render package's status enum at the manifest level.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

// Feature is a local copy of the onshape.Feature type used in manifests.
// Defined here to avoid a dependency on the onshape package from storage.
type Feature struct {
	ID         string `json:"featureId"`
	Name       string `json:"name"`
	Type       string `json:"featureType"`
	Suppressed bool   `json:"suppressed"`
}

// Manifest is the self-describing job record written to manifest.json.
type Manifest struct {
	// Identity
	JobID       string `json:"jobId"`
	DocumentID  string `json:"documentId"`
	WorkspaceID string `json:"workspaceId"`
	ElementID   string `json:"elementId"`

	// Configuration snapshot
	ExportConfig ExportConfig `json:"exportConfig"`

	// Feature list snapshot
	Features []Feature `json:"features"`

	// Status
	Status   JobStatus `json:"status"`
	ErrorMsg string    `json:"error,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Outputs (populated when completed)
	Outputs []OutputFile `json:"outputs,omitempty"`
}

// ExportConfig is a snapshot of the user's export configuration at job-start time.
type ExportConfig struct {
	Resolution   string   `json:"resolution"`
	FrameRate    int      `json:"frameRate"`
	CameraMode   string   `json:"cameraMode"`
	BgColor      string   `json:"bgColor"`
	Transparent  bool     `json:"transparent"`
	HoldFirst    int      `json:"holdFirst"`
	HoldLast     int      `json:"holdLast"`
	FileNaming   string   `json:"fileNaming"`
	Formats      []string `json:"formats"`
	FeatureLabel bool     `json:"featureLabel"`
	SkipSketches bool     `json:"skipSketches"`
	SkipSuppressed bool  `json:"skipSuppressed"`
	SkipConstruction bool `json:"skipConstruction"`
	GeometryOnly bool     `json:"geometryOnly"`
}

// OutputFile describes a generated output file.
type OutputFile struct {
	Format string `json:"format"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

// WriteManifest serializes and writes the manifest to disk.
func WriteManifest(path string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ReadManifest reads and deserializes a manifest from disk.
func ReadManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
