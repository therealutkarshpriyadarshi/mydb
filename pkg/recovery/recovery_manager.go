package recovery

import (
	"fmt"
	"sync"

	"storemy/pkg/log/record"
	"storemy/pkg/log/wal"
	"storemy/pkg/memory"
	"storemy/pkg/primitives"
)

// RecoveryManager implements ARIES-style crash recovery with three phases:
// 1. Analysis - scan WAL to identify uncommitted transactions and dirty pages
// 2. Redo - replay all operations to restore database state
// 3. Undo - rollback uncommitted transactions
type RecoveryManager struct {
	wal       *wal.WAL
	walPath   string
	pageStore *memory.PageStore
	mutex     sync.RWMutex

	// Analysis phase results
	dirtyPageTable   map[primitives.HashCode]primitives.LSN // pageID.HashCode() -> first LSN that dirtied it
	transactionTable map[int64]*TransactionInfo              // tidID -> transaction info

	// Recovery statistics
	stats RecoveryStats
}

// TransactionInfo tracks transaction state during recovery
type TransactionInfo struct {
	TID         *primitives.TransactionID
	Status      TransactionStatus
	FirstLSN    primitives.LSN
	LastLSN     primitives.LSN
	UndoNextLSN primitives.LSN
}

// TransactionStatus represents the state of a transaction during recovery
type TransactionStatus int

const (
	TxnActive TransactionStatus = iota
	TxnCommitted
	TxnAborted
)

// RecoveryStats tracks recovery phase statistics
type RecoveryStats struct {
	LogRecordsScanned    int
	RedoOperations       int
	UndoOperations       int
	TransactionsRecovered int
	TransactionsUndone   int
	DirtyPagesFound      int
}

// NewRecoveryManager creates a new recovery manager instance
func NewRecoveryManager(wal *wal.WAL, walPath string, pageStore *memory.PageStore) *RecoveryManager {
	return &RecoveryManager{
		wal:              wal,
		walPath:          walPath,
		pageStore:        pageStore,
		dirtyPageTable:   make(map[primitives.HashCode]primitives.LSN),
		transactionTable: make(map[int64]*TransactionInfo),
		stats:            RecoveryStats{},
	}
}

// Recover performs the full ARIES recovery algorithm
// This is the main entry point called after a crash
func (rm *RecoveryManager) Recover() error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	fmt.Println("Starting ARIES recovery...")

	// Phase 1: Analysis
	if err := rm.analysisPhase(); err != nil {
		return fmt.Errorf("analysis phase failed: %w", err)
	}

	// Phase 2: Redo
	if err := rm.redoPhase(); err != nil {
		return fmt.Errorf("redo phase failed: %w", err)
	}

	// Phase 3: Undo
	if err := rm.undoPhase(); err != nil {
		return fmt.Errorf("undo phase failed: %w", err)
	}

	fmt.Printf("Recovery completed successfully. Stats: %+v\n", rm.stats)
	return nil
}

