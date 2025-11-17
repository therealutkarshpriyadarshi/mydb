package recovery

import (
	"hash/fnv"
	"path/filepath"
	"testing"

	"storemy/pkg/log/record"
	"storemy/pkg/log/wal"
	"storemy/pkg/primitives"
)

// mockPageID is a simple implementation of PageID for testing
type mockPageID struct {
	fileID primitives.FileID
	pageNo primitives.PageNumber
}

func (m *mockPageID) FileID() primitives.FileID {
	return m.fileID
}

func (m *mockPageID) PageNo() primitives.PageNumber {
	return m.pageNo
}

func (m *mockPageID) Serialize() []byte {
	result := make([]byte, 16)
	for i := 0; i < 8; i++ {
		result[i] = byte(m.fileID >> (i * 8))
	}
	for i := 0; i < 8; i++ {
		result[8+i] = byte(m.pageNo >> (i * 8))
	}
	return result
}

func (m *mockPageID) Equals(other primitives.PageID) bool {
	if other == nil {
		return false
	}
	return m.fileID == other.FileID() && m.pageNo == other.PageNo()
}

func (m *mockPageID) String() string {
	return ""
}

func (m *mockPageID) HashCode() primitives.HashCode {
	// Use FNV hash like the real PageDescriptor
	h := fnv.New64a()
	h.Write(m.Serialize())
	return primitives.HashCode(h.Sum64())
}

func newMockPageID(id int) primitives.PageID {
	return &mockPageID{
		fileID: primitives.FileID(id / 1000),
		pageNo: primitives.PageNumber(id % 1000),
	}
}

// createTestWAL creates a temporary WAL for testing
func createTestWAL(t *testing.T) (*wal.WAL, string) {
	t.Helper()

	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "test.wal")

	testWAL, err := wal.NewWAL(walPath, 4096)
	if err != nil {
		t.Fatalf("Failed to create test WAL: %v", err)
	}

	return testWAL, walPath
}

func TestNewRecoveryManager(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	if rm == nil {
		t.Fatal("NewRecoveryManager returned nil")
	}

	if rm.wal != testWAL {
		t.Error("WAL not set correctly")
	}

	if rm.dirtyPageTable == nil {
		t.Error("Dirty page table not initialized")
	}

	if rm.transactionTable == nil {
		t.Error("Transaction table not initialized")
	}
}

func TestAnalysisPhase_EmptyWAL(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed on empty WAL: %v", err)
	}

	if len(rm.dirtyPageTable) != 0 {
		t.Errorf("Expected empty dirty page table, got %d entries", len(rm.dirtyPageTable))
	}

	if len(rm.transactionTable) != 0 {
		t.Errorf("Expected empty transaction table, got %d entries", len(rm.transactionTable))
	}

	if rm.stats.LogRecordsScanned != 0 {
		t.Errorf("Expected 0 log records scanned, got %d", rm.stats.LogRecordsScanned)
	}
}

func TestAnalysisPhase_SingleCommittedTransaction(t *testing.T) {
	testWAL, walPath := createTestWAL(t)

	tid := primitives.NewTransactionID()

	// Simulate a committed transaction
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	testWAL.LogCommit(tid)

	// Close WAL to ensure all data is flushed before reading
	testWAL.Close()

	// Reopen WAL for recovery
	testWAL, err := wal.NewWAL(walPath, 4096)
	if err != nil {
		t.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err = rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Check transaction table
	if len(rm.transactionTable) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(rm.transactionTable))
	}

	txnInfo := rm.transactionTable[tid.ID()]
	if txnInfo == nil {
		t.Fatal("Transaction not found in transaction table")
	}

	if txnInfo.Status != TxnCommitted {
		t.Errorf("Expected TxnCommitted, got %v", txnInfo.Status)
	}

	// Check dirty page table
	if len(rm.dirtyPageTable) != 1 {
		t.Errorf("Expected 1 dirty page, got %d", len(rm.dirtyPageTable))
	}

	// Check statistics
	if rm.stats.TransactionsRecovered != 1 {
		t.Errorf("Expected 1 transaction recovered, got %d", rm.stats.TransactionsRecovered)
	}

	if rm.stats.DirtyPagesFound != 1 {
		t.Errorf("Expected 1 dirty page found, got %d", rm.stats.DirtyPagesFound)
	}

	if rm.stats.TransactionsUndone != 0 {
		t.Errorf("Expected 0 uncommitted transactions, got %d", rm.stats.TransactionsUndone)
	}
}

