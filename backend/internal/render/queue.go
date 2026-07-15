package render

import (
	"sync"
	"time"

	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// Queue is the in-memory job store.
// Jobs are indexed by ID and by (documentID, workspaceID, elementID) tuple.
type Queue struct {
	mu   sync.RWMutex
	byID map[string]*Job
	// Maps "docId:wsId:elemId" -> most recent job ID for that Part Studio.
	byPart map[string]string
}

// Creates an initialized job queue.
func NewQueue() *Queue {
	return &Queue{
		byID:   make(map[string]*Job),
		byPart: make(map[string]string),
	}
}

// Returns the composite index key for a Part Studio identity tuple.
func partKey(documentID, workspaceID, elementID string) string {
	return documentID + ":" + workspaceID + ":" + elementID
}

// Add registers a new job in the queue.
func (q *Queue) Add(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.byID[job.ID] = job
	q.byPart[partKey(job.DocumentID, job.WorkspaceID, job.ElementID)] = job.ID
}

// Get returns the job with the given ID.
func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	j, ok := q.byID[id]
	return j, ok
}

// Returns the most recent job (running or completed) for a given Part Studio identity tuple.
func (q *Queue) CurrentForPart(documentID, workspaceID, elementID string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	id, ok := q.byPart[partKey(documentID, workspaceID, elementID)]
	if !ok {
		return nil, false
	}
	j, ok := q.byID[id]
	return j, ok
}

// Upddates the progress fields of a job and publishes to subscribers.
// Must be called from the pipeline goroutine.
func (q *Queue) UpdateProgress(jobID string, featureIndex int, featureName string, total int, startedAt time.Time) {
	q.mu.Lock()
	job, ok := q.byID[jobID]
	if !ok {
		q.mu.Unlock()
		return
	}
	job.CurrentFeatureIndex = featureIndex
	job.CurrentFeatureName = featureName
	job.TotalFeatures = total
	if total > 0 {
		job.PercentComplete = float64(featureIndex) / float64(total) * 100
	}
	elapsed := time.Since(startedAt)
	if featureIndex > 0 {
		perFrame := elapsed / time.Duration(featureIndex)
		remaining := perFrame * time.Duration(total-featureIndex)
		job.EstimatedRemaining = remaining
	}
	snap := job.Snapshot()
	broadcaster := job.progress
	q.mu.Unlock()

	if broadcaster != nil {
		broadcaster.Publish(snap)
	}
}

// Transitions a job to a new status and publishes the change.
func (q *Queue) SetStatus(jobID string, status Status, errMsg string) {
	q.mu.Lock()
	job, ok := q.byID[jobID]
	if !ok {
		q.mu.Unlock()
		return
	}
	job.Status = status
	job.ErrorMsg = errMsg
	if status == StatusRunning && job.StartedAt == nil {
		now := time.Now()
		job.StartedAt = &now
	}
	if status.IsTerminal() {
		now := time.Now()
		job.CompletedAt = &now
	}
	snap := job.Snapshot()
	broadcaster := job.progress
	q.mu.Unlock()

	if broadcaster != nil {
		broadcaster.Publish(snap)
		if status.IsTerminal() {
			broadcaster.Close()
		}
	}
}

// Stores the output file list for a completed job.
func (q *Queue) SetOutputs(jobID string, outputs []storage.OutputFile) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, ok := q.byID[jobID]; ok {
		job.Outputs = outputs
	}
}

// Returns a channel that receives progress snapshots for the given
// the channel is closed when the job reaches a terminal state.
// Returns nil, false if the job does not exist.
func (q *Queue) Subscribe(jobID string) (<-chan ProgressSnapshot, bool) {
	q.mu.RLock()
	job, ok := q.byID[jobID]
	if !ok {
		q.mu.RUnlock()
		return nil, false
	}
	broadcaster := job.progress
	q.mu.RUnlock()

	if broadcaster == nil {
		return nil, false
	}
	return broadcaster.Subscribe(), true
}

// Returns a point-in-time copy of job progress.
func (q *Queue) Snapshot(jobID string) (ProgressSnapshot, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	job, ok := q.byID[jobID]
	if !ok {
		return ProgressSnapshot{}, false
	}
	return job.Snapshot(), true
}

// Signals a running job to stop. The pipeline goroutine is responsible
// For actual cleanup; this merely cancels its context.
func (q *Queue) Cancel(jobID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	job, ok := q.byID[jobID]
	if !ok || job.Status != StatusRunning {
		return false
	}
	if job.cancel != nil {
		job.cancel()
	}
	return true
}
