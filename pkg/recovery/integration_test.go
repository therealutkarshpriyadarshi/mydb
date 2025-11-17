package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"storemy/pkg/log/wal"
	"storemy/pkg/primitives"
)

// TestCrashRecovery_SimpleCommit simulates a crash after a committed transaction
func TestCrashRecovery_SimpleCommit(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Normal operation before crash
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()

		// Execute a transaction
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(100), []byte("initial"), []byte("updated"))
		testWAL.LogCommit(tid)

		// Simulate crash (close WAL without proper shutdown)
		testWAL.Close()
	}

	// Phase 2: Recovery after crash
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		// Check if recovery is needed
		needed, err := rm.IsRecoveryNeeded()
		if err != nil {
			t.Fatalf("IsRecoveryNeeded failed: %v", err)
		}

		// Should not need recovery - transaction was committed
		if needed {
			t.Error("Recovery should not be needed for committed transaction")
		}

		// Run recovery anyway to verify it handles committed transactions
		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		stats := rm.GetStats()
		if stats.TransactionsUndone != 0 {
			t.Errorf("Expected 0 transactions undone, got %d", stats.TransactionsUndone)
		}
	}
}

// TestCrashRecovery_UncommittedTransaction simulates a crash before commit
func TestCrashRecovery_UncommittedTransaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	var crashedTID *primitives.TransactionID

	// Phase 1: Normal operation before crash
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		crashedTID = tid

		// Execute a transaction but don't commit (simulate crash)
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(200), []byte("before_crash"), []byte("after_crash"))
		testWAL.LogInsert(tid, newMockPageID(201), []byte("inserted_data"))

		// Simulate crash - no commit!
		testWAL.Close()
	}

	// Phase 2: Recovery after crash
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		// Check if recovery is needed
		needed, err := rm.IsRecoveryNeeded()
		if err != nil {
			t.Fatalf("IsRecoveryNeeded failed: %v", err)
		}

		if !needed {
			t.Error("Recovery should be needed for uncommitted transaction")
		}

		// Run recovery
		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		// Verify the transaction was undone
		stats := rm.GetStats()
		if stats.TransactionsUndone != 1 {
			t.Errorf("Expected 1 transaction undone, got %d", stats.TransactionsUndone)
		}

		if stats.UndoOperations != 2 {
			t.Errorf("Expected 2 undo operations (update + insert), got %d", stats.UndoOperations)
		}

		// Verify dirty pages were identified
		dirtyPages := rm.GetDirtyPageTable()
		if len(dirtyPages) != 2 {
			t.Errorf("Expected 2 dirty pages, got %d", len(dirtyPages))
		}

		_ = crashedTID // Avoid unused variable warning
	}
}

// TestCrashRecovery_MultipleTransactions simulates crash with mixed transaction states
func TestCrashRecovery_MultipleTransactions(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Normal operation with multiple transactions
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		// Transaction 1: Fully committed
		tid1 := primitives.NewTransactionID()
		testWAL.LogBegin(tid1)
		testWAL.LogUpdate(tid1, newMockPageID(1), []byte("old1"), []byte("new1"))
		testWAL.LogCommit(tid1)

		// Transaction 2: Uncommitted (will need undo)
		tid2 := primitives.NewTransactionID()
		testWAL.LogBegin(tid2)
		testWAL.LogUpdate(tid2, newMockPageID(2), []byte("old2"), []byte("new2"))
		testWAL.LogUpdate(tid2, newMockPageID(3), []byte("old3"), []byte("new3"))
		// No commit!

		// Transaction 3: Aborted (already rolled back)
		tid3 := primitives.NewTransactionID()
		testWAL.LogBegin(tid3)
		testWAL.LogUpdate(tid3, newMockPageID(4), []byte("old4"), []byte("new4"))
		testWAL.LogAbort(tid3)

		// Transaction 4: Committed
		tid4 := primitives.NewTransactionID()
		testWAL.LogBegin(tid4)
		testWAL.LogInsert(tid4, newMockPageID(5), []byte("data5"))
		testWAL.LogCommit(tid4)

		// Transaction 5: Uncommitted (will need undo)
		tid5 := primitives.NewTransactionID()
		testWAL.LogBegin(tid5)
		testWAL.LogInsert(tid5, newMockPageID(6), []byte("data6"))
		// No commit!

		// Simulate crash
		testWAL.Close()
	}

	// Phase 2: Recovery after crash
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		// Check if recovery is needed
		needed, err := rm.IsRecoveryNeeded()
		if err != nil {
			t.Fatalf("IsRecoveryNeeded failed: %v", err)
		}

		if !needed {
			t.Error("Recovery should be needed - uncommitted transactions exist")
		}

		// Run recovery
		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		// Verify statistics
		stats := rm.GetStats()

		if stats.TransactionsRecovered != 5 {
			t.Errorf("Expected 5 transactions recovered, got %d", stats.TransactionsRecovered)
		}

		// Should undo 2 transactions (tid2 and tid5)
		if stats.TransactionsUndone != 2 {
			t.Errorf("Expected 2 transactions undone, got %d", stats.TransactionsUndone)
		}

		// Should have 3 undo operations (2 updates from tid2, 1 insert from tid5)
		if stats.UndoOperations != 3 {
			t.Errorf("Expected 3 undo operations, got %d", stats.UndoOperations)
		}

		// Verify dirty pages
		dirtyPages := rm.GetDirtyPageTable()
		if len(dirtyPages) != 6 {
			t.Errorf("Expected 6 dirty pages, got %d", len(dirtyPages))
		}
	}
}