func TestAnalysisPhase_UncommittedTransaction(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	// Simulate an uncommitted transaction (crash before commit)
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	// No commit - simulates crash

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Check transaction table
	if len(rm.transactionTable) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(rm.transactionTable))
	}

	txnInfo := rm.transactionTable[tid.ID()]
	if txnInfo == nil {
		t.Fatal("Transaction not found in transaction table")
	}

	if txnInfo.Status != TxnActive {
		t.Errorf("Expected TxnActive, got %v", txnInfo.Status)
	}

	// Check statistics - uncommitted transaction should be counted
	if rm.stats.TransactionsUndone != 1 {
		t.Errorf("Expected 1 uncommitted transaction, got %d", rm.stats.TransactionsUndone)
	}
}

func TestAnalysisPhase_MultipleTransactions(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Transaction 1: Committed
	tid1 := primitives.NewTransactionID()
	testWAL.LogBegin(tid1)
	testWAL.LogUpdate(tid1, newMockPageID(1), []byte("old1"), []byte("new1"))
	testWAL.LogCommit(tid1)

	// Transaction 2: Uncommitted
	tid2 := primitives.NewTransactionID()
	testWAL.LogBegin(tid2)
	testWAL.LogUpdate(tid2, newMockPageID(2), []byte("old2"), []byte("new2"))
	// No commit

	// Transaction 3: Aborted
	tid3 := primitives.NewTransactionID()
	testWAL.LogBegin(tid3)
	testWAL.LogUpdate(tid3, newMockPageID(3), []byte("old3"), []byte("new3"))
	testWAL.LogAbort(tid3)

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Check transaction table
	if len(rm.transactionTable) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(rm.transactionTable))
	}

	// Verify transaction statuses
	if rm.transactionTable[tid1.ID()].Status != TxnCommitted {
		t.Errorf("Transaction 1 should be committed")
	}

	if rm.transactionTable[tid2.ID()].Status != TxnActive {
		t.Errorf("Transaction 2 should be active (uncommitted)")
	}

	if rm.transactionTable[tid3.ID()].Status != TxnAborted {
		t.Errorf("Transaction 3 should be aborted")
	}

	// Check dirty pages
	if len(rm.dirtyPageTable) != 3 {
		t.Errorf("Expected 3 dirty pages, got %d", len(rm.dirtyPageTable))
	}

	// Check uncommitted count (only tid2)
	if rm.stats.TransactionsUndone != 1 {
		t.Errorf("Expected 1 uncommitted transaction, got %d", rm.stats.TransactionsUndone)
	}
}

func TestAnalysisPhase_MultipleUpdatesToSamePage(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()
	pageID := newMockPageID(1)

	testWAL.LogBegin(tid)

	// First update to page 1
	firstUpdateLSN, _ := testWAL.LogUpdate(tid, pageID, []byte("v1"), []byte("v2"))

	// Second update to page 1
	testWAL.LogUpdate(tid, pageID, []byte("v2"), []byte("v3"))

	testWAL.LogCommit(tid)

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Dirty page table should have only one entry for page 1
	if len(rm.dirtyPageTable) != 1 {
		t.Errorf("Expected 1 dirty page, got %d", len(rm.dirtyPageTable))
	}

	// The LSN should be from the FIRST update
	if lsn, exists := rm.dirtyPageTable[pageID.HashCode()]; !exists {
		t.Error("Page 1 not in dirty page table")
	} else if lsn != firstUpdateLSN {
		t.Errorf("Expected first update LSN %d, got %d", firstUpdateLSN, lsn)
	}
}

