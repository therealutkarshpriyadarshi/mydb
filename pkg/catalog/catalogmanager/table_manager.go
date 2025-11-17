package catalogmanager

import (
	"fmt"
	"storemy/pkg/catalog/operations"
	"storemy/pkg/catalog/schema"
	"storemy/pkg/catalog/systemtable"
	"storemy/pkg/catalog/tablecache"
	"storemy/pkg/concurrency/transaction"
	"storemy/pkg/primitives"
	"storemy/pkg/storage/heap"
	"sync"
)

// TableCatalogOperation encapsulates table-related catalog operations.
//
// This struct provides a clean interface for table lifecycle operations
// (create, drop, rename, load) and metadata queries. It reduces coupling
// by encapsulating dependencies on the catalog manager's internal state.
//
// Design Philosophy:
//   - Single transaction context per operation set
//   - Cache-first for reads (fast path)
//   - Atomic operations (disk + cache updated together)
//   - Proper error handling with rollback support
//
// Usage:
//
//	tableOps := cm.NewTableOps(tx)
//	tableID, err := tableOps.CreateTable(schema)
//	if err != nil {
//	    log.Fatal(err)
//	}
type TableCatalogOperation struct {
	tx           *transaction.TransactionContext
	cache        *tablecache.TableCache
	tableOps     *operations.TableOperations
	colOps       *operations.ColumnOperations
	tablesFileID primitives.FileID
	mu           sync.RWMutex
	cm           *CatalogManager
}

// NewTableOps creates a new TableCatalogOperation instance.
//
// This is the entry point for table operations, providing a transaction-scoped
// context for all table-related catalog operations.
//
// Parameters:
//   - tx: Transaction context for all catalog operations
//
// Returns:
//   - *TableCatalogOperation: New table operations instance
func (cm *CatalogManager) NewTableOps(tx *transaction.TransactionContext) *TableCatalogOperation {
	return &TableCatalogOperation{
		tx:           tx,
		cache:        cm.tableCache,
		tableOps:     cm.tableOps,
		colOps:       cm.colOps,
		tablesFileID: cm.SystemTabs.TablesTableID,
		cm:           cm,
	}
}

// CreateTable creates a new table in the database.
//
// This is a multi-step atomic operation:
//  1. Validates schema is not nil
//  2. Checks table name uniqueness
//  3. Creates physical heap file
//  4. Registers table metadata in CATALOG_TABLES
//  5. Registers column metadata in CATALOG_COLUMNS
//  6. Adds table to in-memory cache
//  7. Registers with page store
//
// If any step fails, the operation rolls back automatically.
//
// Parameters:
//   - sch: TableSchema containing table definition
//
// Returns:
//   - primitives.FileID: The auto-generated table ID
//   - error: nil on success, error describing failure otherwise
func (to *TableCatalogOperation) CreateTable(sch TableSchema) (primitives.FileID, error) {
	if sch == nil {
		return 0, fmt.Errorf("schema cannot be nil")
	}

	to.mu.Lock()
	defer to.mu.Unlock()

	if to.TableExists(sch.TableName) {
		return 0, fmt.Errorf("table %s already exists", sch.TableName)
	}

	heapFile, err := to.createTableFile(sch)
	if err != nil {
		return 0, err
	}

	if err := to.registerTable(sch, heapFile.FilePath()); err != nil {
		heapFile.Close()
		return 0, fmt.Errorf("failed to register table in catalog: %w", err)
	}

	if err := to.addTableToCache(heapFile, sch); err != nil {
		return 0, err
	}

	return sch.TableID, nil
}

// createTableFile creates the physical heap file for a table.
//
// The file is created in the data directory with naming convention:
// <table_name>.dat
//
// After creation, the table ID and column table IDs are updated in the schema
// to match the auto-generated ID from the heap file.
//
// Parameters:
//   - sch: TableSchema containing the table structure
//
// Returns:
//   - *heap.HeapFile: The newly created heap file
//   - error: nil on success, error if file creation fails
func (to *TableCatalogOperation) createTableFile(sch TableSchema) (*heap.HeapFile, error) {
	fileName := sch.TableName + ".dat"
	fullPath := primitives.Filepath(to.cm.dataDir).Join(fileName)

	heapFile, err := heap.NewHeapFile(fullPath, sch.TupleDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to create heap file: %w", err)
	}

	tableID := heapFile.GetID()
	sch.TableID = tableID
	for i := range sch.Columns {
		sch.Columns[i].TableID = tableID
	}

	return heapFile, nil
}

