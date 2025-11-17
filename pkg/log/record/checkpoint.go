package record

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"storemy/pkg/primitives"
	"time"
)

// CheckpointRecord represents a fuzzy checkpoint
// Contains snapshot of active transactions and dirty pages at checkpoint time
type CheckpointRecord struct {
	LSN        primitives.LSN // LSN of this checkpoint
	Timestamp  time.Time

	// Active transactions at checkpoint time
	// Maps transaction ID -> transaction log info
	ActiveTxns map[int64]*TransactionLogInfo

	// Dirty pages at checkpoint time
	// Maps page ID -> first LSN that dirtied the page
	DirtyPages map[primitives.HashCode]primitives.LSN
}

// NewCheckpointRecord creates a new checkpoint record
func NewCheckpointRecord(activeTxns map[*primitives.TransactionID]*TransactionLogInfo, dirtyPages map[primitives.PageID]primitives.LSN) *CheckpointRecord {
	// Convert activeTxns map to use int64 keys for serialization
	txnMap := make(map[int64]*TransactionLogInfo)
	for tid, info := range activeTxns {
		txnMap[tid.ID()] = info
	}

	// Convert dirtyPages to use HashCode keys
	pageMap := make(map[primitives.HashCode]primitives.LSN)
	for pageID, lsn := range dirtyPages {
		pageMap[pageID.HashCode()] = lsn
	}

	return &CheckpointRecord{
		Timestamp:  time.Now(),
		ActiveTxns: txnMap,
		DirtyPages: pageMap,
	}
}

// SerializeCheckpoint serializes a checkpoint record to bytes
//
// Binary format:
// [Size:4][LSN:8][Timestamp:8][NumTxns:4][TxnData...][NumPages:4][PageData...]
//
// TxnData format (repeated NumTxns times):
// [TID:8][FirstLSN:8][LastLSN:8][UndoNextLSN:8]
//
// PageData format (repeated NumPages times):
// [PageHash:8][FirstDirtyLSN:8]
func SerializeCheckpoint(cp *CheckpointRecord) ([]byte, error) {
	var buf bytes.Buffer

	// Write header
	writes := []any{
		uint64(cp.LSN),
		uint64(cp.Timestamp.Unix()),
	}

	for _, v := range writes {
		if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
			return nil, fmt.Errorf("failed to write checkpoint header: %w", err)
		}
	}

	// Write active transactions
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(cp.ActiveTxns))); err != nil {
		return nil, fmt.Errorf("failed to write transaction count: %w", err)
	}

	for tid, info := range cp.ActiveTxns {
		txnWrites := []any{
			uint64(tid),
			uint64(info.FirstLSN),
			uint64(info.LastLSN),
			uint64(info.UndoNextLSN),
		}

		for _, v := range txnWrites {
			if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
				return nil, fmt.Errorf("failed to write transaction data: %w", err)
			}
		}
	}

	// Write dirty pages
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(cp.DirtyPages))); err != nil {
		return nil, fmt.Errorf("failed to write page count: %w", err)
	}

	for pageHash, lsn := range cp.DirtyPages {
		pageWrites := []any{
			uint64(pageHash),
			uint64(lsn),
		}

		for _, v := range pageWrites {
			if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
				return nil, fmt.Errorf("failed to write page data: %w", err)
			}
		}
	}

	// Prepend size
	data := buf.Bytes()
	result := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(result, uint32(len(result)))
	copy(result[4:], data)

	return result, nil
}

// DeserializeCheckpoint deserializes a checkpoint record from bytes
func DeserializeCheckpoint(data []byte) (*CheckpointRecord, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("checkpoint data too short")
	}

	// Read size
	size := binary.BigEndian.Uint32(data[0:4])
	if uint32(len(data)) < size {
		return nil, fmt.Errorf("checkpoint data truncated: expected %d, got %d", size, len(data))
	}

	buf := bytes.NewReader(data[4:])
	cp := &CheckpointRecord{
		ActiveTxns: make(map[int64]*TransactionLogInfo),
		DirtyPages: make(map[primitives.HashCode]primitives.LSN),
	}

	// Read header
	var lsn, timestamp uint64
	if err := binary.Read(buf, binary.BigEndian, &lsn); err != nil {
		return nil, fmt.Errorf("failed to read LSN: %w", err)
	}
	cp.LSN = primitives.LSN(lsn)

	if err := binary.Read(buf, binary.BigEndian, &timestamp); err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}
	cp.Timestamp = time.Unix(int64(timestamp), 0)

	// Read active transactions
	var numTxns uint32
	if err := binary.Read(buf, binary.BigEndian, &numTxns); err != nil {
		return nil, fmt.Errorf("failed to read transaction count: %w", err)
	}

	for i := uint32(0); i < numTxns; i++ {
		var tid, firstLSN, lastLSN, undoNextLSN uint64

		if err := binary.Read(buf, binary.BigEndian, &tid); err != nil {
			return nil, fmt.Errorf("failed to read TID: %w", err)
		}
		if err := binary.Read(buf, binary.BigEndian, &firstLSN); err != nil {
			return nil, fmt.Errorf("failed to read FirstLSN: %w", err)
		}
		if err := binary.Read(buf, binary.BigEndian, &lastLSN); err != nil {
			return nil, fmt.Errorf("failed to read LastLSN: %w", err)
		}
		if err := binary.Read(buf, binary.BigEndian, &undoNextLSN); err != nil {
			return nil, fmt.Errorf("failed to read UndoNextLSN: %w", err)
		}

		cp.ActiveTxns[int64(tid)] = &TransactionLogInfo{
			FirstLSN:    primitives.LSN(firstLSN),
			LastLSN:     primitives.LSN(lastLSN),
			UndoNextLSN: primitives.LSN(undoNextLSN),
		}
	}

	// Read dirty pages
	var numPages uint32
	if err := binary.Read(buf, binary.BigEndian, &numPages); err != nil {
		return nil, fmt.Errorf("failed to read page count: %w", err)
	}

	for i := uint32(0); i < numPages; i++ {
		var pageHash, lsn uint64

		if err := binary.Read(buf, binary.BigEndian, &pageHash); err != nil {
			return nil, fmt.Errorf("failed to read page hash: %w", err)
		}
		if err := binary.Read(buf, binary.BigEndian, &lsn); err != nil {
			return nil, fmt.Errorf("failed to read page LSN: %w", err)
		}

		cp.DirtyPages[primitives.HashCode(pageHash)] = primitives.LSN(lsn)
	}

	return cp, nil
}

// Size returns the serialized size of the checkpoint record
func (cp *CheckpointRecord) Size() int {
	// Size field (4) + LSN (8) + Timestamp (8) + NumTxns (4) + NumPages (4)
	baseSize := 4 + 8 + 8 + 4 + 4

	// Each transaction: TID (8) + FirstLSN (8) + LastLSN (8) + UndoNextLSN (8) = 32 bytes
	txnSize := len(cp.ActiveTxns) * 32

	// Each page: PageHash (8) + LSN (8) = 16 bytes
	pageSize := len(cp.DirtyPages) * 16

	return baseSize + txnSize + pageSize
}