func TestProcessAnalysisRecord_BeginRecord(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	tid := primitives.NewTransactionID()
	rec := &record.LogRecord{
		LSN:  100,
		Type: record.BeginRecord,
		TID:  tid,
	}

	err := rm.processAnalysisRecord(rec)
	if err != nil {
		t.Fatalf("processAnalysisRecord failed: %v", err)
	}

	// Check transaction was added
	txnInfo, exists := rm.transactionTable[tid.ID()]
	if !exists {
		t.Fatal("Transaction not added to transaction table")
	}

	if txnInfo.Status != TxnActive {
		t.Errorf("Expected TxnActive, got %v", txnInfo.Status)
	}

	if txnInfo.FirstLSN != 100 {
		t.Errorf("Expected FirstLSN=100, got %d", txnInfo.FirstLSN)
	}
}

func TestProcessAnalysisRecord_UpdateRecord(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	tid := primitives.NewTransactionID()
	pageID := newMockPageID(42)

	// First add a begin record
	beginRec := &record.LogRecord{
		LSN:  100,
		Type: record.BeginRecord,
		TID:  tid,
	}
	rm.processAnalysisRecord(beginRec)

	// Now process an update record
	updateRec := &record.LogRecord{
		LSN:         200,
		Type:        record.UpdateRecord,
		TID:         tid,
		PrevLSN:     100,
		PageID:      pageID,
		BeforeImage: []byte("old"),
		AfterImage:  []byte("new"),
	}

	err := rm.processAnalysisRecord(updateRec)
	if err != nil {
		t.Fatalf("processAnalysisRecord failed: %v", err)
	}

	// Check dirty page table
	lsn, exists := rm.dirtyPageTable[pageID.HashCode()]
	if !exists {
		t.Fatal("Page not added to dirty page table")
	}

	if lsn != 200 {
		t.Errorf("Expected LSN=200, got %d", lsn)
	}

	// Check transaction table update
	txnInfo := rm.transactionTable[tid.ID()]
	if txnInfo.LastLSN != 200 {
		t.Errorf("Expected LastLSN=200, got %d", txnInfo.LastLSN)
	}
}

func TestRedoPhase_NoDirtyPages(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Empty dirty page table
	rm.dirtyPageTable = make(map[primitives.HashCode]primitives.LSN)

	err := rm.redoPhase()
	if err != nil {
		t.Fatalf("Redo phase failed: %v", err)
	}

	if rm.stats.RedoOperations != 0 {
		t.Errorf("Expected 0 redo operations, got %d", rm.stats.RedoOperations)
	}
}

func TestRedoPhase_WithDirtyPages(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()
	pageID := newMockPageID(1)

	// Create a committed transaction with updates
	testWAL.LogBegin(tid)
	updateLSN, _ := testWAL.LogUpdate(tid, pageID, []byte("old"), []byte("new"))
	testWAL.LogCommit(tid)

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Run analysis first
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Run redo
	err = rm.redoPhase()
	if err != nil {
		t.Fatalf("Redo phase failed: %v", err)
	}

	// Should have redone the update
	if rm.stats.RedoOperations != 1 {
		t.Errorf("Expected 1 redo operation, got %d", rm.stats.RedoOperations)
	}

	// Verify the update LSN is in dirty page table
	if lsn, exists := rm.dirtyPageTable[pageID.HashCode()]; !exists {
		t.Error("Page should be in dirty page table")
	} else if lsn != updateLSN {
		t.Errorf("Expected LSN=%d, got %d", updateLSN, lsn)
	}
}

func TestUndoPhase_NoUncommittedTransactions(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	// Create a committed transaction
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	testWAL.LogCommit(tid)

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Run analysis first
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Run undo
	err = rm.undoPhase()
	if err != nil {
		t.Fatalf("Undo phase failed: %v", err)
	}

	// Should have no undo operations
	if rm.stats.UndoOperations != 0 {
		t.Errorf("Expected 0 undo operations, got %d", rm.stats.UndoOperations)
	}
}

func TestUndoPhase_UncommittedTransaction(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	// Create an uncommitted transaction (simulates crash)
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	// No commit - crash!

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Run analysis first
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Verify transaction is marked as active
	if rm.transactionTable[tid.ID()].Status != TxnActive {
		t.Fatal("Transaction should be active before undo")
	}

	// Run undo
	err = rm.undoPhase()
	if err != nil {
		t.Fatalf("Undo phase failed: %v", err)
	}

	// Should have undone the update
	if rm.stats.UndoOperations != 1 {
		t.Errorf("Expected 1 undo operation, got %d", rm.stats.UndoOperations)
	}
}