// registerTable adds table metadata to the catalog.
//
// This inserts entries into:
//   - CATALOG_TABLES: table metadata
//   - CATALOG_COLUMNS: column definitions
//
// This is an internal helper used by CreateTable.
//
// Parameters:
//   - sch: Table schema
//   - filepath: Path to heap file
//
// Returns:
//   - error: nil on success, error if registration fails
func (to *TableCatalogOperation) registerTable(sch *schema.Schema, filepath primitives.Filepath) error {
	tm := &systemtable.TableMetadata{
		TableName:     sch.TableName,
		TableID:       sch.TableID,
		FilePath:      filepath,
		PrimaryKeyCol: sch.PrimaryKey,
	}
	if err := to.tableOps.Insert(to.tx, tm); err != nil {
		return err
	}

	if err := to.colOps.InsertColumns(to.tx, sch.Columns); err != nil {
		return err
	}

	return nil
}

// addTableToCache adds a newly created table to the in-memory cache.
//
// If adding to cache fails, the method attempts to rollback by:
//  1. Deleting the catalog entry
//  2. Closing the heap file
//
// The method also:
//   - Tracks the open file in cm.openFiles
//   - Registers the file with the page store
//   - Verifies the cache entry was successfully added
//
// Parameters:
//   - file: The heap file to cache
//   - sch: The table schema
//
// Returns:
//   - error: nil on success, error if caching or verification fails
func (to *TableCatalogOperation) addTableToCache(file *heap.HeapFile, sch TableSchema) error {
	if err := to.cache.AddTable(file, sch); err != nil {
		if deleteErr := to.cm.DeleteCatalogEntry(to.tx, sch.TableID); deleteErr != nil {
			fmt.Printf("Warning: failed to rollback catalog entry after cache failure: %v\n", deleteErr)
		}
		file.Close()
		return fmt.Errorf("failed to add table to cache: %w", err)
	}

	to.cm.mu.Lock()
	to.cm.openFiles[sch.TableID] = file
	to.cm.mu.Unlock()

	to.cm.store.RegisterDbFile(sch.TableID, file)
	if _, verifyErr := to.cache.GetDbFile(sch.TableID); verifyErr != nil {
		return fmt.Errorf("table was added to cache but immediate verification failed: %w", verifyErr)
	}

	return nil
}

// DropTable permanently removes a table from the database.
//
// This operation removes:
//  1. In-memory cache entry
//  2. Open heap file handle
//  3. Page store registration
//  4. CATALOG_TABLES entry
//  5. CATALOG_COLUMNS entries
//  6. CATALOG_STATISTICS entries
//  7. CATALOG_INDEXES entries
//
// Note: The physical heap file is NOT deleted from disk.
//
// The operation is atomic - if cache removal fails, the table is not dropped.
// If disk catalog deletion fails after cache removal, the operation attempts to
// rollback by re-adding the table to cache.
//
// Parameters:
//   - tableName: Name of the table to drop
//
// Returns:
//   - error: nil on success, error if table not found or deletion fails
func (to *TableCatalogOperation) DropTable(tableName string) error {
	tableID, err := to.GetTableID(tableName)
	if err != nil {
		return fmt.Errorf("table %s not found: %w", tableName, err)
	}

	// Get table info before removing from cache (needed for potential rollback)
	tableInfo, err := to.cache.GetTableInfo(tableID)
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}

	// Step 1: Remove from cache FIRST so queries immediately stop finding it
	if err := to.cache.RemoveTable(tableName); err != nil {
		return fmt.Errorf("failed to remove table from cache: %w", err)
	}

	// Step 2: Close and remove file handle
	to.cm.mu.Lock()
	heapFile, exists := to.cm.openFiles[tableID]
	if exists {
		delete(to.cm.openFiles, tableID)
	}
	to.cm.mu.Unlock()

	if exists {
		heapFile.Close()
	}

	// Step 3: Unregister from page store
	to.cm.store.UnregisterDbFile(tableID)

	// Step 4: Delete from disk catalog
	if err := to.cm.DeleteCatalogEntry(to.tx, tableID); err != nil {
		// Rollback: Try to re-add table to cache
		if rollbackErr := to.cache.AddTable(tableInfo.File, tableInfo.Schema); rollbackErr != nil {
			fmt.Printf("CRITICAL: failed to rollback cache after disk deletion failure: %v (original error: %v)\n", rollbackErr, err)
		}
		return fmt.Errorf("failed to delete catalog entry: %w", err)
	}

	return nil
}

