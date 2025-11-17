package indexmanager

import (
	"storemy/pkg/concurrency/transaction"
	"storemy/pkg/primitives"
	"storemy/pkg/tuple"
	"storemy/pkg/types"
)

// IndexSearcher implements searching across all indexes for a table.
// This is used by constraint validation to check for duplicate values in UNIQUE constraints.
type IndexSearcherImpl struct {
	indexManager *IndexManager
}

// NewIndexSearcher creates a new IndexSearcher.
func NewIndexSearcher(indexManager *IndexManager) *IndexSearcherImpl {
	return &IndexSearcherImpl{
		indexManager: indexManager,
	}
}

// SearchIndexForKey searches an index for a specific key value.
// Returns all record IDs that match the key, or an error if the index doesn't exist.
//
// Parameters:
//   - tx: Transaction context
//   - tableID: ID of the table
//   - columnIndex: Index of the column to search
//   - keyValue: The key value to search for
//
// Returns:
//   - A slice of record IDs matching the key
//   - An error if the index doesn't exist or search fails
func (is *IndexSearcherImpl) SearchIndexForKey(
	tx *transaction.TransactionContext,
	tableID primitives.FileID,
	columnIndex primitives.ColumnID,
	keyValue types.Field,
) ([]*tuple.TupleRecordID, error) {
	// Load the index for this column
	loader := is.indexManager.NewLoader(tx)
	index, err := loader.LoadIndexForCol(columnIndex, tableID)
	if err != nil {
		return nil, err
	}

	// Search the index for the key
	recordIDs, err := index.Search(keyValue)
	if err != nil {
		return nil, err
	}

	// Return the record IDs directly (they are already []*tuple.TupleRecordID)
	return recordIDs, nil
}