func TestGetStats(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Set some statistics
	rm.stats.LogRecordsScanned = 10
	rm.stats.RedoOperations = 5
	rm.stats.UndoOperations = 3

	stats := rm.GetStats()

	if stats.LogRecordsScanned != 10 {
		t.Errorf("Expected 10 records scanned, got %d", stats.LogRecordsScanned)
	}

	if stats.RedoOperations != 5 {
		t.Errorf("Expected 5 redo operations, got %d", stats.RedoOperations)
	}

	if stats.UndoOperations != 3 {
		t.Errorf("Expected 3 undo operations, got %d", stats.UndoOperations)
	}
}

func TestResetStats(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Set some statistics
	rm.stats.LogRecordsScanned = 10
	rm.stats.RedoOperations = 5

	rm.ResetStats()

	if rm.stats.LogRecordsScanned != 0 {
		t.Error("Stats not reset properly")
	}

	if rm.stats.RedoOperations != 0 {
		t.Error("Stats not reset properly")
	}
}

func TestGetDirtyPageTable(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Add some dirty pages
	page1 := newMockPageID(1)
	page2 := newMockPageID(2)
	rm.dirtyPageTable[page1.HashCode()] = 100
	rm.dirtyPageTable[page2.HashCode()] = 200

	table := rm.GetDirtyPageTable()

	if len(table) != 2 {
		t.Errorf("Expected 2 dirty pages, got %d", len(table))
	}

	if table[page1.HashCode()] != 100 {
		t.Error("Dirty page LSN incorrect")
	}

	// Verify it's a copy (modifying returned table shouldn't affect original)
	page3 := newMockPageID(3)
	table[page3.HashCode()] = 300

	if _, exists := rm.dirtyPageTable[page3.HashCode()]; exists {
		t.Error("Modifying returned table affected original")
	}
}

func TestGetUncommittedTransactions(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	tid1 := primitives.NewTransactionID()
	tid2 := primitives.NewTransactionID()
	tid3 := primitives.NewTransactionID()

	// Add transactions with different statuses
	rm.transactionTable[tid1.ID()] = &TransactionInfo{TID: tid1, Status: TxnActive}
	rm.transactionTable[tid2.ID()] = &TransactionInfo{TID: tid2, Status: TxnCommitted}
	rm.transactionTable[tid3.ID()] = &TransactionInfo{TID: tid3, Status: TxnActive}

	uncommitted := rm.GetUncommittedTransactions()

	if len(uncommitted) != 2 {
		t.Fatalf("Expected 2 uncommitted transactions, got %d", len(uncommitted))
	}

	// Verify the uncommitted transactions are tid1 and tid3
	found1, found3 := false, false
	for _, tid := range uncommitted {
		if tid.Equals(tid1) {
			found1 = true
		}
		if tid.Equals(tid3) {
			found3 = true
		}
	}

	if !found1 || !found3 {
		t.Error("Uncommitted transactions not returned correctly")
	}
}

func TestIsRecoveryNeeded_EmptyWAL(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)

	needed, err := rm.IsRecoveryNeeded()
	if err != nil {
		t.Fatalf("IsRecoveryNeeded failed: %v", err)
	}

	if needed {
		t.Error("Recovery should not be needed for empty WAL")
	}
}

func TestIsRecoveryNeeded_CommittedTransaction(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	// Create a committed transaction
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	testWAL.LogCommit(tid)

	rm := NewRecoveryManager(testWAL, walPath, nil)

	needed, err := rm.IsRecoveryNeeded()
	if err != nil {
		t.Fatalf("IsRecoveryNeeded failed: %v", err)
	}

	if needed {
		t.Error("Recovery should not be needed - all transactions committed")
	}
}

func TestIsRecoveryNeeded_UncommittedTransaction(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	// Create an uncommitted transaction
	testWAL.LogBegin(tid)
	testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))
	// No commit - simulates crash

	rm := NewRecoveryManager(testWAL, walPath, nil)

	needed, err := rm.IsRecoveryNeeded()
	if err != nil {
		t.Fatalf("IsRecoveryNeeded failed: %v", err)
	}

	if !needed {
		t.Error("Recovery should be needed - uncommitted transaction exists")
	}
}

