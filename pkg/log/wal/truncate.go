package wal

import (
	"fmt"
	"io"
	"os"
	"storemy/pkg/log/record"
	"storemy/pkg/primitives"
)

// TruncateConfig configures WAL truncation behavior
type TruncateConfig struct {
	// Enable automatic truncation after checkpoint
	Enabled bool

	// Minimum WAL size before truncation (avoid truncating tiny files)
	MinWALSizeForTruncation int64

	// Keep at least this many bytes of WAL history
	MinRetainedSize int64
}

// DefaultTruncateConfig returns sensible defaults
func DefaultTruncateConfig() TruncateConfig {
	return TruncateConfig{
		Enabled:                 true,
		MinWALSizeForTruncation: 5 * 1024 * 1024,  // 5MB
		MinRetainedSize:         1 * 1024 * 1024,  // 1MB
	}
}

// TruncateWAL truncates the WAL after a successful checkpoint
// This removes old log records that are no longer needed for recovery
//
// Safety rules:
// 1. Never truncate before the oldest active transaction's FirstLSN
// 2. Never truncate before the oldest dirty page's FirstDirtyLSN
// 3. Keep at least the checkpoint record itself
//
// Returns the number of bytes truncated
func (w *WAL) TruncateWAL(checkpoint *record.CheckpointRecord, config TruncateConfig) (int64, error) {
	if !config.Enabled {
		return 0, nil
	}

	// Check current WAL size
	info, err := w.file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat WAL file: %w", err)
	}

	currentSize := info.Size()
	if currentSize < config.MinWALSizeForTruncation {
		// WAL is too small, skip truncation
		return 0, nil
	}

	// Calculate the safe truncation point
	truncateLSN := w.calculateTruncationPoint(checkpoint)
	if truncateLSN == 0 {
		// Can't truncate anything
		return 0, nil
	}

	// Ensure we retain minimum size
	if int64(truncateLSN) < config.MinRetainedSize {
		truncateLSN = primitives.LSN(config.MinRetainedSize)
	}

	// Don't truncate if we wouldn't save much space
	bytesToTruncate := int64(truncateLSN)
	if bytesToTruncate < currentSize/10 {
		// Would only save < 10% of space, skip
		return 0, nil
	}

	fmt.Printf("Truncating WAL: current size=%d, truncate LSN=%d, will save=%d bytes\n",
		currentSize, truncateLSN, bytesToTruncate)

	// Perform the actual truncation
	if err := w.performTruncation(truncateLSN); err != nil {
		return 0, fmt.Errorf("failed to truncate WAL: %w", err)
	}

	return bytesToTruncate, nil
}

// calculateTruncationPoint determines the safe LSN to truncate up to
func (w *WAL) calculateTruncationPoint(checkpoint *record.CheckpointRecord) primitives.LSN {
	// Start with checkpoint LSN as the baseline
	minLSN := checkpoint.LSN

	// Can't truncate before any active transaction started
	for _, txnInfo := range checkpoint.ActiveTxns {
		if txnInfo.FirstLSN < minLSN {
			minLSN = txnInfo.FirstLSN
		}
	}

	// Can't truncate before any page became dirty
	for _, dirtyLSN := range checkpoint.DirtyPages {
		if dirtyLSN < minLSN {
			minLSN = dirtyLSN
		}
	}

	// Safety margin: keep at least some records before the calculated point
	// This helps with debugging and provides additional safety
	const safetyMargin = primitives.LSN(1024) // Keep at least 1KB before
	if minLSN > safetyMargin {
		minLSN -= safetyMargin
	} else {
		minLSN = 0
	}

	return minLSN
}