// TestCrashRecovery_LongRunningTransaction tests recovery with large transaction
func TestCrashRecovery_LongRunningTransaction(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	const numUpdates = 100

	// Phase 1: Create a long-running transaction
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)

		// Perform many updates
		for i := 0; i < numUpdates; i++ {
			pageID := newMockPageID(i)
			testWAL.LogUpdate(tid, pageID, []byte("old"), []byte("new"))
		}

		// Simulate crash before commit
		testWAL.Close()
	}

	// Phase 2: Recovery
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		// Verify all updates were undone
		stats := rm.GetStats()
		if stats.UndoOperations != numUpdates {
			t.Errorf("Expected %d undo operations, got %d", numUpdates, stats.UndoOperations)
		}

		if stats.DirtyPagesFound != numUpdates {
			t.Errorf("Expected %d dirty pages, got %d", numUpdates, stats.DirtyPagesFound)
		}
	}
}

// TestCrashRecovery_InterleavedTransactions tests recovery with interleaved operations
func TestCrashRecovery_InterleavedTransactions(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Create interleaved transactions
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid1 := primitives.NewTransactionID()
		tid2 := primitives.NewTransactionID()
		tid3 := primitives.NewTransactionID()

		// Interleave operations from different transactions
		testWAL.LogBegin(tid1)
		testWAL.LogBegin(tid2)
		testWAL.LogUpdate(tid1, newMockPageID(1), []byte("a"), []byte("b"))
		testWAL.LogUpdate(tid2, newMockPageID(2), []byte("c"), []byte("d"))
		testWAL.LogBegin(tid3)
		testWAL.LogUpdate(tid3, newMockPageID(3), []byte("e"), []byte("f"))
		testWAL.LogUpdate(tid1, newMockPageID(4), []byte("g"), []byte("h"))
		testWAL.LogCommit(tid1) // Only tid1 commits

		testWAL.LogUpdate(tid2, newMockPageID(5), []byte("i"), []byte("j"))
		testWAL.LogUpdate(tid3, newMockPageID(6), []byte("k"), []byte("l"))
		// tid2 and tid3 don't commit - simulate crash

		testWAL.Close()
	}

	// Phase 2: Recovery
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		stats := rm.GetStats()

		// Should undo tid2 and tid3
		if stats.TransactionsUndone != 2 {
			t.Errorf("Expected 2 transactions undone, got %d", stats.TransactionsUndone)
		}

		// Should undo 4 operations (2 from tid2, 2 from tid3)
		if stats.UndoOperations != 4 {
			t.Errorf("Expected 4 undo operations, got %d", stats.UndoOperations)
		}

		// Verify all 6 pages are in dirty page table
		dirtyPages := rm.GetDirtyPageTable()
		if len(dirtyPages) != 6 {
			t.Errorf("Expected 6 dirty pages, got %d", len(dirtyPages))
		}
	}
}

// TestCrashRecovery_EmptyWAL tests recovery with no operations
func TestCrashRecovery_EmptyWAL(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Create empty WAL
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}
		testWAL.Close()
	}

	// Recovery should succeed with no operations
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		needed, err := rm.IsRecoveryNeeded()
		if err != nil {
			t.Fatalf("IsRecoveryNeeded failed: %v", err)
		}

		if needed {
			t.Error("Recovery should not be needed for empty WAL")
		}

		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		stats := rm.GetStats()
		if stats.TransactionsRecovered != 0 {
			t.Errorf("Expected 0 transactions recovered, got %d", stats.TransactionsRecovered)
		}
	}
}