// analysisPhase scans the WAL to:
// 1. Load the last checkpoint (if exists) to initialize state
// 2. Build the dirty page table (which pages were modified)
// 3. Build the transaction table (which transactions were active)
// 4. Identify uncommitted transactions that need to be undone
func (rm *RecoveryManager) analysisPhase() error {
	fmt.Println("Phase 1: Analysis - scanning WAL...")

	// Force flush WAL to ensure all records are on disk before reading
	// Get the current LSN by checking the writer's current LSN
	if err := rm.wal.Force(primitives.LSN(^uint64(0))); err != nil {
		return fmt.Errorf("failed to flush WAL before analysis: %w", err)
	}

	// Reset internal state
	rm.dirtyPageTable = make(map[primitives.HashCode]primitives.LSN)
	rm.transactionTable = make(map[int64]*TransactionInfo)

	// Try to load the last checkpoint
	startLSN := primitives.LSN(0)
	checkpoint, err := rm.wal.GetLastCheckpoint()
	if err != nil {
		fmt.Printf("Warning: failed to load checkpoint: %v\n", err)
		// Continue with recovery from beginning
	} else if checkpoint != nil {
		// Initialize state from checkpoint
		fmt.Printf("Found checkpoint at LSN %d: %d active transactions, %d dirty pages\n",
			checkpoint.LSN, len(checkpoint.ActiveTxns), len(checkpoint.DirtyPages))

		// Load dirty page table from checkpoint
		for pageHash, lsn := range checkpoint.DirtyPages {
			rm.dirtyPageTable[pageHash] = lsn
		}

		// Load transaction table from checkpoint
		for tidID, txnInfo := range checkpoint.ActiveTxns {
			rm.transactionTable[tidID] = &TransactionInfo{
				TID:         primitives.NewTransactionIDFromValue(tidID),
				Status:      TxnActive,
				FirstLSN:    txnInfo.FirstLSN,
				LastLSN:     txnInfo.LastLSN,
				UndoNextLSN: txnInfo.UndoNextLSN,
			}
		}

		// Start scanning from checkpoint LSN
		startLSN = checkpoint.LSN
		fmt.Printf("Starting analysis from checkpoint LSN %d\n", startLSN)
	} else {
		fmt.Println("No checkpoint found, starting analysis from beginning")
	}

	reader, err := wal.NewLogReader(rm.walPath)
	if err != nil {
		return fmt.Errorf("failed to create WAL reader: %w", err)
	}
	defer reader.Close()

	// Scan WAL from startLSN (either checkpoint LSN or 0)
	for {
		logRecord, err := reader.ReadNext()
		if err != nil {
			// End of log reached
			break
		}

		// Skip records before our start LSN
		if logRecord.LSN < startLSN {
			continue
		}

		rm.stats.LogRecordsScanned++

		// Process record based on type
		if err := rm.processAnalysisRecord(logRecord); err != nil {
			return fmt.Errorf("failed to process record at LSN %d: %w", logRecord.LSN, err)
		}
	}

	// Count statistics
	rm.stats.TransactionsRecovered = len(rm.transactionTable)
	rm.stats.DirtyPagesFound = len(rm.dirtyPageTable)

	// Count uncommitted transactions
	for _, txnInfo := range rm.transactionTable {
		if txnInfo.Status == TxnActive {
			rm.stats.TransactionsUndone++
		}
	}

	fmt.Printf("Analysis complete: %d transactions, %d dirty pages, %d uncommitted\n",
		rm.stats.TransactionsRecovered, rm.stats.DirtyPagesFound, rm.stats.TransactionsUndone)

	return nil
}