// RenameTable renames a table in both memory and disk catalog.
//
// This is an atomic operation that updates the table name in:
//  1. In-memory cache
//  2. CATALOG_TABLES entry on disk
//
// If disk update fails, the in-memory rename is rolled back.
//
// The physical heap file is NOT renamed - only the logical table name changes.
//
// Parameters:
//   - oldName: Current table name
//   - newName: New table name (must not exist)
//
// Returns:
//   - error: nil on success, error if validation fails or rename cannot complete
func (to *TableCatalogOperation) RenameTable(oldName, newName string) error {
	if to.TableExists(newName) {
		return fmt.Errorf("table %s already exists", newName)
	}

	if err := to.cache.RenameTable(oldName, newName); err != nil {
		return fmt.Errorf("failed to rename in memory: %w", err)
	}

	err := to.tableOps.UpdateBy(to.tx,
		func(tm *systemtable.TableMetadata) bool {
			return tm.TableName == oldName
		},
		func(tm *systemtable.TableMetadata) *systemtable.TableMetadata {
			tm.TableName = newName
			return tm
		})

	if err != nil {
		// Rollback in-memory rename
		to.cache.RenameTable(newName, oldName)
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}
	return nil
}

// LoadTable loads a table from disk into memory on-demand.
//
// This is used for lazy loading - tables are only loaded when first accessed
// rather than all at startup. If the table is already in cache, this is a no-op.
//
// Steps performed:
//  1. Checks if table already exists in cache
//  2. Reads table metadata from CATALOG_TABLES
//  3. Reconstructs the schema from CATALOG_COLUMNS
//  4. Opens the heap file from disk
//  5. Adds table to in-memory cache
//  6. Registers with the page store
//
// Parameters:
//   - tableName: Name of the table to load
//
// Returns:
//   - error: nil on success, error if table doesn't exist or loading fails
func (to *TableCatalogOperation) LoadTable(tableName string) error {
	if to.cache.TableExists(tableName) {
		return nil
	}

	sch, filePath, err := to.loadFromDisk(tableName)
	if err != nil {
		return err
	}

	return to.openTable(filePath, sch)
}

// loadFromDisk retrieves table metadata and schema from the catalog.
//
// This is an internal helper that reads from CATALOG_TABLES and
// CATALOG_COLUMNS to reconstruct a complete TableSchema object.
//
// Parameters:
//   - tableName: Name of the table to load
//
// Returns:
//   - TableSchema: The reconstructed schema with columns and tuple descriptor
//   - primitives.Filepath: The file path to the heap file
//   - error: nil on success, error if metadata cannot be read
func (to *TableCatalogOperation) loadFromDisk(tableName string) (TableSchema, primitives.Filepath, error) {
	tm, err := to.GetTableMetadataByName(tableName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get table metadata: %w", err)
	}

	sch, err := to.LoadTableSchema(tm.TableID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load schema: %w", err)
	}

	return sch, tm.FilePath, nil
}

// openTable opens a heap file and registers it with the cache and page store.
//
// This is an internal helper used during table loading. It handles
// the physical file opening and registration with all necessary components.
//
// If adding to cache fails, the heap file is automatically closed.
//
// Parameters:
//   - filePath: Absolute path to the heap file
//   - sch: The table schema
//
// Returns:
//   - error: nil on success, error if file cannot be opened or registered
func (to *TableCatalogOperation) openTable(filePath primitives.Filepath, sch TableSchema) error {
	heapFile, err := heap.NewHeapFile(filePath, sch.TupleDesc)
	if err != nil {
		return fmt.Errorf("failed to open heap file: %w", err)
	}

	if err := to.cache.AddTable(heapFile, sch); err != nil {
		heapFile.Close()
		return fmt.Errorf("failed to add table to cache: %w", err)
	}

	to.cm.mu.Lock()
	to.cm.openFiles[sch.TableID] = heapFile
	to.cm.mu.Unlock()

	to.cm.store.RegisterDbFile(sch.TableID, heapFile)
	return nil
}

// LoadAllTables loads all user tables from disk into memory.
//
// This reads CATALOG_TABLES, reconstructs schemas from CATALOG_COLUMNS,
// opens heap files, and registers everything with the page store.
//
// System tables (CATALOG_*) are not loaded by this method as they are
// managed separately during initialization.
//
// The operation stops at the first error encountered.
//
// Returns:
//   - error: nil if all tables loaded successfully, error describing which table failed
func (to *TableCatalogOperation) LoadAllTables() error {
	tables, err := to.tableOps.GetAllTables(to.tx)
	if err != nil {
		return fmt.Errorf("failed to read tables from catalog: %w", err)
	}

	for _, table := range tables {
		if err := to.LoadTable(table.TableName); err != nil {
			return fmt.Errorf("error loading the table %s: %v", table.TableName, err)
		}
	}

	return nil
}

// GetTableID retrieves the table ID for a given table name.
//
// Checks in-memory cache first (O(1)), then falls back to disk scan.
//
// Parameters:
//   - tableName: Name of the table
//
// Returns:
//   - primitives.FileID: The table's ID
//   - error: Error if table is not found
func (to *TableCatalogOperation) GetTableID(tableName string) (primitives.FileID, error) {
	if id, err := to.cache.GetTableID(tableName); err == nil {
		return id, nil
	}

	if md, err := to.GetTableMetadataByName(tableName); err == nil {
		return md.TableID, nil
	}

	return 0, fmt.Errorf("table %s not found", tableName)
}