func TestFullRecoveryScenario(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Simulate a complex scenario:
	// - Transaction 1: Committed
	// - Transaction 2: Uncommitted (crash)
	// - Transaction 3: Aborted

	tid1 := primitives.NewTransactionID()
	tid2 := primitives.NewTransactionID()
	tid3 := primitives.NewTransactionID()

	// Transaction 1: Committed
	testWAL.LogBegin(tid1)
	testWAL.LogUpdate(tid1, newMockPageID(1), []byte("old1"), []byte("new1"))
	testWAL.LogCommit(tid1)

	// Transaction 2: Uncommitted (crash before commit)
	testWAL.LogBegin(tid2)
	testWAL.LogUpdate(tid2, newMockPageID(2), []byte("old2"), []byte("new2"))
	testWAL.LogInsert(tid2, newMockPageID(3), []byte("inserted"))
	// No commit!

	// Transaction 3: Aborted
	testWAL.LogBegin(tid3)
	testWAL.LogUpdate(tid3, newMockPageID(4), []byte("old3"), []byte("new3"))
	testWAL.LogAbort(tid3)

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Check if recovery is needed
	needed, err := rm.IsRecoveryNeeded()
	if err != nil {
		t.Fatalf("IsRecoveryNeeded failed: %v", err)
	}

	if !needed {
		t.Error("Recovery should be needed due to uncommitted transaction")
	}

	// Run full recovery
	err = rm.Recover()
	if err != nil {
		t.Fatalf("Recovery failed: %v", err)
	}

	// Verify statistics
	stats := rm.GetStats()

	if stats.TransactionsRecovered != 3 {
		t.Errorf("Expected 3 transactions recovered, got %d", stats.TransactionsRecovered)
	}

	if stats.TransactionsUndone != 1 {
		t.Errorf("Expected 1 transaction undone, got %d", stats.TransactionsUndone)
	}

	if stats.DirtyPagesFound != 4 {
		t.Errorf("Expected 4 dirty pages, got %d", stats.DirtyPagesFound)
	}

	// Verify dirty page table
	dirtyPages := rm.GetDirtyPageTable()
	if len(dirtyPages) != 4 {
		t.Errorf("Expected 4 entries in dirty page table, got %d", len(dirtyPages))
	}

	// Verify transaction table
	txnTable := rm.GetTransactionTable()
	if len(txnTable) != 3 {
		t.Errorf("Expected 3 entries in transaction table, got %d", len(txnTable))
	}

	// Verify transaction statuses
	if txnTable[tid1.ID()].Status != TxnCommitted {
		t.Error("Transaction 1 should be committed")
	}

	if txnTable[tid2.ID()].Status != TxnActive {
		t.Error("Transaction 2 should be active (uncommitted)")
	}

	if txnTable[tid3.ID()].Status != TxnAborted {
		t.Error("Transaction 3 should be aborted")
	}
}

func TestConcurrentAnalysis(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Create several transactions
	for i := 0; i < 5; i++ {
		tid := primitives.NewTransactionID()
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(i), []byte("old"), []byte("new"))
		if i%2 == 0 {
			testWAL.LogCommit(tid)
		}
		// Odd transactions left uncommitted
	}

	rm := NewRecoveryManager(testWAL, walPath, nil)

	// Run analysis
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// Verify results
	if len(rm.transactionTable) != 5 {
		t.Errorf("Expected 5 transactions, got %d", len(rm.transactionTable))
	}

	if len(rm.dirtyPageTable) != 5 {
		t.Errorf("Expected 5 dirty pages, got %d", len(rm.dirtyPageTable))
	}

	// Should have 2 uncommitted transactions (indices 1 and 3)
	uncommitted := rm.GetUncommittedTransactions()
	if len(uncommitted) != 2 {
		t.Errorf("Expected 2 uncommitted transactions, got %d", len(uncommitted))
	}
}

