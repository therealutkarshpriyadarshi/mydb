package wal

import (
	"os"
	"storemy/pkg/log/record"
	"storemy/pkg/primitives"
	"storemy/pkg/storage/page"
	"testing"
	"time"
)

// TestCheckpointRecordSerialization tests checkpoint record serialization/deserialization
func TestCheckpointRecordSerialization(t *testing.T) {
	// Create sample checkpoint data
	activeTxns := map[*primitives.TransactionID]*record.TransactionLogInfo{
		primitives.NewTransactionIDFromValue(1): {
			FirstLSN:    100,
			LastLSN:     200,
			UndoNextLSN: 150,
		},
		primitives.NewTransactionIDFromValue(2): {
			FirstLSN:    300,
			LastLSN:     400,
			UndoNextLSN: 350,
		},
	}

	dirtyPages := map[primitives.PageID]primitives.LSN{
		page.NewPageDescriptor(primitives.FileID(1), primitives.PageNumber(0)): 100,
		page.NewPageDescriptor(primitives.FileID(1), primitives.PageNumber(1)): 200,
		page.NewPageDescriptor(primitives.FileID(2), primitives.PageNumber(0)): 300,
	}

	// Create checkpoint record
	cp := record.NewCheckpointRecord(activeTxns, dirtyPages)
	cp.LSN = 500

	// Serialize
	data, err := record.SerializeCheckpoint(cp)
	if err != nil {
		t.Fatalf("Failed to serialize checkpoint: %v", err)
	}

	// Deserialize
	cp2, err := record.DeserializeCheckpoint(data)
	if err != nil {
		t.Fatalf("Failed to deserialize checkpoint: %v", err)
	}

	// Verify
	if cp2.LSN != cp.LSN {
		t.Errorf("LSN mismatch: expected %d, got %d", cp.LSN, cp2.LSN)
	}

	if len(cp2.ActiveTxns) != len(cp.ActiveTxns) {
		t.Errorf("ActiveTxns count mismatch: expected %d, got %d", len(cp.ActiveTxns), len(cp2.ActiveTxns))
	}

	if len(cp2.DirtyPages) != len(cp.DirtyPages) {
		t.Errorf("DirtyPages count mismatch: expected %d, got %d", len(cp.DirtyPages), len(cp2.DirtyPages))
	}

	// Verify transaction data
	for tidID, info := range cp.ActiveTxns {
		info2, exists := cp2.ActiveTxns[tidID]
		if !exists {
			t.Errorf("Transaction %d missing in deserialized checkpoint", tidID)
			continue
		}
		if info2.FirstLSN != info.FirstLSN || info2.LastLSN != info.LastLSN || info2.UndoNextLSN != info.UndoNextLSN {
			t.Errorf("Transaction %d data mismatch", tidID)
		}
	}

	// Verify dirty pages
	for pageHash, lsn := range cp.DirtyPages {
		lsn2, exists := cp2.DirtyPages[pageHash]
		if !exists {
			t.Errorf("Page %d missing in deserialized checkpoint", pageHash)
			continue
		}
		if lsn2 != lsn {
			t.Errorf("Page %d LSN mismatch: expected %d, got %d", pageHash, lsn, lsn2)
		}
	}
}