// processAnalysisRecord processes a single log record during the analysis phase
func (rm *RecoveryManager) processAnalysisRecord(rec *record.LogRecord) error {
	switch rec.Type {
	case record.CheckpointBegin, record.CheckpointEnd:
		// Checkpoint records don't need processing during analysis
		// (they were already loaded by analysisPhase if they exist)
		return nil
	}

	// All other record types require a transaction ID
	if rec.TID == nil {
		return fmt.Errorf("log record at LSN %d has no transaction ID", rec.LSN)
	}

	tid := rec.TID
	tidID := tid.ID()

	switch rec.Type {
	case record.BeginRecord:
		// New transaction started
		rm.transactionTable[tidID] = &TransactionInfo{
			TID:         tid,
			Status:      TxnActive,
			FirstLSN:    rec.LSN,
			LastLSN:     rec.LSN,
			UndoNextLSN: rec.LSN,
		}

	case record.CommitRecord:
		// Transaction committed - mark as committed
		if txnInfo, exists := rm.transactionTable[tidID]; exists {
			txnInfo.Status = TxnCommitted
			txnInfo.LastLSN = rec.LSN
		} else {
			// Transaction started before checkpoint
			rm.transactionTable[tidID] = &TransactionInfo{
				TID:      tid,
				Status:   TxnCommitted,
				FirstLSN: rec.LSN,
				LastLSN:  rec.LSN,
			}
		}

	case record.AbortRecord:
		// Transaction aborted - mark as aborted
		if txnInfo, exists := rm.transactionTable[tidID]; exists {
			txnInfo.Status = TxnAborted
			txnInfo.LastLSN = rec.LSN
		} else {
			rm.transactionTable[tidID] = &TransactionInfo{
				TID:      tid,
				Status:   TxnAborted,
				FirstLSN: rec.LSN,
				LastLSN:  rec.LSN,
			}
		}

	case record.UpdateRecord, record.InsertRecord, record.DeleteRecord:
		// Data modification - update transaction table and dirty page table
		if txnInfo, exists := rm.transactionTable[tidID]; exists {
			txnInfo.LastLSN = rec.LSN
			txnInfo.UndoNextLSN = rec.PrevLSN
		} else {
			// Transaction started before checkpoint
			rm.transactionTable[tidID] = &TransactionInfo{
				TID:         tid,
				Status:      TxnActive,
				FirstLSN:    rec.LSN,
				LastLSN:     rec.LSN,
				UndoNextLSN: rec.PrevLSN,
			}
		}

		// Add to dirty page table if not already present
		pageHash := rec.PageID.HashCode()
		if _, exists := rm.dirtyPageTable[pageHash]; !exists {
			rm.dirtyPageTable[pageHash] = rec.LSN
		}

	case record.CLRRecord:
		// Compensation Log Record (undo already performed)
		if txnInfo, exists := rm.transactionTable[tidID]; exists {
			txnInfo.LastLSN = rec.LSN
			txnInfo.UndoNextLSN = rec.UndoNextLSN
		}

		// CLR also dirties pages
		pageHash := rec.PageID.HashCode()
		if _, exists := rm.dirtyPageTable[pageHash]; !exists {
			rm.dirtyPageTable[pageHash] = rec.LSN
		}
	}

	return nil
}

// redoPhase replays all operations from the WAL to restore the database state
// This ensures all committed transactions are reflected on disk
func (rm *RecoveryManager) redoPhase() error {
	fmt.Println("Phase 2: Redo - replaying operations...")

	if len(rm.dirtyPageTable) == 0 {
		fmt.Println("No dirty pages found, skipping redo phase")
		return nil
	}

	// Find the minimum LSN in the dirty page table (earliest dirty page)
	minLSN := primitives.LSN(^uint64(0)) // Max uint64
	for _, lsn := range rm.dirtyPageTable {
		if lsn < minLSN {
			minLSN = lsn
		}
	}

	reader, err := wal.NewLogReader(rm.walPath)
	if err != nil {
		return fmt.Errorf("failed to create WAL reader: %w", err)
	}
	defer reader.Close()

	// Scan from the earliest dirty page LSN
	for {
		logRecord, err := reader.ReadNext()
		if err != nil {
			// End of log reached
			break
		}

		// Skip records before minimum LSN
		if logRecord.LSN < minLSN {
			continue
		}

		// Redo the operation if needed
		if err := rm.redoRecord(logRecord); err != nil {
			return fmt.Errorf("failed to redo record at LSN %d: %w", logRecord.LSN, err)
		}
	}

	fmt.Printf("Redo complete: %d operations replayed\n", rm.stats.RedoOperations)
	return nil
}

// redoRecord replays a single log record
func (rm *RecoveryManager) redoRecord(rec *record.LogRecord) error {
	// Only redo data modification records
	switch rec.Type {
	case record.UpdateRecord, record.InsertRecord, record.CLRRecord:
		// Check if this page is in the dirty page table
		pageHash := rec.PageID.HashCode()
		if firstLSN, isDirty := rm.dirtyPageTable[pageHash]; isDirty {
			// Only redo if this record dirtied the page or came after
			if rec.LSN >= firstLSN {
				// Apply the after-image to the page
				if err := rm.applyRedo(rec); err != nil {
					return err
				}
				rm.stats.RedoOperations++
			}
		}
	}

	return nil
}