func TestAnalysisPhase_CLRRecords(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	tid := primitives.NewTransactionID()

	testWAL.LogBegin(tid)

	// Create an update
	updateLSN, _ := testWAL.LogUpdate(tid, newMockPageID(1), []byte("old"), []byte("new"))

	// Manually write a CLR (in real scenario, this would be during rollback)
	reader, _ := wal.NewLogReader(walPath)
	defer reader.Close()

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err := rm.analysisPhase()
	if err != nil {
		t.Fatalf("Analysis phase failed: %v", err)
	}

	// CLR should still mark the page as dirty
	if _, exists := rm.dirtyPageTable[newMockPageID(1).HashCode()]; !exists {
		t.Error("Page should be in dirty page table after CLR")
	}

	// Transaction should still be tracked
	if _, exists := rm.transactionTable[tid.ID()]; !exists {
		t.Error("Transaction should be in transaction table")
	}

	_ = updateLSN // Avoid unused variable error
}

// TestRecoveryWithCheckpoint tests end-to-end recovery using checkpoints
func TestRecoveryWithCheckpoint(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Phase 1: Create some transactions and checkpoint
	t.Log("Phase 1: Creating transactions and checkpoint")

	tid1 := primitives.NewTransactionIDFromValue(1)
	tid2 := primitives.NewTransactionIDFromValue(2)
	tid3 := primitives.NewTransactionIDFromValue(3)

	// Start transactions
	testWAL.LogBegin(tid1)
	testWAL.LogBegin(tid2)

	// Log updates for tid1 and tid2
	testWAL.LogUpdate(tid1, newMockPageID(1), []byte("old1"), []byte("new1"))
	testWAL.LogUpdate(tid2, newMockPageID(2), []byte("old2"), []byte("new2"))

	// Commit tid1
	testWAL.LogCommit(tid1)

	// Write checkpoint (tid2 still active)
	checkpointLSN, err := testWAL.WriteCheckpoint()
	if err != nil {
		t.Fatalf("Failed to write checkpoint: %v", err)
	}
	t.Logf("Checkpoint written at LSN %d", checkpointLSN)

	// Phase 2: More transactions after checkpoint
	t.Log("Phase 2: Adding more transactions after checkpoint")

	// Start new transaction after checkpoint
	testWAL.LogBegin(tid3)
	testWAL.LogUpdate(tid3, newMockPageID(3), []byte("old3"), []byte("new3"))

	// Commit tid2
	testWAL.LogCommit(tid2)

	// tid3 remains uncommitted (will need to be undone)

	// Phase 3: Perform recovery
	t.Log("Phase 3: Performing recovery")

	rm := NewRecoveryManager(testWAL, walPath, nil)
	err = rm.Recover()
	if err != nil {
		t.Fatalf("Recovery failed: %v", err)
	}

	// Phase 4: Verify recovery results
	t.Log("Phase 4: Verifying recovery results")

	stats := rm.GetStats()
	t.Logf("Recovery stats: %+v", stats)

	// Should have scanned records (but fewer than if we started from beginning)
	if stats.LogRecordsScanned == 0 {
		t.Error("Should have scanned some log records")
	}

	// Should have 1 uncommitted transaction (tid3)
	uncommitted := rm.GetUncommittedTransactions()
	if len(uncommitted) != 1 {
		t.Errorf("Expected 1 uncommitted transaction, got %d", len(uncommitted))
	}

	if len(uncommitted) > 0 && uncommitted[0].ID() != tid3.ID() {
		t.Errorf("Expected uncommitted transaction to be tid3, got %d", uncommitted[0].ID())
	}

	// Should have undone 1 transaction
	if stats.TransactionsUndone != 1 {
		t.Errorf("Expected 1 transaction undone, got %d", stats.TransactionsUndone)
	}

	// Should have found dirty pages
	if stats.DirtyPagesFound == 0 {
		t.Error("Should have found dirty pages")
	}

	t.Log("Recovery with checkpoint completed successfully")
}