// TestWriteCheckpoint tests writing a checkpoint to WAL
func TestWriteCheckpoint(t *testing.T) {
	// Create temporary WAL
	tmpFile, err := os.CreateTemp("", "wal_checkpoint_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	wal, err := NewWAL(tmpFile.Name(), 4096)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()
	defer os.Remove(tmpFile.Name() + ".checkpoint")

	// Start some transactions
	tid1 := primitives.NewTransactionIDFromValue(1)
	tid2 := primitives.NewTransactionIDFromValue(2)

	if _, err := wal.LogBegin(tid1); err != nil {
		t.Fatalf("Failed to log begin: %v", err)
	}

	if _, err := wal.LogBegin(tid2); err != nil {
		t.Fatalf("Failed to log begin: %v", err)
	}

	// Log some updates
	pageID := page.NewPageDescriptor(primitives.FileID(1), primitives.PageNumber(0))
	before := []byte("before")
	after := []byte("after")

	if _, err := wal.LogUpdate(tid1, pageID, before, after); err != nil {
		t.Fatalf("Failed to log update: %v", err)
	}

	// Write checkpoint
	checkpointLSN, err := wal.WriteCheckpoint()
	if err != nil {
		t.Fatalf("Failed to write checkpoint: %v", err)
	}

	if checkpointLSN == 0 {
		t.Error("Checkpoint LSN should not be 0")
	}

	// Verify checkpoint was written
	checkpoint, err := wal.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	if checkpoint == nil {
		t.Fatal("Checkpoint should exist")
	}

	// Verify checkpoint contains active transactions
	if len(checkpoint.ActiveTxns) != 2 {
		t.Errorf("Expected 2 active transactions, got %d", len(checkpoint.ActiveTxns))
	}

	// Verify checkpoint contains dirty pages
	if len(checkpoint.DirtyPages) != 1 {
		t.Errorf("Expected 1 dirty page, got %d", len(checkpoint.DirtyPages))
	}

	t.Logf("Checkpoint written successfully at LSN %d", checkpointLSN)
}

// TestCheckpointDaemon tests the checkpoint daemon
func TestCheckpointDaemon(t *testing.T) {
	// Create temporary WAL
	tmpFile, err := os.CreateTemp("", "wal_daemon_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	wal, err := NewWAL(tmpFile.Name(), 4096)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()
	defer os.Remove(tmpFile.Name() + ".checkpoint")

	// Create daemon with short interval for testing
	config := CheckpointConfig{
		Interval:        2 * time.Second,
		MaxWALSize:      1024,
		MaxTransactions: 10,
		Enabled:         true,
	}

	daemon := NewCheckpointDaemon(wal, config)

	// Start daemon
	if err := daemon.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Check that daemon is running
	if !daemon.IsRunning() {
		t.Error("Daemon should be running")
	}

	// Wait for at least one automatic checkpoint
	time.Sleep(3 * time.Second)

	// Stop daemon
	if err := daemon.Stop(); err != nil {
		t.Fatalf("Failed to stop daemon: %v", err)
	}

	// Check that daemon stopped
	if daemon.IsRunning() {
		t.Error("Daemon should be stopped")
	}

	// Check stats
	stats := daemon.GetStats()
	if stats.TotalCheckpoints == 0 {
		t.Error("Expected at least one checkpoint")
	}

	t.Logf("Daemon stats: %+v", stats)
}

// TestManualCheckpoint tests manual checkpoint triggering
func TestManualCheckpoint(t *testing.T) {
	// Create temporary WAL
	tmpFile, err := os.CreateTemp("", "wal_manual_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	wal, err := NewWAL(tmpFile.Name(), 4096)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()
	defer os.Remove(tmpFile.Name() + ".checkpoint")

	// Create daemon (but don't start it)
	config := CheckpointConfig{
		Interval:        1 * time.Hour, // Long interval
		MaxWALSize:      100 * 1024 * 1024,
		MaxTransactions: 10000,
		Enabled:         false,
	}

	daemon := NewCheckpointDaemon(wal, config)

	// Trigger manual checkpoint
	lsn, err := daemon.TriggerManualCheckpoint()
	if err != nil {
		t.Fatalf("Manual checkpoint failed: %v", err)
	}

	if lsn == 0 {
		t.Error("Checkpoint LSN should not be 0")
	}

	// Verify checkpoint exists
	checkpoint, err := wal.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	if checkpoint == nil {
		t.Fatal("Checkpoint should exist")
	}

	// Check stats
	stats := daemon.GetStats()
	if stats.ManualTriggers != 1 {
		t.Errorf("Expected 1 manual trigger, got %d", stats.ManualTriggers)
	}

	t.Logf("Manual checkpoint completed at LSN %d", lsn)
}

// TestNoCheckpoint tests behavior when no checkpoint exists
func TestNoCheckpoint(t *testing.T) {
	// Create temporary WAL
	tmpFile, err := os.CreateTemp("", "wal_no_checkpoint_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	wal, err := NewWAL(tmpFile.Name(), 4096)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Try to get checkpoint (should return nil, no error)
	checkpoint, err := wal.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("GetLastCheckpoint should not error: %v", err)
	}

	if checkpoint != nil {
		t.Error("Checkpoint should be nil when none exists")
	}
}

// TestCheckpointWithCommittedTransaction tests checkpoint after transaction commit
func TestCheckpointWithCommittedTransaction(t *testing.T) {
	// Create temporary WAL
	tmpFile, err := os.CreateTemp("", "wal_commit_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	wal, err := NewWAL(tmpFile.Name(), 4096)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()
	defer os.Remove(tmpFile.Name() + ".checkpoint")

	// Start transaction
	tid := primitives.NewTransactionIDFromValue(1)
	if _, err := wal.LogBegin(tid); err != nil {
		t.Fatalf("Failed to log begin: %v", err)
	}

	// Log update
	pageID := page.NewPageDescriptor(primitives.FileID(1), primitives.PageNumber(0))
	if _, err := wal.LogUpdate(tid, pageID, []byte("before"), []byte("after")); err != nil {
		t.Fatalf("Failed to log update: %v", err)
	}

	// Commit transaction
	if _, err := wal.LogCommit(tid); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Write checkpoint
	if _, err := wal.WriteCheckpoint(); err != nil {
		t.Fatalf("Failed to write checkpoint: %v", err)
	}

	// Verify checkpoint
	checkpoint, err := wal.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	// After commit, transaction should not be in active transactions
	if len(checkpoint.ActiveTxns) != 0 {
		t.Errorf("Expected 0 active transactions after commit, got %d", len(checkpoint.ActiveTxns))
	}

	// Dirty page should still be there
	if len(checkpoint.DirtyPages) != 1 {
		t.Errorf("Expected 1 dirty page, got %d", len(checkpoint.DirtyPages))
	}
}