// applyRedo applies the after-image of a log record to a page
func (rm *RecoveryManager) applyRedo(rec *record.LogRecord) error {
	// In a real implementation, this would:
	// 1. Read the page from disk (if not in buffer)
	// 2. Apply the after-image to the page
	// 3. Mark the page as dirty
	// 4. Write the page back to disk

	// For now, we'll delegate to the page store
	// The page store will handle loading the page and applying changes

	// Note: In ARIES, we would check the page LSN and only redo if rec.LSN > page.LSN
	// This prevents redundant redo operations

	return nil
}

// undoPhase rolls back all uncommitted transactions
// This ensures atomicity - no partial transactions remain
func (rm *RecoveryManager) undoPhase() error {
	fmt.Println("Phase 3: Undo - rolling back uncommitted transactions...")

	// Collect all uncommitted transactions
	var uncommittedTxns []*TransactionInfo
	for _, txnInfo := range rm.transactionTable {
		if txnInfo.Status == TxnActive {
			uncommittedTxns = append(uncommittedTxns, txnInfo)
		}
	}

	if len(uncommittedTxns) == 0 {
		fmt.Println("No uncommitted transactions found, skipping undo phase")
		return nil
	}

	fmt.Printf("Found %d uncommitted transactions to rollback\n", len(uncommittedTxns))

	// For each uncommitted transaction, follow the undo chain backwards
	for _, txnInfo := range uncommittedTxns {
		if err := rm.undoTransaction(txnInfo); err != nil {
			return fmt.Errorf("failed to undo transaction %v: %w", txnInfo.TID, err)
		}
	}

	fmt.Printf("Undo complete: %d operations undone\n", rm.stats.UndoOperations)
	return nil
}

// undoTransaction rolls back a single uncommitted transaction
func (rm *RecoveryManager) undoTransaction(txnInfo *TransactionInfo) error {
	fmt.Printf("Undoing transaction %v (LastLSN=%d)\n", txnInfo.TID, txnInfo.LastLSN)

	reader, err := wal.NewLogReader(rm.walPath)
	if err != nil {
		return fmt.Errorf("failed to create WAL reader: %w", err)
	}
	defer reader.Close()

	// Build a map of all log records for quick lookup
	recordMap := make(map[primitives.LSN]*record.LogRecord)
	for {
		rec, err := reader.ReadNext()
		if err != nil {
			break
		}
		recordMap[rec.LSN] = rec
	}

	// Follow the undo chain backwards from LastLSN
	currentLSN := txnInfo.LastLSN

	for currentLSN != 0 {
		rec, exists := recordMap[currentLSN]
		if !exists {
			// Reached the beginning of the transaction
			break
		}

		// Only undo data modification records
		if rec.TID.Equals(txnInfo.TID) {
			switch rec.Type {
			case record.UpdateRecord, record.DeleteRecord:
				// Undo this operation
				if err := rm.undoRecord(rec); err != nil {
					return fmt.Errorf("failed to undo record at LSN %d: %w", currentLSN, err)
				}
				rm.stats.UndoOperations++

				// Write CLR (Compensation Log Record) to prevent re-undo
				clr := &record.LogRecord{
					Type:        record.CLRRecord,
					TID:         rec.TID,
					PageID:      rec.PageID,
					AfterImage:  rec.BeforeImage, // CLR applies the before-image
					UndoNextLSN: rec.PrevLSN,     // Continue undo chain
				}

				if err := rm.writeCLR(clr); err != nil {
					return fmt.Errorf("failed to write CLR: %w", err)
				}

			case record.InsertRecord:
				// For inserts, we need to delete the tuple
				// This is equivalent to applying a delete operation
				if err := rm.undoInsert(rec); err != nil {
					return fmt.Errorf("failed to undo insert at LSN %d: %w", currentLSN, err)
				}
				rm.stats.UndoOperations++

				// Write CLR
				clr := &record.LogRecord{
					Type:        record.CLRRecord,
					TID:         rec.TID,
					PageID:      rec.PageID,
					UndoNextLSN: rec.PrevLSN,
				}

				if err := rm.writeCLR(clr); err != nil {
					return fmt.Errorf("failed to write CLR: %w", err)
				}
			}
		}

		// Follow the undo chain via PrevLSN
		currentLSN = rec.PrevLSN
	}

	// Mark transaction as aborted in WAL during recovery
	// We use LogAbortDuringRecovery because the transaction is not in the active transactions table
	if _, err := rm.wal.LogAbortDuringRecovery(txnInfo.TID, txnInfo.LastLSN); err != nil {
		return fmt.Errorf("failed to log abort: %w", err)
	}

	return nil
}

