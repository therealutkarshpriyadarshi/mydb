# Logging Guidelines

This document defines logging standards for the StoreMy database project.

## Overview

We use Go's standard `log/slog` package for structured logging. All logging goes through the `pkg/logging` package.

## Initialization

Initialize logging once at application startup:

```go
import "storemy/pkg/logging"

// In main.go or database initialization
logging.Init(logging.Config{
    Level:      logging.LevelInfo,  // DEBUG, INFO, WARN, ERROR
    OutputPath: "logs/database.log", // Empty string = stdout
    Format:     "json",              // "json" or "text"
})
```

## Log Levels

Use appropriate log levels:

- **DEBUG**: Detailed diagnostic information (disabled in production)
  - Function entry/exit
  - Variable values
  - Iteration counts
  - Cache hits/misses

- **INFO**: Important business events
  - Transaction begin/commit
  - Table/index creation
  - Query execution start/finish
  - Lock acquisition

- **WARN**: Potentially problematic situations
  - Retries
  - Deprecated API usage
  - Resource thresholds exceeded
  - Fallback behavior triggered

- **ERROR**: Error conditions that don't halt execution
  - Failed operations
  - Constraint violations
  - I/O errors
  - Deadlocks detected

## Where to Add Logging

### 1. Transaction Boundaries
```go
func (ps *PageStore) BeginTransaction() int {
    txID := ps.generateTxID()
    log := logging.WithTx(txID)
    log.Info("transaction started")
    // ... logic ...
    return txID
}

func (ps *PageStore) CommitTransaction(txID int) error {
    log := logging.WithTx(txID)
    log.Info("transaction committing", "dirty_pages", count)
    // ... logic ...
    log.Info("transaction committed")
    return nil
}
```

### 2. Catalog Operations
```go
func (cm *CatalogManager) CreateTable(tx TxContext, tableName string, ...) error {
    log := logging.WithTableTx(tx, tableName)
    log.Info("creating table", "columns", len(columns))

    // ... validation ...

    if err := cm.insertTable(tx, metadata); err != nil {
        log.Error("failed to register table", "error", err)
        return err
    }

    log.Info("table created successfully", "table_id", tableID)
    return nil
}
```

### 3. Query Execution
```go
func (db *Database) ExecuteQuery(query string) (*QueryResult, error) {
    log := logging.Logger.With("query_type", queryType)
    log.Info("executing query", "query_length", len(query))

    // ... parsing, planning, execution ...

    log.Info("query completed", "duration_ms", elapsed, "rows_affected", count)
    return result, nil
}
```

### 4. Lock Operations
```go
func (lm *LockManager) AcquireLock(txID int, resourceID string, lockType LockType) error {
    log := logging.WithLock(txID, resourceID)
    log.Debug("requesting lock", "lock_type", lockType)

    // ... acquire logic ...

    if deadlock {
        log.Warn("deadlock detected", "victim_tx", victimTxID)
        return ErrDeadlock
    }

    log.Info("lock acquired", "lock_type", lockType, "wait_time_ms", waitTime)
    return nil
}
```

### 5. Page I/O and Buffer Pool
```go
func (bp *BufferPool) PinPage(pageID int, txID int) (*Page, error) {
    log := logging.WithPage(pageID).With("tx_id", txID)
    log.Debug("pinning page")

    // ... logic ...

    if evicted {
        log.Debug("page evicted to make room", "evicted_page", evictedPageID)
    }

    return page, nil
}
```

### 6. Index Operations
```go
func (idx *BTreeIndex) Insert(key Field, recordID RecordID) error {
    log := logging.WithIndex(idx.name)
    log.Debug("inserting into index", "key", key.String())

    // ... logic ...

    if split {
        log.Debug("node split occurred", "level", level)
    }

    return nil
}
```

### 7. Error Paths
```go
func someOperation() error {
    result, err := riskyOperation()
    if err != nil {
        logging.Error("operation failed",
            "operation", "someOperation",
            "error", err,
            "context", additionalInfo)
        return fmt.Errorf("failed to perform operation: %w", err)
    }
    return nil
}
```

## Best Practices

### DO ✅

1. **Use structured logging** (key-value pairs):
   ```go
   log.Info("table created", "table", name, "columns", count)
   ```

2. **Use context helpers** for automatic attributes:
   ```go
   log := logging.WithTableTx(tx, tableName)
   log.Info("operation") // Automatically includes tx_id and table
   ```

3. **Log at function entry/exit for complex operations**:
   ```go
   func ComplexOperation() error {
       log := logging.WithComponent("catalog")
       log.Debug("starting complex operation")
       defer log.Debug("finished complex operation")
       // ... logic ...
   }
   ```

4. **Include metrics** (counts, durations, sizes):
   ```go
   log.Info("query executed", "duration_ms", elapsed, "rows", rowCount)
   ```