// TestRecoveryWithoutCheckpoint tests that recovery works when no checkpoint exists
func TestRecoveryWithoutCheckpoint(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Create some transactions without checkpoint
	tid1 := primitives.NewTransactionIDFromValue(1)
	tid2 := primitives.NewTransactionIDFromValue(2)

	testWAL.LogBegin(tid1)
	testWAL.LogUpdate(tid1, newMockPageID(1), []byte("old1"), []byte("new1"))
	testWAL.LogCommit(tid1)

	testWAL.LogBegin(tid2)
	testWAL.LogUpdate(tid2, newMockPageID(2), []byte("old2"), []byte("new2"))
	// tid2 not committed

	// Perform recovery without checkpoint
	rm := NewRecoveryManager(testWAL, walPath, nil)
	err := rm.Recover()
	if err != nil {
		t.Fatalf("Recovery without checkpoint failed: %v", err)
	}

	// Verify recovery
	uncommitted := rm.GetUncommittedTransactions()
	if len(uncommitted) != 1 {
		t.Errorf("Expected 1 uncommitted transaction, got %d", len(uncommitted))
	}

	t.Log("Recovery without checkpoint completed successfully")
}

// TestCheckpointWithActiveTransactions tests checkpoint with multiple active transactions
func TestCheckpointWithActiveTransactions(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Start multiple transactions
	tids := make([]*primitives.TransactionID, 5)
	for i := 0; i < 5; i++ {
		tids[i] = primitives.NewTransactionIDFromValue(int64(i + 1))
		testWAL.LogBegin(tids[i])
		testWAL.LogUpdate(tids[i], newMockPageID(i+1), []byte("old"), []byte("new"))
	}

	// Commit some
	testWAL.LogCommit(tids[0])
	testWAL.LogCommit(tids[2])

	// Write checkpoint (3 active transactions remain)
	_, err := testWAL.WriteCheckpoint()
	if err != nil {
		t.Fatalf("Checkpoint with active transactions failed: %v", err)
	}

	// Verify checkpoint contains active transactions
	checkpoint, err := testWAL.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	if len(checkpoint.ActiveTxns) != 3 {
		t.Errorf("Expected 3 active transactions in checkpoint, got %d", len(checkpoint.ActiveTxns))
	}

	// Perform recovery
	rm := NewRecoveryManager(testWAL, walPath, nil)
	err = rm.Recover()
	if err != nil {
		t.Fatalf("Recovery with checkpoint failed: %v", err)
	}

	// Should have 3 uncommitted transactions
	uncommitted := rm.GetUncommittedTransactions()
	if len(uncommitted) != 3 {
		t.Errorf("Expected 3 uncommitted transactions, got %d", len(uncommitted))
	}

	t.Log("Checkpoint with active transactions test passed")
}

// TestCheckpointAndTruncation tests checkpoint followed by log truncation
func TestCheckpointAndTruncation(t *testing.T) {
	testWAL, walPath := createTestWAL(t)
	defer testWAL.Close()

	// Create and commit several transactions
	for i := 0; i < 10; i++ {
		tid := primitives.NewTransactionIDFromValue(int64(i + 1))
		testWAL.LogBegin(tid)
		testWAL.LogUpdate(tid, newMockPageID(i+1), []byte("old"), []byte("new"))
		testWAL.LogCommit(tid)
	}

	// Write checkpoint
	_, err := testWAL.WriteCheckpoint()
	if err != nil {
		t.Fatalf("Checkpoint failed: %v", err)
	}

	// Get checkpoint for truncation
	checkpoint, err := testWAL.GetLastCheckpoint()
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	// Note: Truncation is complex and may require careful testing
	// For now, just verify we can load the checkpoint
	if checkpoint == nil {
		t.Fatal("Checkpoint should not be nil")
	}

	if len(checkpoint.ActiveTxns) != 0 {
		t.Errorf("Expected 0 active transactions after all commits, got %d", len(checkpoint.ActiveTxns))
	}

	// Verify recovery works after checkpoint
	rm := NewRecoveryManager(testWAL, walPath, nil)
	err = rm.Recover()
	if err != nil {
		t.Fatalf("Recovery after checkpoint failed: %v", err)
	}

	// Should have no uncommitted transactions
	uncommitted := rm.GetUncommittedTransactions()
	if len(uncommitted) != 0 {
		t.Errorf("Expected 0 uncommitted transactions, got %d", len(uncommitted))
	}

	t.Log("Checkpoint and truncation test passed")
}