// TestCrashRecovery_RepeatedRecovery tests running recovery multiple times
func TestCrashRecovery_RepeatedRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Create uncommitted transaction
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
		// No commit

		testWAL.Close()
	}

	// Phase 2: First recovery
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}

		rm := NewRecoveryManager(testWAL, walPath, nil)

		err = rm.Recover()
		if err != nil {
			t.Fatalf("First recovery failed: %v", err)
		}

		stats1 := rm.GetStats()

		// Run recovery again on the same instance
		rm.ResetStats()
		err = rm.Recover()
		if err != nil {
			t.Fatalf("Second recovery failed: %v", err)
		}

		stats2 := rm.GetStats()

		// Both recoveries should produce the same results
		if stats1.TransactionsRecovered != stats2.TransactionsRecovered {
			t.Error("Repeated recovery produced different results")
		}

		testWAL.Close()
	}
}

// TestCrashRecovery_CorruptedWAL tests recovery with file issues
func TestCrashRecovery_CorruptedWAL(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Create valid WAL
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))

		testWAL.Close()
	}

	// Phase 2: Corrupt the WAL by truncating it
	{
		file, err := os.OpenFile(walPath, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("Failed to open WAL file: %v", err)
		}

		// Get file size
		info, err := file.Stat()
		if err != nil {
			file.Close()
			t.Fatalf("Failed to stat file: %v", err)
		}

		// Truncate to half size (simulate corruption)
		newSize := info.Size() / 2
		if newSize > 0 {
			err = file.Truncate(newSize)
			if err != nil {
				file.Close()
				t.Fatalf("Failed to truncate file: %v", err)
			}
		}

		file.Close()
	}

	// Phase 3: Try to recover (should handle gracefully)
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		// Recovery should handle the truncated WAL gracefully
		// It may fail or succeed depending on where truncation occurred
		err = rm.Recover()
		// We don't assert on error here because the behavior depends on
		// where exactly the file was truncated

		_ = err // Acknowledge we're intentionally not checking
	}
}

// TestCrashRecovery_SequentialCrashes tests multiple sequential crashes
func TestCrashRecovery_SequentialCrashes(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Crash 1: Uncommitted transaction
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(1), []byte("v1"), []byte("v2"))
		// Crash!

		testWAL.Close()
	}

	// Recovery 1
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}

		rm := NewRecoveryManager(testWAL, walPath, nil)
		err = rm.Recover()
		if err != nil {
			t.Fatalf("First recovery failed: %v", err)
		}

		testWAL.Close()
	}

	// Crash 2: Another uncommitted transaction after recovery
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}

		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(2), []byte("v3"), []byte("v4"))
		// Crash again!

		testWAL.Close()
	}

	// Recovery 2
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)
		err = rm.Recover()
		if err != nil {
			t.Fatalf("Second recovery failed: %v", err)
		}

		// Should handle both crashed transactions correctly
		stats := rm.GetStats()
		if stats.TransactionsRecovered < 1 {
			t.Error("Expected at least 1 transaction to be recovered")
		}
	}
}

// TestCrashRecovery_MixedPageAccess tests recovery with multiple operations on same page
func TestCrashRecovery_MixedPageAccess(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "crash_test.wal")

	// Phase 1: Multiple transactions accessing the same page
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		pageID := newMockPageID(100)

		tid1 := primitives.NewTransactionID()
		testWAL.LogBegin(tid1)
		testWAL.LogUpdate(tid1, pageID, []byte("v1"), []byte("v2"))
		testWAL.LogCommit(tid1)

		tid2 := primitives.NewTransactionID()
		testWAL.LogBegin(tid2)
		testWAL.LogUpdate(tid2, pageID, []byte("v2"), []byte("v3"))
		// No commit - crash!

		testWAL.Close()
	}

	// Phase 2: Recovery should handle multiple updates to same page
	{
		testWAL, err := wal.NewWAL(walPath, 4096)
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer testWAL.Close()

		rm := NewRecoveryManager(testWAL, walPath, nil)

		err = rm.Recover()
		if err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		// Should undo tid2's update
		stats := rm.GetStats()
		if stats.TransactionsUndone != 1 {
			t.Errorf("Expected 1 transaction undone, got %d", stats.TransactionsUndone)
		}

		// Page should be in dirty page table
		dirtyPages := rm.GetDirtyPageTable()
		if _, exists := dirtyPages[newMockPageID(100)]; !exists {
			t.Error("Page 100 should be in dirty page table")
		}
	}
}