5. **Log state transitions**:
   ```go
   log.Info("lock state changed", "from", oldState, "to", newState)
   ```

### DON'T ❌

1. **Don't log in tight loops** (use counters instead):
   ```go
   // BAD
   for _, row := range rows {
       log.Debug("processing row", "row", row) // Spam!
   }

   // GOOD
   log.Debug("processing rows", "count", len(rows))
   ```

2. **Don't use string concatenation**:
   ```go
   // BAD
   log.Info("Table " + name + " created")

   // GOOD
   log.Info("table created", "table", name)
   ```

3. **Don't log sensitive data** (passwords, PII):
   ```go
   // BAD
   log.Info("user login", "password", password)

   // GOOD
   log.Info("user login", "user_id", userID)
   ```

4. **Don't ignore log levels**:
   ```go
   // BAD - Everything at INFO
   log.Info("variable x =", x)

   // GOOD - Use DEBUG for diagnostic details
   log.Debug("variable state", "x", x)
   ```

5. **Don't duplicate error messages**:
   ```go
   // BAD
   if err != nil {
       log.Error("insert failed", "error", err)
       return fmt.Errorf("insert failed: %w", err) // Duplicate!
   }

   // GOOD
   if err != nil {
       log.Error("insert failed", "error", err)
       return err // Or wrap with additional context
   }
   ```

## Performance Considerations

1. **Lazy evaluation** - slog only evaluates args if level is enabled:
   ```go
   log.Debug("stats", "count", expensiveCount()) // Only called if DEBUG enabled
   ```

2. **Reuse loggers** - Create once, use many times:
   ```go
   type TableManager struct {
       log *slog.Logger
   }

   func NewTableManager(tableName string) *TableManager {
       return &TableManager{
           log: logging.WithTable(tableName),
       }
   }
   ```

3. **Avoid allocations** in hot paths - use numeric types over strings:
   ```go
   log.Debug("page accessed", "page_id", 42) // Better than "page_id", "42"
   ```

## Example Integration

Here's a complete example of adding logging to an existing function:

```go
// BEFORE
func (cm *CatalogManager) CreateIndex(tx TxContext, indexName, tableName, columnName string, indexType index.IndexType) (int, string, error) {
    tableID, err := cm.GetTableID(tx, tableName)
    if err != nil {
        return 0, "", fmt.Errorf("table %s not found: %w", tableName, err)
    }

    if err := cm.canCreateIndex(tx, tableID, columnName, indexName, tableName); err != nil {
        return 0, "", err
    }

    metadata := cm.createIndexMeta(tableID, tableName, columnName, indexName, indexType)

    if err := cm.insertIndex(tx, *metadata); err != nil {
        return 0, "", fmt.Errorf("failed to register index in catalog: %w", err)
    }

    return metadata.IndexID, metadata.FilePath, nil
}

// AFTER (with logging)
func (cm *CatalogManager) CreateIndex(tx TxContext, indexName, tableName, columnName string, indexType index.IndexType) (int, string, error) {
    log := logging.WithTableTx(tx, tableName).With("index", indexName)
    log.Info("creating index", "column", columnName, "type", indexType)

    tableID, err := cm.GetTableID(tx, tableName)
    if err != nil {
        log.Error("table not found", "error", err)
        return 0, "", fmt.Errorf("table %s not found: %w", tableName, err)
    }

    if err := cm.canCreateIndex(tx, tableID, columnName, indexName, tableName); err != nil {
        log.Warn("index validation failed", "error", err)
        return 0, "", err
    }

    metadata := cm.createIndexMeta(tableID, tableName, columnName, indexName, indexType)
    log.Debug("index metadata created", "index_id", metadata.IndexID, "file_path", metadata.FilePath)

    if err := cm.insertIndex(tx, *metadata); err != nil {
        log.Error("failed to register index in catalog", "error", err)
        return 0, "", fmt.Errorf("failed to register index in catalog: %w", err)
    }

    log.Info("index created successfully", "index_id", metadata.IndexID)
    return metadata.IndexID, metadata.FilePath, nil
}
```

## Testing with Logging

In tests, either initialize a test logger or use the default:

```go
func TestSomething(t *testing.T) {
    // Option 1: Suppress logs
    logging.Init(logging.Config{Level: logging.LevelError})

    // Option 2: Log to test output
    logging.InitDefault()

    // ... your test ...
}
```

## Production Configuration

```go
// Production: JSON format, file output, INFO level
logging.Init(logging.Config{
    Level:      logging.LevelInfo,
    OutputPath: "/var/log/storemy/database.log",
    Format:     "json",
})

// Development: Text format, stdout, DEBUG level
logging.Init(logging.Config{
    Level:      logging.LevelDebug,
    OutputPath: "",
    Format:     "text",
})
```
