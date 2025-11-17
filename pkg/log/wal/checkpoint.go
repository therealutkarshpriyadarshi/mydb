package wal

import (
	"fmt"
	"os"
	"storemy/pkg/log/record"
	"storemy/pkg/primitives"
	"sync/atomic"
	"time"
)

// checkpointState tracks the last successful checkpoint
type checkpointState struct {
	lastCheckpointLSN atomic.Value // stores primitives.LSN
	checkpointFile    string
}

var globalCheckpointState = &checkpointState{}

// WriteCheckpoint creates a fuzzy checkpoint (non-blocking)
// This captures the current state of active transactions and dirty pages
// without blocking ongoing operations
//
// Returns the LSN of the checkpoint end record
func (w *WAL) WriteCheckpoint() (primitives.LSN, error) {
	fmt.Println("Starting fuzzy checkpoint...")

	// Phase 1: Write CheckpointBegin record
	beginLSN, err := w.writeCheckpointBegin()
	if err != nil {
		return 0, fmt.Errorf("failed to write checkpoint begin: %w", err)
	}

	// Phase 2: Capture snapshot of active transactions and dirty pages (with lock)
	// This is a "fuzzy" checkpoint - we capture the state at a point in time
	// but transactions can continue to run
	w.mutex.RLock()
	activeTxns := make(map[*primitives.TransactionID]*record.TransactionLogInfo)
	for tid, info := range w.activeTxns {
		activeTxns[tid] = &record.TransactionLogInfo{
			FirstLSN:    info.FirstLSN,
			LastLSN:     info.LastLSN,
			UndoNextLSN: info.UndoNextLSN,
		}
	}

	dirtyPages := make(map[primitives.PageID]primitives.LSN)
	for pageID, lsn := range w.dirtyPages {
		dirtyPages[pageID] = lsn
	}
	w.mutex.RUnlock()

	// Phase 3: Create and serialize checkpoint record
	checkpointRec := record.NewCheckpointRecord(activeTxns, dirtyPages)
	checkpointRec.LSN = beginLSN

	checkpointData, err := record.SerializeCheckpoint(checkpointRec)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize checkpoint: %w", err)
	}

	// Phase 4: Write checkpoint data to a separate checkpoint file
	// This allows us to keep checkpoints separate from the main WAL
	// and makes recovery faster (no need to scan entire WAL to find checkpoint)
	checkpointPath := w.getCheckpointPath()
	if err := w.writeCheckpointFile(checkpointPath, checkpointData); err != nil {
		return 0, fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	// Phase 5: Write CheckpointEnd record (this completes the checkpoint)
	endLSN, err := w.writeCheckpointEnd(beginLSN)
	if err != nil {
		return 0, fmt.Errorf("failed to write checkpoint end: %w", err)
	}

	// Phase 6: Force checkpoint records to disk
	if err := w.Force(endLSN); err != nil {
		return 0, fmt.Errorf("failed to force checkpoint to disk: %w", err)
	}

	// Update global checkpoint state
	globalCheckpointState.lastCheckpointLSN.Store(endLSN)
	globalCheckpointState.checkpointFile = checkpointPath

	fmt.Printf("Checkpoint completed: LSN=%d, ActiveTxns=%d, DirtyPages=%d, Size=%d bytes\n",
		endLSN, len(activeTxns), len(dirtyPages), len(checkpointData))

	return endLSN, nil
}

// GetLastCheckpoint retrieves the most recent checkpoint data
// Returns nil if no checkpoint exists
func (w *WAL) GetLastCheckpoint() (*record.CheckpointRecord, error) {
	// Check if we have a checkpoint file
	checkpointPath := w.getCheckpointPath()

	// Try to read checkpoint file
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No checkpoint exists
		}
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	// Deserialize checkpoint
	checkpoint, err := record.DeserializeCheckpoint(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize checkpoint: %w", err)
	}

	return checkpoint, nil
}

// writeCheckpointBegin writes a CheckpointBegin log record
func (w *WAL) writeCheckpointBegin() (primitives.LSN, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	rec := record.NewLogRecord(record.CheckpointBegin, nil, nil, nil, nil, 0)
	lsn, err := w.writeRecord(rec)
	if err != nil {
		return 0, fmt.Errorf("failed to write checkpoint begin record: %w", err)
	}

	return lsn, nil
}

// writeCheckpointEnd writes a CheckpointEnd log record
func (w *WAL) writeCheckpointEnd(beginLSN primitives.LSN) (primitives.LSN, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// CheckpointEnd record references the CheckpointBegin LSN via PrevLSN
	rec := record.NewLogRecord(record.CheckpointEnd, nil, nil, nil, nil, beginLSN)
	lsn, err := w.writeRecord(rec)
	if err != nil {
		return 0, fmt.Errorf("failed to write checkpoint end record: %w", err)
	}

	return lsn, nil
}

// writeCheckpointFile writes checkpoint data to a file
func (w *WAL) writeCheckpointFile(path string, data []byte) error {
	// Write to a temporary file first, then atomically rename
	tempPath := path + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary checkpoint file: %w", err)
	}

	// Atomically rename to final path
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	return nil
}

// getCheckpointPath returns the path to the checkpoint file
func (w *WAL) getCheckpointPath() string {
	// Store checkpoint in the same directory as the WAL
	// Use a fixed name so we can easily find the latest checkpoint
	return w.file.Name() + ".checkpoint"
}

// GetCheckpointStats returns statistics about the last checkpoint
func (w *WAL) GetCheckpointStats() *CheckpointStats {
	var lastLSN primitives.LSN
	if val := globalCheckpointState.lastCheckpointLSN.Load(); val != nil {
		lastLSN = val.(primitives.LSN)
	}

	return &CheckpointStats{
		LastCheckpointLSN:  lastLSN,
		CheckpointFilePath: globalCheckpointState.checkpointFile,
	}
}

// CheckpointStats contains statistics about checkpointing
type CheckpointStats struct {
	LastCheckpointLSN  primitives.LSN
	CheckpointFilePath string
}

// ShouldCheckpoint returns true if a checkpoint should be triggered
// based on WAL size or time since last checkpoint
func (w *WAL) ShouldCheckpoint(maxWALSize int64, maxInterval time.Duration) bool {
	// Check WAL size
	info, err := w.file.Stat()
	if err == nil && info.Size() >= maxWALSize {
		return true
	}

	// Check time since last checkpoint
	checkpointPath := w.getCheckpointPath()
	info, err = os.Stat(checkpointPath)
	if err != nil {
		// No checkpoint exists yet
		return true
	}

	timeSinceCheckpoint := time.Since(info.ModTime())
	return timeSinceCheckpoint >= maxInterval
}