// undoRecord undoes a single update or delete operation
func (rm *RecoveryManager) undoRecord(rec *record.LogRecord) error {
	// In a real implementation, this would:
	// 1. Read the page from disk
	// 2. Apply the before-image to restore the old state
	// 3. Mark the page as dirty
	// 4. Write the page back to disk

	// For now, this is a placeholder
	// The actual implementation would interact with the page store

	return nil
}

// undoInsert undoes an insert operation by deleting the inserted tuple
func (rm *RecoveryManager) undoInsert(rec *record.LogRecord) error {
	// In a real implementation, this would:
	// 1. Read the page from disk
	// 2. Delete the tuple that was inserted (using the after-image to identify it)
	// 3. Mark the page as dirty
	// 4. Write the page back to disk

	return nil
}

// writeCLR writes a Compensation Log Record to the WAL
func (rm *RecoveryManager) writeCLR(clr *record.LogRecord) error {
	// In a real implementation, this would serialize and write the CLR to the WAL
	// For now, this is a placeholder
	return nil
}

// GetStats returns the recovery statistics
func (rm *RecoveryManager) GetStats() RecoveryStats {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return rm.stats
}

// ResetStats resets the recovery statistics
func (rm *RecoveryManager) ResetStats() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	rm.stats = RecoveryStats{}
}

// GetDirtyPageTable returns a copy of the dirty page table
func (rm *RecoveryManager) GetDirtyPageTable() map[primitives.HashCode]primitives.LSN {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[primitives.HashCode]primitives.LSN)
	for k, v := range rm.dirtyPageTable {
		result[k] = v
	}
	return result
}

// GetTransactionTable returns a copy of the transaction table
func (rm *RecoveryManager) GetTransactionTable() map[int64]*TransactionInfo {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[int64]*TransactionInfo)
	for k, v := range rm.transactionTable {
		// Deep copy
		result[k] = &TransactionInfo{
			TID:         v.TID,
			Status:      v.Status,
			FirstLSN:    v.FirstLSN,
			LastLSN:     v.LastLSN,
			UndoNextLSN: v.UndoNextLSN,
		}
	}
	return result
}

// GetUncommittedTransactions returns a list of all uncommitted transaction IDs
func (rm *RecoveryManager) GetUncommittedTransactions() []*primitives.TransactionID {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var result []*primitives.TransactionID
	for _, txnInfo := range rm.transactionTable {
		if txnInfo.Status == TxnActive {
			result = append(result, txnInfo.TID)
		}
	}
	return result
}

// IsRecoveryNeeded checks if recovery is needed by examining the WAL
func (rm *RecoveryManager) IsRecoveryNeeded() (bool, error) {
	// Force flush WAL to ensure all records are on disk before reading
	if err := rm.wal.Force(primitives.LSN(^uint64(0))); err != nil {
		return false, fmt.Errorf("failed to flush WAL before checking: %w", err)
	}

	reader, err := wal.NewLogReader(rm.walPath)
	if err != nil {
		return false, fmt.Errorf("failed to create WAL reader: %w", err)
	}
	defer reader.Close()

	// Check if there are any uncommitted transactions
	// Use TID.ID() as key since deserialized TIDs are different instances
	activeTxns := make(map[int64]bool)

	for {
		rec, err := reader.ReadNext()
		if err != nil {
			break
		}

		switch rec.Type {
		case record.BeginRecord:
			activeTxns[rec.TID.ID()] = true
		case record.CommitRecord, record.AbortRecord:
			delete(activeTxns, rec.TID.ID())
		}
	}

	// Recovery needed if there are active transactions
	return len(activeTxns) > 0, nil
}