// performTruncation actually truncates the WAL file
func (w *WAL) performTruncation(truncateLSN primitives.LSN) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Step 1: Flush any pending writes
	if err := w.writer.Close(); err != nil {
		return fmt.Errorf("failed to flush WAL before truncation: %w", err)
	}

	// Step 2: Create a new temporary WAL file
	newWALPath := w.file.Name() + ".truncate.tmp"
	newFile, err := os.OpenFile(newWALPath, os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temporary WAL: %w", err)
	}

	// Step 3: Copy records from truncateLSN onwards to the new file
	oldPath := w.file.Name()
	copiedBytes, err := w.copyWALRecords(oldPath, newFile, truncateLSN)
	if err != nil {
		newFile.Close()
		os.Remove(newWALPath)
		return fmt.Errorf("failed to copy WAL records: %w", err)
	}

	// Step 4: Close the old WAL file
	if err := w.file.Close(); err != nil {
		newFile.Close()
		os.Remove(newWALPath)
		return fmt.Errorf("failed to close old WAL: %w", err)
	}

	// Step 5: Atomically replace old WAL with new WAL
	oldWALPath := oldPath
	backupPath := oldPath + ".old"

	// Rename old WAL to backup
	if err := os.Rename(oldWALPath, backupPath); err != nil {
		newFile.Close()
		return fmt.Errorf("failed to backup old WAL: %w", err)
	}

	// Rename new WAL to active WAL
	newFile.Close()
	if err := os.Rename(newWALPath, oldWALPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, oldWALPath)
		return fmt.Errorf("failed to activate new WAL: %w", err)
	}

	// Step 6: Reopen the new WAL file
	file, err := os.OpenFile(oldWALPath, os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to reopen WAL: %w", err)
	}

	// Step 7: Recreate the writer with adjusted LSNs
	// LSNs in the new file start from 0, but we need to continue from where we were
	w.file = file
	w.writer = NewLogWriter(file, w.writer.bufferSize, primitives.LSN(copiedBytes), primitives.LSN(copiedBytes))

	// Step 8: Update dirty page table LSNs (subtract truncateLSN)
	newDirtyPages := make(map[primitives.PageID]primitives.LSN)
	for pageID, lsn := range w.dirtyPages {
		if lsn >= truncateLSN {
			newDirtyPages[pageID] = lsn - truncateLSN
		}
	}
	w.dirtyPages = newDirtyPages

	// Step 9: Clean up backup file
	os.Remove(backupPath)

	fmt.Printf("WAL truncation completed: new size=%d bytes\n", copiedBytes)
	return nil
}

// copyWALRecords copies WAL records from startLSN onwards to a new file
func (w *WAL) copyWALRecords(oldPath string, newFile *os.File, startLSN primitives.LSN) (int64, error) {
	reader, err := NewLogReader(oldPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create reader: %w", err)
	}
	defer reader.Close()

	var totalBytes int64
	newLSN := primitives.LSN(0)

	for {
		rec, err := reader.ReadNext()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("failed to read record: %w", err)
		}

		// Skip records before startLSN
		if rec.LSN < startLSN {
			continue
		}

		// Serialize record
		data, err := record.SerializeLogRecord(rec)
		if err != nil {
			return 0, fmt.Errorf("failed to serialize record: %w", err)
		}

		// Write to new file
		if _, err := newFile.WriteAt(data, int64(newLSN)); err != nil {
			return 0, fmt.Errorf("failed to write record: %w", err)
		}

		newLSN += primitives.LSN(len(data))
		totalBytes += int64(len(data))
	}

	// Ensure everything is written to disk
	if err := newFile.Sync(); err != nil {
		return 0, fmt.Errorf("failed to sync new WAL: %w", err)
	}

	return totalBytes, nil
}

// TruncateAfterCheckpoint is a convenience method that performs checkpoint
// followed by automatic truncation
func (w *WAL) TruncateAfterCheckpoint(truncateConfig TruncateConfig) (primitives.LSN, int64, error) {
	// Step 1: Perform checkpoint
	checkpointLSN, err := w.WriteCheckpoint()
	if err != nil {
		return 0, 0, fmt.Errorf("checkpoint failed: %w", err)
	}

	// Step 2: Load the checkpoint we just wrote
	checkpoint, err := w.GetLastCheckpoint()
	if err != nil {
		return checkpointLSN, 0, fmt.Errorf("failed to load checkpoint for truncation: %w", err)
	}

	// Step 3: Truncate WAL
	bytesRemoved, err := w.TruncateWAL(checkpoint, truncateConfig)
	if err != nil {
		return checkpointLSN, 0, fmt.Errorf("truncation failed: %w", err)
	}

	return checkpointLSN, bytesRemoved, nil
}