// GetTableName retrieves the table name for a given table ID.
//
// Checks in-memory cache first (O(1)), then falls back to disk scan.
//
// Parameters:
//   - tableID: ID of the table
//
// Returns:
//   - string: The table's name
//   - error: Error if table is not found
func (to *TableCatalogOperation) GetTableName(tableID primitives.FileID) (string, error) {
	if info, err := to.cache.GetTableInfo(tableID); err == nil {
		return info.Schema.TableName, nil
	}

	if md, err := to.GetTableMetadataByID(tableID); err == nil {
		return md.TableName, nil
	}

	return "", fmt.Errorf("table with ID %d not found", tableID)
}

// GetTableSchema retrieves the schema for a table.
//
// Checks in-memory cache first, then loads from disk if necessary.
//
// Parameters:
//   - tableID: ID of the table
//
// Returns:
//   - *schema.Schema: The table's schema definition
//   - error: Error if schema cannot be retrieved
func (to *TableCatalogOperation) GetTableSchema(tableID primitives.FileID) (*schema.Schema, error) {
	if info, err := to.cache.GetTableInfo(tableID); err == nil {
		return info.Schema, nil
	}

	schema, err := to.LoadTableSchema(tableID)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// TableExists checks if a table exists by name.
//
// Checks memory first (fast), then disk catalog (slower).
//
// Parameters:
//   - tableName: Name of the table
//
// Returns:
//   - bool: true if table exists, false otherwise
func (to *TableCatalogOperation) TableExists(tableName string) bool {
	if to.cache.TableExists(tableName) {
		return true
	}
	_, err := to.GetTableMetadataByName(tableName)
	return err == nil
}

// ListAllTables returns all table names.
//
// Parameters:
//   - refreshFromDisk: If true, scans CATALOG_TABLES (includes unloaded tables).
//     If false, returns tables from memory cache (faster).
//
// Returns:
//   - []string: List of table names
//   - error: Error if disk scan fails (only when refreshFromDisk=true)
func (to *TableCatalogOperation) ListAllTables(refreshFromDisk bool) ([]string, error) {
	if !refreshFromDisk {
		return to.cache.GetAllTableNames(), nil
	}

	tables, err := to.tableOps.GetAllTables(to.tx)
	if err != nil {
		return nil, err
	}

	tableNames := make([]string, 0, len(tables))
	for _, table := range tables {
		tableNames = append(tableNames, table.TableName)
	}

	return tableNames, nil
}

// GetTableMetadataByID retrieves complete table metadata from CATALOG_TABLES by table ID.
//
// Parameters:
//   - tableID: ID of the table
//
// Returns:
//   - *systemtable.TableMetadata: Table metadata
//   - error: Error if table is not found
func (to *TableCatalogOperation) GetTableMetadataByID(tableID primitives.FileID) (*systemtable.TableMetadata, error) {
	return to.tableOps.GetTableMetadataByID(to.tx, tableID)
}

// GetTableMetadataByName retrieves complete table metadata from CATALOG_TABLES by table name.
//
// Table name matching is case-insensitive.
//
// Parameters:
//   - tableName: Name of the table
//
// Returns:
//   - *systemtable.TableMetadata: Table metadata
//   - error: Error if table is not found
func (to *TableCatalogOperation) GetTableMetadataByName(tableName string) (*systemtable.TableMetadata, error) {
	return to.tableOps.GetTableMetadataByName(to.tx, tableName)
}

// GetAllTables retrieves metadata for all tables registered in the catalog.
//
// This includes both user tables and system catalog tables.
//
// Returns:
//   - []*systemtable.TableMetadata: List of table metadata
//   - error: Error if catalog read fails
func (to *TableCatalogOperation) GetAllTables() ([]*systemtable.TableMetadata, error) {
	return to.tableOps.GetAllTables(to.tx)
}

// LoadTableSchema reconstructs the complete schema for a table from CATALOG_COLUMNS.
//
// This includes column definitions, types, and constraints.
//
// Parameters:
//   - tableID: ID of the table
//
// Returns:
//   - *schema.Schema: The reconstructed schema
//   - error: Error if schema cannot be loaded
func (to *TableCatalogOperation) LoadTableSchema(tableID primitives.FileID) (*schema.Schema, error) {
	tm, err := to.GetTableMetadataByID(tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to get table metadata: %w", err)
	}

	columns, err := to.colOps.LoadColumnMetadata(to.tx, tableID)
	if err != nil {
		return nil, err
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("no columns found for table %d", tableID)
	}

	sch, err := schema.NewSchema(tableID, tm.TableName, columns)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return sch, nil
}
