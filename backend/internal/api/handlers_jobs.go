package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/abstractmelon/onshape-replay/internal/render"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// startJobRequest is the body expected by POST /jobs.
type startJobRequest struct {
	DocumentID  string              `json:"documentId"`
	WorkspaceID string              `json:"workspaceId"`
	ElementID   string              `json:"elementId"`
	Config      storage.ExportConfig `json:"config"`
}

// jobResponse is the API representation of a job.
type jobResponse struct {
	ID                  string               `json:"id"`
	DocumentID          string               `json:"documentId"`
	WorkspaceID         string               `json:"workspaceId"`
	ElementID           string               `json:"elementId"`
	Status              string               `json:"status"`
	PercentComplete     float64              `json:"percentComplete"`
	CurrentFeatureName  string               `json:"currentFeatureName"`
	CurrentFeatureIndex int                  `json:"currentFeatureIndex"`
	TotalFeatures       int                  `json:"totalFeatures"`
	EstimatedRemaining  float64              `json:"estimatedRemainingSeconds"`
	CreatedAt           time.Time            `json:"createdAt"`
	StartedAt           *time.Time           `json:"startedAt,omitempty"`
	CompletedAt         *time.Time           `json:"completedAt,omitempty"`
	Outputs             []storage.OutputFile `json:"outputs,omitempty"`
	ErrorMsg            string               `json:"error,omitempty"`
}

func jobToResponse(j *render.Job) jobResponse {
	return jobResponse{
		ID:                  j.ID,
		DocumentID:          j.DocumentID,
		WorkspaceID:         j.WorkspaceID,
		ElementID:           j.ElementID,
		Status:              string(j.Status),
		PercentComplete:     j.PercentComplete,
		CurrentFeatureName:  j.CurrentFeatureName,
		CurrentFeatureIndex: j.CurrentFeatureIndex,
		TotalFeatures:       j.TotalFeatures,
		EstimatedRemaining:  j.EstimatedRemaining.Seconds(),
		CreatedAt:           j.CreatedAt,
		StartedAt:           j.StartedAt,
		CompletedAt:         j.CompletedAt,
		Outputs:             j.Outputs,
		ErrorMsg:            j.ErrorMsg,
	}
}

// makeStartJobHandler handles POST /jobs.
func makeStartJobHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.DocumentID == "" || req.WorkspaceID == "" || req.ElementID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documentId, workspaceId, and elementId are required"})
			return
		}

		job := &render.Job{
			ID:          uuid.New().String(),
			DocumentID:  req.DocumentID,
			WorkspaceID: req.WorkspaceID,
			ElementID:   req.ElementID,
			Config:      req.Config,
			Status:      render.StatusPending,
			CreatedAt:   time.Now(),
		}

		svc.Queue.Add(job)

		sess := getSession(svc, r)
		onshapeClient := svc.Onshape(sess)

		deps := render.Dependencies{
			Onshape:     onshapeClient,
			Encoder:     svc.Encoder,
			Queue:       svc.Queue,
			Log:         svc.Log,
			StorageRoot: svc.StorageRoot,
		}
		render.StartPipeline(job, deps)

		writeJSON(w, http.StatusCreated, jobToResponse(job))
	}
}

// makeCurrentJobHandler handles GET /jobs/current.
func makeCurrentJobHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		docID := q.Get("documentId")
		wsID := q.Get("workspaceId")
		elemID := q.Get("elementId")

		if docID == "" || wsID == "" || elemID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documentId, workspaceId, and elementId are required"})
			return
		}

		job, ok := svc.Queue.CurrentForPart(docID, wsID, elemID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no job found"})
			return
		}

		writeJSON(w, http.StatusOK, jobToResponse(job))
	}
}

// makeGetJobHandler handles GET /jobs/{jobId}.
func makeGetJobHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		job, ok := svc.Queue.Get(jobID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, jobToResponse(job))
	}
}

// makeCancelHandler handles POST /jobs/{jobId}/cancel.
func makeCancelHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		if !svc.Queue.Cancel(jobID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job not found or not running"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
	}
}

// makeManifestHandler handles GET /jobs/{jobId}/manifest.
func makeManifestHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")

		// Try the in-memory queue first.
		if job, ok := svc.Queue.Get(jobID); ok {
			paths := storage.Layout(svc.StorageRoot, job.DocumentID, job.ElementID, jobID)
			manifest, err := storage.ReadManifest(paths.Manifest)
			if err != nil {
				// Manifest may not exist yet for in-progress jobs; return the in-memory state.
				writeJSON(w, http.StatusOK, jobToResponse(job))
				return
			}
			writeJSON(w, http.StatusOK, manifest)
			return
		}

		// Job not in queue (server restarted). Search disk for the manifest.
		manifest, err := findManifest(svc.StorageRoot, jobID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, manifest)
	}
}

// findManifest searches the storage tree for a completed job's manifest on disk.
// Path layout: {storageRoot}/{documentId}/{elementId}/{jobId}/manifest.json
func findManifest(storageRoot, jobID string) (*storage.Manifest, error) {
	matches, err := filepath.Glob(filepath.Join(storageRoot, "*", "*", jobID, "manifest.json"))
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		manifest, err := storage.ReadManifest(m)
		if err == nil {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("no manifest found for job %s", jobID)
}

// makePreviewHandler handles POST /jobs/preview.
// Captures a single frame with the given config and returns it as a PNG.
func makePreviewHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.DocumentID == "" || req.WorkspaceID == "" || req.ElementID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documentId, workspaceId, and elementId are required"})
			return
		}

		client := onshapeClientForReq(svc, r)

		pngBytes, err := render.CapturePreview(r.Context(), client, req.DocumentID, "w", req.WorkspaceID, req.ElementID, req.Config)
		if err != nil {
			svc.Log.Error("preview capture failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(pngBytes)
	}
}

// makeDownloadHandler handles GET /jobs/{jobId}/download/{format}.
func makeDownloadHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		format := chi.URLParam(r, "format")

		docID, elemID := lookupJobIDs(svc, jobID)

		paths := storage.Layout(svc.StorageRoot, docID, elemID, jobID)

		var filePath string
		var contentType string
		var downloadName string

		switch format {
		case "mp4":
			filePath = paths.OutputMP4
			contentType = "video/mp4"
			downloadName = fmt.Sprintf("replay-%s.mp4", jobID[:8])
		case "gif":
			filePath = paths.OutputGIF
			contentType = "image/gif"
			downloadName = fmt.Sprintf("replay-%s.gif", jobID[:8])
		case "zip":
			filePath = paths.OutputZIP
			contentType = "application/zip"
			downloadName = fmt.Sprintf("replay-%s-frames.zip", jobID[:8])
		default:
			http.Error(w, "unsupported format", http.StatusBadRequest)
			return
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(downloadName)))
		http.ServeFile(w, r, filePath)
	}
}

// lookupJobIDs returns (documentID, elementID) for a job, preferring the
// in-memory queue and falling back to the disk manifest.
func lookupJobIDs(svc Services, jobID string) (string, string) {
	if job, ok := svc.Queue.Get(jobID); ok {
		return job.DocumentID, job.ElementID
	}
	manifest, err := findManifest(svc.StorageRoot, jobID)
	if err != nil {
		return "", ""
	}
	return manifest.DocumentID, manifest.ElementID
}
