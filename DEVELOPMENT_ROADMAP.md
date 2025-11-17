# StoreMy Database - Development Roadmap & Analysis

**Generated:** 2025-10-16
**Project:** StoreMy - Educational Database Management System
**Status:** Active Development

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [What's Already Implemented](#whats-already-implemented)
3. [Architecture Analysis](#architecture-analysis)
4. [What's Missing](#whats-missing)
5. [Development Roadmap](#development-roadmap)
6. [Quick Wins](#quick-wins)
7. [Index-Heap Integration Analysis](#index-heap-integration-analysis)

---

## Project Overview

StoreMy is a PostgreSQL-inspired relational database management system written in Go. It features:

- **Storage Layer:** Slotted page structure for heap files
- **Indexing:** B+Tree and Hash indexes
- **Transactions:** 2-Phase Locking with WAL
- **Query Engine:** Iterator-based execution model
- **SQL Parser:** Support for DDL and DML operations
- **Buffer Management:** LRU-based page cache

**Target Audience:** Educational, learning database internals
**Language:** Go (Type-safe, concurrent)
**Architecture:** Single-process database with interactive CLI

---

## What's Already Implemented

### ✅ Storage Layer

#### Heap Storage (`pkg/storage/heap/`)
- **Slotted Page Structure** - PostgreSQL-style with stable RecordIDs
- **Variable-Length Tuples** - Support for different tuple sizes
- **Page Compaction** - Defragmentation without invalidating RecordIDs
- **Slot Pointer Array** - Efficient space management
- **Thread-Safe Operations** - Concurrent page access with mutexes

**Key Features:**
```
Page Layout:
[SlotPointer0][SlotPointer1]...[SlotPointerN][FreeSpace][...TupleData...]
     ↑ Slot numbers remain stable even after compaction
```

- Max tuple size: 65535 bytes
- Slot pointer size: 4 bytes (offset + length)
- Page size: 4096 bytes

#### Index Storage

##### B+Tree Index (`pkg/storage/index/btree/`)
- Leaf and internal page structures
- Key-value storage with RecordID pointers
- Page serialization/deserialization
- Dirty page tracking for transactions
- Before-image support for rollback
- Max entries per page: 150 (conservative estimate)

**Features:**
- Internal nodes: Separator keys + child pointers
- Leaf nodes: (Key, RecordID) pairs
- Leaf linking: PrevLeaf/NextLeaf for range scans
- Parent tracking for tree navigation

##### Hash Index (`pkg/storage/index/hash/`)
- Bucket-based organization
- Overflow page chaining
- Entry find/add/remove operations
- Support for duplicate keys (multiple RIDs per key)
- Max entries per page: 150

**Features:**
- Fixed bucket allocation
- Dynamic overflow when bucket fills
- O(1) point lookups
- Poor range query performance

#### Page Management (`pkg/storage/page/`)
- Common page interface for all page types
- PageID abstraction (HeapPageID, BTreePageID, HashPageID)
- Page serialization protocol
- Dirty tracking with transaction IDs

---

### ✅ Memory Management

#### Buffer Pool (`pkg/memory/cache.go`)
- **LRU Eviction Policy** - Least Recently Used pages evicted first
- **NO-STEAL Policy** - Uncommitted pages never evicted
- **FORCE Policy** - Pages flushed at commit time
- **Page Pinning** - Prevent eviction of active pages
- **Thread-Safe** - Concurrent access with proper locking

**Configuration:**
- Default capacity: 50 pages
- Configurable pool size
- Page replacement on cache miss

**Statistics Tracking:**
- Cache hits/misses
- Hit ratio calculation
- Eviction count

---

### ✅ Transaction Management

#### Transaction Context (`pkg/concurrency/transaction/context.go`)

**Full Transaction Lifecycle:**
```
ACTIVE → COMMITTING → COMMITTED
   ↓
ABORTING → ABORTED
```

**Features:**
- Transaction ID generation (atomic counter)
- State machine for transaction phases
- Page access tracking (read/write lists)
- Tuple-level read/write statistics
- Deadlock detection support (waitingFor graph)
- WAL LSN chain management

**Page Permissions:**
```go
type Permissions int
const (
    None Permissions = iota
    ReadOnly
    ReadWrite
)
```

**Transaction Statistics:**
- Pages read/written
- Tuples read/written
- Duration tracking
- Lock acquisition times

#### Lock Manager (`pkg/concurrency/lock/`)

**2-Phase Locking (2PL) Protocol:**
1. **Growing Phase:** Acquire locks as needed
2. **Shrinking Phase:** Release all locks at commit/abort

**Lock Modes:**
- **Shared (S):** Multiple transactions can read
- **Exclusive (X):** Single transaction for write
- Strict 2PL: Locks held until transaction end

**Deadlock Detection:**
- Wait-for graph construction
- Cycle detection using DFS
- Transaction abort for deadlock resolution
- Configurable victim selection

**Features:**
- Lock compatibility matrix
- Lock upgrade (S → X when needed)
- Thread-safe lock table
- Per-page lock granularity

---

### ✅ Query Execution

#### Execution Operators (`pkg/execution/operators/`)

**Iterator Pattern Implementation:**
```go
type Operator interface {
    Open() error
    Next() (*tuple.Tuple, error)
    Close() error
    GetChildren() []Operator
}
```

**Scan Operators:**
1. **Sequential Scan** (`seqscan.go`)
   - Full table scan with page-by-page iteration
   - Automatic page locking (shared locks)
   - Tuple filtering during scan
   - Memory-efficient streaming

2. **Index Scan** (`indexscan.go`)
   - B+Tree index navigation
   - Range scan support
   - Index-to-heap RID following
   - Cost-based index selection

**Filtering & Transformation:**
3. **Filter/Selection** (`filter.go`)
   - WHERE clause evaluation
   - Predicate pushdown
   - Complex condition support (AND/OR/NOT)

4. **Projection** (`project.go`)
   - SELECT field list
   - Column reordering
   - Expression evaluation

**Join Operators:**
5. **Nested Loop Join** (`nested_loop_join.go`)
   - Simple but slow O(n×m)
   - Good for small tables
   - Supports all join types (INNER, LEFT, RIGHT, FULL)

6. **Hash Join** (`hash_join.go`)
   - Build hash table from smaller relation
   - Probe with larger relation
   - O(n+m) complexity
   - Memory-intensive

7. **Sort-Merge Join** (`sort_merge_join.go`)
   - Sort both inputs on join key
   - Merge sorted streams
   - Good for pre-sorted data
   - O(n log n + m log m)

**Aggregation:**
8. **Aggregate** (`aggregate.go`)
   - GROUP BY support
   - Aggregate functions: COUNT, SUM, AVG, MIN, MAX
   - Hash-based grouping
   - Multiple aggregates per query

**Other Operators:**
9. **Limit** (`limit.go`)
   - Row count limiting
   - OFFSET support for pagination
   - Early termination optimization

10. **Delete** (`delete.go`)
    - Tuple deletion with index maintenance
    - Transaction-aware deletion
    - Lock acquisition for deleted tuples

11. **Insert** (`insert.go`)
    - Tuple insertion with index updates
    - Page selection for insertion
    - Transaction logging

**Join Strategy Selection:**
- Cost-based join algorithm selection
- Statistics-aware optimization
- Comprehensive test coverage
- Left-deep join tree construction

---

### ✅ Query Planning & Optimization

#### Query Planner (`pkg/planner/`)

**DDL Operations:**
- `CREATE TABLE` - Schema definition and catalog registration
- `CREATE INDEX` - Index creation (B+Tree or Hash)
- `DROP TABLE` - Table removal with cascading index deletion
- `DROP INDEX` - Index removal

**DML Operations:**
- `INSERT` - Row insertion planning
- `UPDATE` - Row update with index maintenance
- `DELETE` - Row deletion with index cleanup
- `SELECT` - Query plan generation

**Query Optimization:**
1. **Index Selection**
   - Choose appropriate index for query
   - Cost comparison: sequential vs index scan
   - Multi-index evaluation

2. **Join Ordering**
   - Left-deep join trees
   - Cost-based reordering
   - Statistics-driven decisions

3. **Predicate Pushdown**
   - Filter predicates moved close to data source
   - Reduces intermediate result sizes

4. **Aggregation Planning**
   - Hash-based vs sort-based aggregation
   - Group by optimization

**Cost Model:**
- I/O cost estimation
- CPU cost estimation
- Cardinality estimation (basic)
- Selectivity factors

---

### ✅ Catalog Management

#### System Catalog (`pkg/catalog/`)

**Metadata Tables:**

1. **CATALOG_TABLES**
   - Schema: `(table_id INT, table_name VARCHAR, file_path VARCHAR, primary_key VARCHAR)`
   - Stores table definitions
   - File path mapping
   - Primary key information

2. **CATALOG_COLUMNS**
   - Schema: `(table_id INT, column_name VARCHAR, column_type VARCHAR, column_position INT, is_primary BOOL)`
   - Column definitions per table
   - Type information
   - Ordinal position
   - Primary key flags

3. **CATALOG_STATISTICS**
   - Schema: `(table_id INT, num_tuples INT, num_pages INT, avg_tuple_size FLOAT)`
   - Table statistics for optimization
   - Updated on INSERT/DELETE/UPDATE
   - Used by query planner

4. **CATALOG_INDEXES**
   - Schema: `(index_id INT, index_name VARCHAR, table_id INT, column_name VARCHAR, index_type VARCHAR)`
   - Index registry
   - Type tracking (BTREE/HASH)
   - Column association

**Features:**
- **Table Cache** - In-memory schema caching
- **Transactional Catalog Ops** - Atomic metadata changes
- **Schema Validation** - Type checking and constraints
- **Lazy Loading** - Load metadata on demand

**CatalogManager (`pkg/catalog/catalogmanager/`):**
- Table creation/deletion
- Column management
- Statistics updates
- Index registration
- Thread-safe operations

---

### ✅ Index Management

#### IndexManager (`pkg/indexmanager/index_manager.go`)

**Core Responsibilities:**
1. Index lifecycle management
2. DML-aware index maintenance
3. Lazy loading and caching
4. Multi-index coordination

**Key Methods:**

```go
// Load index from disk or cache
GetIndex(tableID int, columnName string) (Index, error)

// DML Hooks - automatically maintain indexes
OnInsert(tableID int, tuple *Tuple) error
OnDelete(tableID int, tuple *Tuple) error
OnUpdate(tableID int, oldTuple, newTuple *Tuple) error
```

**Features:**
- **Index Caching** - LRU cache for loaded indexes
- **Automatic Sync** - Indexes updated on tuple changes
- **Multi-Index Support** - Multiple indexes per table
- **Thread-Safe** - Concurrent index access
- **Index Selection** - Query optimizer integration

**Integration Points:**
- Catalog: Metadata retrieval for index definitions
- Operators: Insert/Delete operators call DML hooks
- Planner: Index availability for query planning

---

### ✅ Durability & Recovery

#### Write-Ahead Log (WAL) (`pkg/log/`)

**Log Record Types:**
```go
const (
    BEGIN_TXN  int32 = 0  // Transaction start
    INSERT     int32 = 1  // Tuple insertion
    DELETE     int32 = 2  // Tuple deletion
    UPDATE     int32 = 3  // Tuple modification
    COMMIT_TXN int32 = 4  // Transaction commit
    ABORT_TXN  int32 = 5  // Transaction abort
)
```

**WAL Entry Structure:**
```
[LSN][TransactionID][Type][PrevLSN][...TypeSpecificData...]
```

**Features:**
- **LSN Chaining** - Linked list per transaction
- **Tuple Before/After Images** - Full tuple state
- **PageID Tracking** - Know which pages changed
- **Serialization** - Binary log format
- **Log Reader** - Recovery and debugging

**WAL Writer:**
- Append-only log file
- Fsync for durability
- Log sequence number (LSN) generation
- Transaction boundary markers

**Current Status:**
- ✅ Log writing implemented
- ✅ Log reading implemented
- ✅ Log serialization/deserialization
- ⚠️ **Recovery algorithm NOT fully implemented**

**Missing:**
- REDO phase (replay committed transactions)
- UNDO phase (rollback uncommitted transactions)
- Checkpoint management
- Recovery manager component

---

### ✅ Data Types & Tuples

#### Type System (`pkg/types/`)

**Supported Types:**

1. **IntField** (`integer.go`)
   - 64-bit signed integer
   - Fixed size: 8 bytes
   - Arithmetic and comparison operations

2. **Float64Field** (`float.go`)
   - IEEE 754 double precision
   - Fixed size: 8 bytes
   - Handles NaN and infinity

3. **StringField** (`string.go`)
   - Variable-length strings
   - Max size: configurable (default 128 bytes)
   - UTF-8 encoding
   - Padding for fixed-length storage

4. **BoolField** (`boolean.go`)
   - Boolean values (true/false)
   - Fixed size: 1 byte
   - Comparison operations

**Type Interface:**
```go
type Field interface {
    Serialize(w io.Writer) error
    Compare(op Predicate, other Field) (bool, error)
    Equals(other Field) bool
    Hash() uint32
    GetType() Type
}
```

**Operations:**
- Serialization to binary format
- Deserialization from byte streams
- Comparison (=, <, >, <=, >=, !=)
- Hashing for hash joins
- Type equality checking

#### Tuple Management (`pkg/tuple/`)

**TupleDescription** - Schema definition:
```go
type TupleDescription struct {
    fields     []Type
    fieldNames []string
}
```

**Tuple** - Data record:
```go
type Tuple struct {
    TupleDesc *TupleDescription
    RecordID  *TupleRecordID  // Location in heap
    fields    []Field
}
```

**TupleRecordID** - Pointer to heap location:
```go
type TupleRecordID struct {
    PageID   primitives.PageID  // Can be HeapPageID, BTreePageID, etc.
    TupleNum int                // Slot number within page
}
```

**Features:**
- Field access by index or name
- Schema validation
- Tuple comparison
- Iterator utilities
- Deep copy support

---

### ✅ SQL Parser

#### Lexer (`pkg/parser/lexer.go`)
- Tokenization of SQL statements
- Keyword recognition (SELECT, FROM, WHERE, etc.)
- Identifier parsing
- Literal value parsing (strings, numbers, booleans)
- Operator recognition

#### Parser (`pkg/parser/parser.go`)

**Supported SQL Statements:**

**DDL (Data Definition Language):**
```sql
CREATE TABLE users (
    id INT PRIMARY KEY,
    name VARCHAR(100),
    age INT
);

CREATE INDEX idx_name ON users(name) USING BTREE;

DROP TABLE users;
DROP INDEX idx_name;
```

**DML (Data Manipulation Language):**
```sql
-- Insert
INSERT INTO users VALUES (1, 'Alice', 30);

-- Update
UPDATE users SET age = 31 WHERE id = 1;

-- Delete
DELETE FROM users WHERE age < 18;

-- Select
SELECT id, name FROM users WHERE age >= 18;
```

**Query Features:**
```sql
-- JOINs
SELECT u.name, o.total
FROM users u
JOIN orders o ON u.id = o.user_id;

-- GROUP BY with Aggregates
SELECT department, COUNT(*), AVG(salary)
FROM employees
GROUP BY department;

-- LIMIT and Pagination
SELECT * FROM users ORDER BY created_at DESC LIMIT 10 OFFSET 20;
```

**AST (Abstract Syntax Tree):**
- Structured representation of queries
- Type-safe AST nodes
- Visitor pattern support

**Comprehensive Test Coverage:**
- 34+ test cases in `parser_test.go`
- Edge case handling
- Error recovery

---

### ✅ User Interface & Tools

#### Interactive CLI (`pkg/ui/`)
- **Bubble Tea TUI** - Terminal user interface
- **Colored Output** - Syntax highlighting for results
- **Query History** - Command history navigation
- **Result Formatting** - Pretty-printed tables

**Features:**
```
┌─────────────────────────────┐
│   StoreMy Database Shell    │
└─────────────────────────────┘

storemy> SELECT * FROM users;
┌────┬───────┬─────┐
│ ID │ Name  │ Age │
├────┼───────┼─────┤
│  1 │ Alice │  30 │
│  2 │ Bob   │  25 │
└────┴───────┴─────┘
```

#### Debug Tools (`pkg/debug/`)

1. **Catalog Reader** (`catalog_reader.go`)
   - Inspect system catalog tables
   - View table definitions
   - Check index metadata
   - Statistics viewer

2. **Heap Reader** (`heap_reader.go`)
   - Dump heap page contents
   - Slot pointer inspection
   - Tuple visualization
   - Free space analysis

3. **Log Reader** (`log_reader.go`)
   - WAL inspection
   - Transaction history
   - Log entry parsing
   - Recovery analysis

**Usage:**
```bash
# View catalog
storemy-debug catalog --database mydb

# Inspect heap file
storemy-debug heap --file data/table_1.dat --page 5

# Read WAL
storemy-debug wal --file log/wal.log
```

---

### ✅ Demo & Examples

#### Demo Mode (`pkg/database/demo.go`)
- Sample schema creation
- Pre-populated test data
- Example queries
- Performance demonstrations

**Sample Schema:**
```sql
CREATE TABLE employees (
    id INT PRIMARY KEY,
    name VARCHAR(100),
    department VARCHAR(50),
    salary FLOAT
);

CREATE INDEX idx_dept ON employees(department);
```

#### Database Initialization
- Automatic directory structure creation
- Catalog table setup
- Index file initialization
- WAL log creation

#### SQL File Import
- Batch query execution
- Transaction support
- Error handling
- Progress reporting

---

## Architecture Analysis

### High-Level Architecture

```
┌─────────────────────────────────────────────────┐
│          Application / CLI Interface            │
└────────────────────┬────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────┐
│         Database API (ExecuteQuery)             │
└────────────────────┬────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
┌───────▼────────┐      ┌─────────▼────────┐
│  Query Parser  │      │  Catalog Manager │
│   (SQL → AST)  │      │   (Metadata)     │
└───────┬────────┘      └─────────┬────────┘
        │                         │
┌───────▼─────────────────────────▼────────┐
│         Query Planner & Optimizer        │
│  (AST → Execution Plan → Operator Tree)  │
└───────┬──────────────────────────────────┘
        │
┌───────▼────────────────────────────────┐
│      Execution Engine                  │
│  (Iterator Pattern - Volcano Model)    │
│                                         │
│  ┌──────────┐  ┌──────────┐           │
│  │SeqScan   │  │IndexScan │  ...      │
│  └────┬─────┘  └────┬─────┘           │
│       │             │                  │
│  ┌────▼─────────────▼─────┐           │
│  │    Join Operators      │           │
│  └────┬───────────────────┘           │
│       │                                │
│  ┌────▼───────────┐                   │
│  │  Aggregation   │                   │
│  └────┬───────────┘                   │
│       │                                │
│  ┌────▼───────────┐                   │
│  │  Projection    │                   │
│  └────────────────┘                   │
└───────┬────────────────────────────────┘
        │
┌───────▼──────────────────────────────────┐
│           Page Store Manager             │
│  (Buffer Pool + Lock Manager + WAL)      │
│                                           │
│  ┌──────────┐  ┌───────────┐  ┌──────┐  │
│  │BufferPool│  │LockManager│  │ WAL  │  │
│  └────┬─────┘  └─────┬─────┘  └──┬───┘  │
│       │              │            │      │
│       └──────────────┴────────────┘      │
└───────┬───────────────────────────────────┘
        │
┌───────▼─────────────────────────────────┐
│         Storage Layer                   │
│                                         │
│  ┌──────────┐  ┌─────────────────────┐ │
│  │HeapFiles │  │  Index Files        │ │
│  │(Slotted  │  │  ┌────────┐         │ │
│  │ Pages)   │  │  │B+Tree  │  Hash   │ │
│  └──────────┘  │  └────────┘  Index  │ │
│                └─────────────────────┘ │
└─────────────────────────────────────────┘
```

### Component Interaction Flow

#### Query Execution Flow:
```
1. SQL Query
   ↓
2. Lexer (Tokenization)
   ↓
3. Parser (AST Construction)
   ↓
4. Planner (Logical Plan)
   ↓
5. Optimizer (Physical Plan)
   ↓
6. Execution (Iterator Pattern)
   ↓
7. Page Store (Buffer Pool + Locking)
   ↓
8. Storage (Heap Files + Indexes)
   ↓
9. Results (Tuple Stream)
```

#### Transaction Flow:
```
BEGIN
  ↓
Transaction Context Created (ACTIVE state)
  ↓
Query Execution
  ├─ Acquire Locks (via LockManager)
  ├─ Read Pages (via BufferPool)
  ├─ Write WAL Records (via WAL)
  └─ Mark Pages Dirty (with TransactionID)
  ↓
COMMIT/ABORT
  ├─ Write COMMIT/ABORT log record
  ├─ Flush dirty pages (FORCE policy)
  ├─ Release all locks (Strict 2PL)
  └─ Update transaction state (COMMITTED/ABORTED)
```

### Directory Structure Deep Dive

```
StoreMy/
│
├── cmd/                          # Executable binaries
│   ├── storemy/                  # Main database CLI
│   └── debug/                    # Debug utilities
│
├── pkg/                          # Core packages
│   │
│   ├── catalog/                  # Metadata Management
│   │   ├── catalog_tables.go    # CATALOG_TABLES definition
│   │   ├── catalog_columns.go   # CATALOG_COLUMNS definition
│   │   ├── catalog_statistics.go # CATALOG_STATISTICS definition
│   │   ├── catalog_indexes.go   # CATALOG_INDEXES definition
│   │   └── catalogmanager/      # Catalog operations
│   │       ├── catalog_manager.go
│   │       └── catalog_manager_test.go
│   │
│   ├── concurrency/              # Concurrency Control
│   │   ├── lock/                 # Lock Manager
│   │   │   ├── lock_manager.go  # 2PL implementation
│   │   │   └── deadlock.go      # Deadlock detection
│   │   └── transaction/          # Transaction Context
│   │       ├── context.go       # Transaction lifecycle
│   │       └── permissions.go   # Page access permissions
│   │
│   ├── database/                 # Database Coordinator
│   │   ├── database.go          # Main DB struct
│   │   ├── query_executor.go   # Query execution
│   │   └── demo.go              # Demo data loader
│   │
│   ├── execution/                # Query Execution Engine
│   │   └── operators/           # Physical operators
│   │       ├── aggregate.go     # GROUP BY + aggregates
│   │       ├── delete.go        # DELETE execution
│   │       ├── filter.go        # WHERE clause
│   │       ├── hash_join.go     # Hash join algorithm
│   │       ├── indexscan.go     # Index-based scan
│   │       ├── insert.go        # INSERT execution
│   │       ├── limit.go         # LIMIT/OFFSET
│   │       ├── nested_loop_join.go # Nested loop join
│   │       ├── project.go       # Projection (SELECT list)
│   │       ├── seqscan.go       # Sequential scan
│   │       └── sort_merge_join.go # Sort-merge join
│   │
│   ├── indexmanager/             # Index Lifecycle Manager
│   │   └── index_manager.go     # Index coordination
│   │
│   ├── log/                      # Write-Ahead Logging
│   │   ├── log_writer.go        # WAL writing
│   │   ├── log_reader.go        # WAL reading
│   │   └── log_record.go        # Log entry types
│   │
│   ├── memory/                   # Buffer Pool
│   │   ├── cache.go             # LRU page cache
│   │   └── buffer_pool.go       # Buffer management
│   │
│   ├── parser/                   # SQL Parser
│   │   ├── lexer.go             # Tokenization
│   │   ├── parser.go            # Parsing logic
│   │   ├── ast.go               # AST definitions
│   │   └── parser_test.go       # Parser tests (34+ tests)
│   │
│   ├── planner/                  # Query Planner
│   │   ├── planner.go           # Query planning
│   │   ├── ddl.go               # DDL planning
│   │   ├── dml.go               # DML planning
│   │   └── select.go            # SELECT planning
│   │
│   ├── primitives/               # Core Primitives
│   │   ├── page_id.go           # PageID interface
│   │   ├── predicate.go         # Comparison operators
│   │   └── transaction.go       # TransactionID
│   │
│   ├── storage/                  # Storage Layer
│   │   ├── heap/                # Heap Files
│   │   │   ├── page.go          # Slotted heap page
│   │   │   ├── page_id.go       # HeapPageID
│   │   │   ├── heap_file.go     # Heap file management
│   │   │   └── page_test.go     # Heap page tests
│   │   │
│   │   ├── index/               # Index Structures
│   │   │   ├── index.go         # Index interface
│   │   │   ├── btree/           # B+Tree Index
│   │   │   │   ├── btree.go     # B+Tree implementation
│   │   │   │   ├── btree_page.go # B+Tree page
│   │   │   │   ├── btree_page_id.go
│   │   │   │   ├── btree_file.go
│   │   │   │   └── btree_page_test.go (11 tests - ✅ Created)
│   │   │   │
│   │   │   └── hash/            # Hash Index
│   │   │       ├── hash_index.go # Hash implementation
│   │   │       ├── hash_page.go  # Hash bucket page
│   │   │       ├── hash_page_id.go
│   │   │       ├── hash_file.go
│   │   │       └── hash_page_test.go (15 tests - ✅ Created)
│   │   │
│   │   └── page/                # Page Interface
│   │       └── page.go          # Common page interface
│   │
│   ├── tuple/                    # Tuple Management
│   │   ├── tuple.go             # Tuple struct
│   │   ├── tuple_desc.go        # Schema description
│   │   └── record_id.go         # TupleRecordID
│   │
│   ├── types/                    # Data Types
│   │   ├── type.go              # Type interface
│   │   ├── integer.go           # IntField
│   │   ├── float.go             # Float64Field
│   │   ├── string.go            # StringField
│   │   ├── boolean.go           # BoolField
│   │   └── *_test.go            # Type tests
│   │
│   ├── ui/                       # User Interface
│   │   ├── cli.go               # CLI interface
│   │   └── formatter.go         # Result formatting
│   │
│   ├── debug/                    # Debug Tools
│   │   ├── catalog_reader.go   # Catalog inspector
│   │   ├── heap_reader.go      # Heap page inspector
│   │   └── log_reader.go       # WAL inspector
│   │
│   └── utils/                    # Utilities
│       └── functools/           # Functional helpers
│
├── data/                         # Data Directory (runtime)
│   ├── heap/                    # Heap files
│   ├── index/                   # Index files
│   └── catalog/                 # Catalog files
│
├── log/                          # Log Directory (runtime)
│   └── wal.log                  # Write-ahead log
│
├── go.mod                        # Go module definition
├── go.sum                        # Dependency checksums
└── README.md                     # Project documentation
```

### Key Design Patterns

#### 1. Iterator Pattern (Volcano Model)
Used throughout the execution engine:
```go
type Operator interface {
    Open() error              // Initialize operator
    Next() (*Tuple, error)    // Get next tuple (pull-based)
    Close() error             // Cleanup
}
```

**Benefits:**
- Memory-efficient streaming
- Pipeline parallelism potential
- Composable operators
- Lazy evaluation

#### 2. Page Interface Pattern
Unified interface for all page types:
```go
type Page interface {
    GetID() PageID
    IsDirty() *TransactionID
    MarkDirty(bool, *TransactionID)
    GetPageData() []byte
    GetBeforeImage() Page
}
```

**Benefits:**
- Polymorphic page handling
- Buffer pool simplification
- Transaction uniformity

#### 3. Slotted Page Structure
PostgreSQL-inspired design:
```
[Header: Slot Pointers] [Free Space] [Tuple Data ←]
```

**Benefits:**
- Stable RecordIDs even after compaction
- Variable-length tuple support
- Efficient space reclamation
- Index stability

#### 4. Two-Phase Locking (2PL)
Concurrency control pattern:
```
Growing Phase: Acquire locks
Shrinking Phase: Release locks (at commit/abort)
```

**Benefits:**
- Serializability guarantee
- Simple deadlock detection
- Thread-safe operations

#### 5. Write-Ahead Logging (WAL)
Durability pattern:
```
1. Write log record
2. Flush log to disk
3. Modify page
4. Eventually flush page
```

**Benefits:**
- Crash recovery
- Transaction atomicity
- Reduced disk I/O

---

## What's Missing

### 🔴 Critical Missing Features

#### 1. Crash Recovery Implementation
**Status:** ⚠️ Partially Implemented

**What Exists:**
- ✅ WAL writing
- ✅ Log record serialization
- ✅ Log reading

**What's Missing:**
- ❌ Recovery Manager component
- ❌ REDO phase (replay committed transactions)
- ❌ UNDO phase (rollback uncommitted transactions)
- ❌ Checkpoint mechanism
- ❌ Log truncation after recovery

**Files to Create:**
- `pkg/recovery/recovery_manager.go`
- `pkg/recovery/redo.go`
- `pkg/recovery/undo.go`
- `pkg/recovery/checkpoint.go`

**Recovery Algorithm:**
```
1. Analysis Phase:
   - Scan WAL from last checkpoint
   - Identify uncommitted transactions
   - Build dirty page table

2. REDO Phase:
   - Replay all operations (even aborted)
   - Restore database to crash state

3. UNDO Phase:
   - Rollback uncommitted transactions
   - Use before-images to restore

4. Cleanup:
   - Write checkpoint record
   - Truncate old log entries
```

---

#### 2. Query Optimization Improvements
**Status:** ⚠️ Basic Implementation

**What Exists:**
- ✅ Simple cost model
- ✅ Index selection
- ✅ Join ordering (basic)

**What's Missing:**
- ❌ Histogram-based statistics
- ❌ Cardinality estimation
- ❌ Selectivity estimation
- ❌ Multi-column statistics
- ❌ Query plan caching
- ❌ Dynamic programming for join ordering

**Improvements Needed:**

**A. Statistics Collection:**
```sql
-- Command to collect statistics
ANALYZE table_name;

-- Histogram storage
CREATE TABLE CATALOG_HISTOGRAMS (
    table_id INT,
    column_name VARCHAR,
    bucket_id INT,
    bucket_min VARCHAR,
    bucket_max VARCHAR,
    bucket_count INT
);
```

**B. Cardinality Estimation:**
```go
// Estimate result size for WHERE clause
func EstimateCardinality(table Table, predicate Predicate) int {
    // Use histograms + selectivity factors
    baseCardinality := table.GetStatistics().NumTuples
    selectivity := EstimateSelectivity(predicate)
    return int(float64(baseCardinality) * selectivity)
}
```

**C. Cost Model Improvements:**
```go
type CostEstimate struct {
    IOCost       float64  // Disk I/O cost
    CPUCost      float64  // CPU processing cost
    MemoryCost   float64  // Memory usage
    NetworkCost  float64  // Network transfer (for distributed)
    TotalCost    float64  // Weighted sum
    Cardinality  int      // Estimated result size
}
```

---

### 🟠 High Priority Missing Features

#### 3. Advanced SQL Features

##### A. DISTINCT Support
**Status:** ❌ Not Implemented

```sql
SELECT DISTINCT city FROM users;
```

**Implementation:**
- Create `pkg/execution/operators/distinct.go`
- Hash-based deduplication
- Memory-efficient implementation

```go
type DistinctOperator struct {
    child    Operator
    seenKeys map[uint32]bool  // Hash of seen tuples
}

func (d *DistinctOperator) Next() (*Tuple, error) {
    for {
        tuple, err := d.child.Next()
        if err != nil {
            return nil, err
        }

        hash := tuple.Hash()
        if !d.seenKeys[hash] {
            d.seenKeys[hash] = true
            return tuple, nil
        }
        // Skip duplicate, continue loop
    }
}
```

---

##### B. Subquery Support
**Status:** ❌ Not Implemented

**Types of Subqueries:**

1. **Scalar Subqueries** (single value):
```sql
SELECT * FROM users
WHERE age > (SELECT AVG(age) FROM users);
```

2. **IN/NOT IN Subqueries** (set membership):
```sql
SELECT * FROM users
WHERE department_id IN (SELECT id FROM departments WHERE active = true);
```

3. **EXISTS/NOT EXISTS** (existence check):
```sql
SELECT * FROM users u
WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id);
```

4. **Correlated Subqueries** (refers to outer query):
```sql
SELECT u.name,
       (SELECT COUNT(*) FROM orders o WHERE o.user_id = u.id) as order_count
FROM users u;
```

**Implementation Strategy:**
- Extend parser to recognize subquery syntax
- AST nodes for subquery expressions
- Subquery execution operators
- Correlation variable handling

**Files to Create:**
- `pkg/execution/operators/subquery.go`
- `pkg/parser/subquery.go`

---

##### C. UNION/INTERSECT/EXCEPT
**Status:** ❌ Not Implemented

```sql
-- UNION (combine, remove duplicates)
SELECT id FROM users UNION SELECT id FROM admins;

-- UNION ALL (combine, keep duplicates)
SELECT id FROM users UNION ALL SELECT id FROM admins;

-- INTERSECT (common rows)
SELECT email FROM users INTERSECT SELECT email FROM newsletter_subscribers;

-- EXCEPT (difference)
SELECT id FROM all_users EXCEPT SELECT id FROM active_users;
```

**Implementation:**
- Set operation operators
- Schema compatibility checking
- Deduplication for UNION/INTERSECT

**Files to Create:**
- `pkg/execution/operators/union.go`
- `pkg/execution/operators/intersect.go`
- `pkg/execution/operators/except.go`

---

#### 4. Constraints & Data Integrity

##### A. UNIQUE Constraints
**Status:** ❌ Not Implemented

```sql
CREATE TABLE users (
    id INT PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    username VARCHAR(50) UNIQUE
);
```

**Implementation:**
- Automatic unique index creation
- INSERT/UPDATE validation
- Constraint metadata in catalog

**Catalog Extension:**
```sql
CREATE TABLE CATALOG_CONSTRAINTS (
    constraint_id INT PRIMARY KEY,
    constraint_name VARCHAR,
    table_id INT,
    constraint_type VARCHAR,  -- UNIQUE, CHECK, FOREIGN_KEY
    column_names VARCHAR,      -- Comma-separated
    check_expression VARCHAR   -- For CHECK constraints
);
```

---

##### B. CHECK Constraints
**Status:** ❌ Not Implemented

```sql
CREATE TABLE employees (
    id INT PRIMARY KEY,
    age INT CHECK (age >= 18 AND age <= 100),
    salary FLOAT CHECK (salary > 0),
    email VARCHAR CHECK (email LIKE '%@%.%')
);
```

**Implementation:**
- Parse CHECK expressions
- Evaluate on INSERT/UPDATE
- Store constraint definitions

---

##### C. DEFAULT Values
**Status:** ❌ Not Implemented

```sql
CREATE TABLE orders (
    id INT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR DEFAULT 'pending',
    quantity INT DEFAULT 1
);
```

**Implementation:**
- Store default values in column metadata
- Apply defaults when column omitted in INSERT
- Function evaluation (CURRENT_TIMESTAMP, etc.)

---

##### D. Foreign Keys
**Status:** ❌ Not Implemented

```sql
CREATE TABLE orders (
    id INT PRIMARY KEY,
    user_id INT REFERENCES users(id),
    product_id INT REFERENCES products(id) ON DELETE CASCADE
);
```

**Features Needed:**
- Referential integrity checking
- Cascade operations (CASCADE, SET NULL, SET DEFAULT, RESTRICT)
- Foreign key index for performance

**Implementation:**
- FK validation on INSERT/UPDATE
- Cascade trigger on DELETE/UPDATE of parent
- Deadlock prevention strategy

---

### 🟡 Medium Priority Missing Features

#### 5. MVCC (Multi-Version Concurrency Control)
**Status:** ❌ Not Implemented

**Current:** Strict 2PL with locks
**Desired:** MVCC for better concurrency

**MVCC Design:**

**Tuple Versioning:**
```go
type HeapTuple struct {
    // Existing fields...
    Xmin *TransactionID  // Transaction that created this version
    Xmax *TransactionID  // Transaction that deleted/updated this version

    // Version chain
    NextVersion *TupleRecordID  // Points to newer version
}
```

**Snapshot Isolation:**
```go
type Snapshot struct {
    SnapshotID  int64
    ActiveTxns  []TransactionID  // Transactions active at snapshot time
    MinTxnID    TransactionID    // Oldest active transaction
    MaxTxnID    TransactionID    // Newest transaction at snapshot
}

func (s *Snapshot) IsVisible(tuple *HeapTuple, currentTxn *Transaction) bool {
    // Visibility rules:
    // 1. Created by current transaction → visible
    // 2. Created by committed transaction before snapshot → visible
    // 3. Created by active transaction → not visible
    // 4. Deleted by transaction after snapshot → visible
}
```

**Vacuum Process:**
- Remove old tuple versions
- Reclaim space
- Update indexes

**Benefits:**
- Read operations don't block writes
- Write operations don't block reads
- Better throughput for read-heavy workloads

**Challenges:**
- Increased storage for versions
- Complex visibility logic
- Vacuum overhead

---

#### 6. Multiple Isolation Levels
**Status:** ❌ Not Implemented (only Serializable)

**Desired Isolation Levels:**

```sql
SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
SET TRANSACTION ISOLATION LEVEL REPEATABLE READ;
SET TRANSACTION ISOLATION LEVEL SERIALIZABLE;
```

**Implementation:**

| Level | Read Locks | Write Locks | Phenomena Allowed |
|-------|------------|-------------|-------------------|
| READ UNCOMMITTED | None | X (until commit) | Dirty read, Non-repeatable, Phantom |
| READ COMMITTED | S (release immediately) | X (until commit) | Non-repeatable, Phantom |
| REPEATABLE READ | S (until commit) | X (until commit) | Phantom reads |
| SERIALIZABLE | S (until commit) | X (until commit) | None |

**Files to Modify:**
- `pkg/concurrency/transaction/context.go` (add isolation level)
- `pkg/concurrency/lock/lock_manager.go` (level-aware locking)

---

#### 7. Query Plan Caching
**Status:** ❌ Not Implemented

**Motivation:**
- Parsing is expensive (lexing, syntax analysis)
- Planning is expensive (optimization, cost estimation)
- Same query executed repeatedly

**Design:**

```go
type PlanCache struct {
    cache map[string]*CachedPlan  // SQL hash → plan
    lru   *LRUList
    mu    sync.RWMutex
}

type CachedPlan struct {
    SQL             string
    Plan            *PhysicalPlan
    ParamTypes      []Type
    CreatedAt       time.Time
    LastUsed        time.Time
    ExecutionCount  int
    AvgExecutionMs  float64
}

// Cache key = SQL text hash
func (pc *PlanCache) Get(sql string) (*PhysicalPlan, bool) {
    // Check cache
    // Update LRU
    // Return plan
}
```

**Invalidation:**
- Schema changes (DROP TABLE, ALTER TABLE)
- Index changes (CREATE INDEX, DROP INDEX)
- Statistics updates (ANALYZE)
- Cache size limits

**Parameterized Queries:**
```sql
-- Prepare once
PREPARE get_user AS SELECT * FROM users WHERE id = $1;

-- Execute many times
EXECUTE get_user(1);
EXECUTE get_user(2);
EXECUTE get_user(3);
```

---

#### 8. Parallel Query Execution
**Status:** ❌ Not Implemented

**Parallel Operators:**

1. **Parallel Sequential Scan:**
```go
type ParallelSeqScan struct {
    table       Table
    numWorkers  int
    pageRanges  []PageRange  // Partition pages among workers
    resultChan  chan *Tuple
}
```

2. **Parallel Hash Join:**
- Partition both inputs by hash
- Each worker joins matching partitions
- Final merge step

3. **Parallel Aggregation:**
- Partial aggregates per worker
- Final combine step

**Implementation:**
- Worker pool management
- Page range partitioning
- Result merging
- Resource limits (max workers)

**Benefits:**
- Utilize multi-core CPUs
- Faster query execution
- Better hardware utilization

**Challenges:**
- Overhead for small queries
- Lock contention
- Memory management

---

### 🟢 Nice-to-Have Features

#### 9. Views
**Status:** ❌ Not Implemented

```sql
CREATE VIEW active_users AS
SELECT id, name, email
FROM users
WHERE active = true;

-- Use view like a table
SELECT * FROM active_users WHERE email LIKE '%@example.com';
```

**Implementation:**

**A. Regular Views (Query Rewriting):**
```go
type View struct {
    Name       string
    Definition string  // SQL query
    Columns    []string
}

// When querying view, rewrite query to use base tables
SELECT * FROM active_users WHERE age > 18
→
SELECT id, name, email FROM users
WHERE active = true AND age > 18
```

**B. Materialized Views (Cached Results):**
```sql
CREATE MATERIALIZED VIEW user_stats AS
SELECT department, COUNT(*) as count, AVG(salary) as avg_salary
FROM employees
GROUP BY department;

-- Refresh cached data
REFRESH MATERIALIZED VIEW user_stats;
```

**Catalog Extension:**
```sql
CREATE TABLE CATALOG_VIEWS (
    view_id INT PRIMARY KEY,
    view_name VARCHAR,
    view_definition TEXT,
    is_materialized BOOL,
    last_refreshed TIMESTAMP
);
```

---

#### 10. Stored Procedures & UDFs
**Status:** ❌ Not Implemented

```sql
-- User-Defined Function
CREATE FUNCTION calculate_discount(price FLOAT, discount_percent FLOAT)
RETURNS FLOAT AS $$
BEGIN
    RETURN price * (1 - discount_percent / 100);
END;
$$ LANGUAGE plsql;

-- Usage
SELECT product_name, calculate_discount(price, 10) as discounted_price
FROM products;

-- Stored Procedure
CREATE PROCEDURE apply_bulk_discount(
    category VARCHAR,
    discount FLOAT
) AS $$
BEGIN
    UPDATE products
    SET price = price * (1 - discount / 100)
    WHERE category = category;
END;
$$ LANGUAGE plsql;

-- Execute
CALL apply_bulk_discount('electronics', 15);
```

**Implementation:**

**Components Needed:**
1. **PL/SQL Parser** - Parse procedure language
2. **Execution Context** - Variables, control flow
3. **Function Registry** - Store UDF definitions
4. **Catalog Storage** - Persist function definitions

**Files to Create:**
- `pkg/procedure/` (new package)
- `pkg/procedure/parser.go`
- `pkg/procedure/executor.go`
- `pkg/procedure/registry.go`

---

#### 11. Triggers
**Status:** ❌ Not Implemented

```sql
-- Trigger definition
CREATE TRIGGER update_modified_timestamp
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp_column();

-- Audit log trigger
CREATE TRIGGER audit_user_changes
    AFTER INSERT OR UPDATE OR DELETE ON users
    FOR EACH ROW
    EXECUTE FUNCTION log_user_change();
```

**Trigger Types:**
- **BEFORE** triggers: Modify data before operation
- **AFTER** triggers: Side effects after operation
- **FOR EACH ROW**: Execute per tuple
- **FOR EACH STATEMENT**: Execute once per statement

**Implementation:**

```go
type Trigger struct {
    Name       string
    Table      string
    Timing     TriggerTiming     // BEFORE, AFTER
    Event      TriggerEvent      // INSERT, UPDATE, DELETE
    Granularity TriggerLevel     // ROW, STATEMENT
    Function   string            // Function to execute
    Enabled    bool
}

type TriggerManager struct {
    triggers map[string][]*Trigger  // table → triggers
}

// Execute in DML operators
func (op *InsertOperator) Next() (*Tuple, error) {
    // 1. Execute BEFORE triggers
    tuple = triggerManager.ExecuteBefore(INSERT, tuple)

    // 2. Perform INSERT
    err := table.InsertTuple(tuple)

    // 3. Execute AFTER triggers
    triggerManager.ExecuteAfter(INSERT, tuple)
}
```

---

#### 12. Full-Text Search
**Status:** ❌ Not Implemented

```sql
-- Create text search index
CREATE INDEX users_search ON users USING GIN(to_tsvector(bio));

-- Search query
SELECT * FROM users
WHERE to_tsvector(bio) @@ to_tsquery('database & postgresql');

-- Ranked results
SELECT *, ts_rank(to_tsvector(bio), to_tsquery('database')) as rank
FROM users
WHERE to_tsvector(bio) @@ to_tsquery('database')
ORDER BY rank DESC;
```

**Features:**
- **Inverted Index** for text
- **Stemming** (running → run)
- **Stop Words** (the, a, an)
- **Relevance Scoring**
- **Phrase Search**
- **Fuzzy Matching**

**Implementation:**

```go
type GINIndex struct {
    // Inverted index: term → list of documents
    index map[string][]DocumentRef
}

type DocumentRef struct {
    TupleRID  *TupleRecordID
    Positions []int  // Where term appears in document
    Frequency int    // How many times term appears
}

// Text search functions
func to_tsvector(text string) *TSVector {
    // 1. Tokenize
    // 2. Normalize (lowercase)
    // 3. Remove stop words
    // 4. Stem
    // 5. Build vector
}

func to_tsquery(query string) *TSQuery {
    // Parse query: "database & (postgresql | mysql)"
}

func Match(vector *TSVector, query *TSQuery) bool {
    // Evaluate query against vector
}
```

---

#### 13. Client/Server Architecture
**Status:** ❌ Not Implemented (single-process only)

**Current:** Embedded database (same process as application)
**Desired:** Client/server model like PostgreSQL

**Architecture:**

```
┌──────────┐                    ┌──────────┐
│  Client  │ ←─── TCP/TLS ───→ │  Server  │
│   CLI    │                    │ (daemon) │
└──────────┘                    └──────────┘
                                     ↓
                                Database Engine
```

**Components:**

**A. Network Protocol:**
```go
// Wire protocol messages
type Message interface {
    Serialize() []byte
    Deserialize([]byte) error
}

type QueryMessage struct {
    SQL       string
    Params    []interface{}
}

type ResultMessage struct {
    Columns   []string
    Rows      [][]*Field
    RowCount  int
    Error     string
}
```

**B. TCP Server:**
```go
type DatabaseServer struct {
    listener  net.Listener
    database  *Database
    clients   map[string]*ClientConnection
    config    *ServerConfig
}

func (s *DatabaseServer) Start() error {
    for {
        conn, err := s.listener.Accept()
        if err != nil {
            continue
        }
        go s.handleClient(conn)
    }
}
```

**C. Client Library:**
```go
type Client struct {
    conn net.Conn
}

func (c *Client) Query(sql string) (*ResultSet, error) {
    // 1. Send QueryMessage
    // 2. Receive ResultMessage
    // 3. Parse result
}
```

**Benefits:**
- Multi-user support
- Remote access
- Resource isolation
- Centralized management

**Files to Create:**
- `pkg/server/` (new package)
- `pkg/protocol/` (new package)
- `cmd/storemy-server/`
- `cmd/storemy-client/`

---

#### 14. Replication
**Status:** ❌ Not Implemented

**Replication Types:**

**A. Logical Replication:**
- Replicate SQL statements or logical changes
- Replay WAL on replicas

**B. Physical Replication:**
- Byte-level page replication
- Faster but less flexible

**Architecture:**
```
┌─────────┐                    ┌─────────┐
│ Primary │ ──── WAL Stream ──→│ Replica │
│ (R/W)   │                    │   (R)   │
└─────────┘                    └─────────┘
```

**Implementation:**

```go
type ReplicationManager struct {
    primary  *Database
    replicas []*ReplicaConnection
    walQueue chan *WALRecord
}

func (rm *ReplicationManager) StreamWAL() {
    for record := range rm.walQueue {
        // Send to all replicas
        for _, replica := range rm.replicas {
            replica.Send(record)
        }
    }
}

type ReplicaConnection struct {
    conn      net.Conn
    lastLSN   LSN  // Last applied log sequence number
}
```

**Features:**
- **Streaming Replication:** Real-time WAL streaming
- **Synchronous Mode:** Wait for replica ACK before commit
- **Asynchronous Mode:** Don't wait (better performance, risk data loss)
- **Failover:** Promote replica to primary

---

#### 15. Backup & Restore
**Status:** ❌ Not Implemented

**Backup Types:**

**A. Full Backup (Logical):**
```bash
storemy backup --database mydb --output backup.sql

# Creates SQL file:
CREATE TABLE users (...);
INSERT INTO users VALUES (...);
...
```

**B. Binary Backup (Physical):**
```bash
storemy backup --binary --database mydb --output backup.tar.gz

# Archives:
- data/heap/*
- data/index/*
- data/catalog/*
- log/wal.log
```

**C. Incremental Backup:**
```bash
# Base backup
storemy backup --base --database mydb --output base.tar.gz

# Incremental (only changes since base)
storemy backup --incremental --database mydb --output incr_001.tar.gz
```

**Point-in-Time Recovery (PITR):**
```bash
# Restore to specific timestamp
storemy restore --database mydb \
                --backup base.tar.gz \
                --until "2024-10-15 14:30:00"
```

**Implementation:**

```go
type BackupManager struct {
    database *Database
}

func (bm *BackupManager) FullBackup(outputPath string) error {
    // 1. Start transaction (read-only)
    // 2. Iterate all tables
    // 3. Generate CREATE TABLE statements
    // 4. Generate INSERT statements
    // 5. Write to output file
}

func (bm *BackupManager) BinaryBackup(outputPath string) error {
    // 1. Take checkpoint (flush all pages)
    // 2. Copy all data files
    // 3. Copy WAL
    // 4. Create manifest
}

func (bm *BackupManager) Restore(backupPath string) error {
    // 1. Clear database directory
    // 2. Extract backup
    // 3. Initialize database
    // 4. Run recovery if needed
}
```

---

### 📋 Other Missing Features

#### 16. Date/Time Types
**Status:** ❌ Not Implemented

**Current Types:** INT, FLOAT, VARCHAR, BOOL
**Missing:** DATE, TIME, TIMESTAMP, INTERVAL

```sql
CREATE TABLE events (
    id INT PRIMARY KEY,
    event_date DATE,
    event_time TIME,
    created_at TIMESTAMP,
    duration INTERVAL
);

-- Date arithmetic
SELECT * FROM events
WHERE event_date >= CURRENT_DATE - INTERVAL '7 days';
```

---

#### 17. Composite/Array Types
**Status:** ❌ Not Implemented

```sql
-- Array type
CREATE TABLE users (
    id INT PRIMARY KEY,
    tags VARCHAR[]  -- Array of strings
);

INSERT INTO users VALUES (1, ARRAY['admin', 'developer']);

-- Composite type
CREATE TYPE address AS (
    street VARCHAR,
    city VARCHAR,
    zip VARCHAR
);

CREATE TABLE users (
    id INT PRIMARY KEY,
    home_address address
);
```

---

#### 18. JSON Support
**Status:** ❌ Not Implemented

```sql
CREATE TABLE products (
    id INT PRIMARY KEY,
    metadata JSON
);

INSERT INTO products VALUES (
    1,
    '{"brand": "Acme", "specs": {"weight": 1.5, "color": "blue"}}'
);

-- JSON queries
SELECT * FROM products
WHERE metadata->>'brand' = 'Acme';
```

---

#### 19. Window Functions
**Status:** ❌ Not Implemented

```sql
-- Running total
SELECT
    order_id,
    amount,
    SUM(amount) OVER (ORDER BY order_date) as running_total
FROM orders;

-- Ranking
SELECT
    name,
    salary,
    RANK() OVER (PARTITION BY department ORDER BY salary DESC) as rank
FROM employees;
```

---

#### 20. CTEs (Common Table Expressions)
**Status:** ❌ Not Implemented

```sql
-- Non-recursive CTE
WITH high_value_customers AS (
    SELECT customer_id, SUM(amount) as total
    FROM orders
    GROUP BY customer_id
    HAVING SUM(amount) > 10000
)
SELECT c.name, hvc.total
FROM customers c
JOIN high_value_customers hvc ON c.id = hvc.customer_id;

-- Recursive CTE (org chart)
WITH RECURSIVE org_chart AS (
    SELECT id, name, manager_id, 1 as level
    FROM employees
    WHERE manager_id IS NULL

    UNION ALL

    SELECT e.id, e.name, e.manager_id, oc.level + 1
    FROM employees e
    JOIN org_chart oc ON e.manager_id = oc.id
)
SELECT * FROM org_chart;
```

---

#### 21. Explain Plan
**Status:** ⚠️ Partially Implemented (plan exists but no EXPLAIN command)

```sql
EXPLAIN SELECT * FROM users WHERE age > 18;

-- Output:
-- Seq Scan on users (cost=0.00..50.00 rows=500)
--   Filter: (age > 18)

EXPLAIN ANALYZE SELECT ...;
-- Shows actual execution statistics
```

---

#### 22. Transaction Savepoints
**Status:** ❌ Not Implemented

```sql
BEGIN;
    INSERT INTO users VALUES (1, 'Alice');
    SAVEPOINT sp1;

    INSERT INTO users VALUES (2, 'Bob');
    SAVEPOINT sp2;

    INSERT INTO users VALUES (3, 'Charlie');

    -- Oops, rollback last insert
    ROLLBACK TO sp2;

    -- But keep first two inserts
COMMIT;
```

---

#### 23. Bulk Load Utilities
**Status:** ❌ Not Implemented

```sql
-- Fast bulk load (bypass WAL, no indexes until end)
COPY users FROM '/path/to/users.csv' WITH (FORMAT csv, DELIMITER ',');
```

---

#### 24. Vacuum & Maintenance
**Status:** ❌ Not Implemented

```sql
-- Reclaim space from deleted tuples
VACUUM table_name;

-- Rebuild indexes, update statistics
VACUUM FULL table_name;

-- Update statistics only
ANALYZE table_name;
```

---

#### 25. User Authentication & Authorization
**Status:** ❌ Not Implemented

```sql
-- User management
CREATE USER alice WITH PASSWORD 'secret';
CREATE ROLE admin;
GRANT admin TO alice;

-- Permissions
GRANT SELECT, INSERT ON users TO alice;
REVOKE DELETE ON orders FROM bob;
```

---

#### 26. Resource Management
**Status:** ❌ Not Implemented

**Missing:**
- Memory quotas per transaction
- Disk space limits
- Query timeout
- Connection limits
- Rate limiting

---

## Development Roadmap

Based on the analysis above, here's a prioritized roadmap for developing StoreMy:

---

### Phase 1: Reliability & Recovery (4-6 weeks)
**Goal:** Make the database production-ready for crash recovery

#### Week 1-2: Crash Recovery Foundation
- [ ] Create Recovery Manager component
- [ ] Implement REDO algorithm
- [ ] Implement UNDO algorithm
- [ ] Add checkpoint mechanism

**Files to Create:**
```
pkg/recovery/
  ├── recovery_manager.go
  ├── redo.go
  ├── undo.go
  ├── checkpoint.go
  └── recovery_test.go
```

**Test Scenarios:**
- Crash during active transaction
- Crash between commit and page flush
- Multiple concurrent transactions
- Recovery with various checkpoint states

#### Week 3-4: Recovery Testing & Refinement
- [ ] Comprehensive recovery tests
- [ ] Performance benchmarks
- [ ] Log truncation after recovery
- [ ] Recovery documentation

**Acceptance Criteria:**
- Database recovers correctly from crashes
- All committed transactions preserved
- All uncommitted transactions rolled back
- Performance impact < 10% for WAL overhead

---

### Phase 2: Query Optimization (4-6 weeks)
**Goal:** Significantly improve query performance

#### Week 1-2: Statistics Collection
- [ ] Implement ANALYZE command
- [ ] Histogram-based statistics
- [ ] Multi-column statistics
- [ ] Auto-update statistics on DML

**Files to Create:**
```
pkg/catalog/statistics/
  ├── histogram.go
  ├── analyzer.go
  └── selectivity.go

pkg/planner/
  ├── cost_model.go
  └── cardinality_estimator.go
```

#### Week 3-4: Cost Model Improvements
- [ ] Better I/O cost estimation
- [ ] CPU cost modeling
- [ ] Memory cost consideration
- [ ] Selectivity estimation for WHERE clauses

#### Week 5-6: Optimization Algorithms
- [ ] Dynamic programming for join ordering
- [ ] Query plan caching
- [ ] Plan comparison and selection

**Test Data:**
- TPC-H benchmark queries
- Complex multi-join queries
- Performance regression tests

**Acceptance Criteria:**
- 10-100x speedup on complex queries
- Accurate cardinality estimates (within 2x of actual)
- Query plan cache hit rate > 80%

---

### Phase 3: SQL Completeness (6-8 weeks)
**Goal:** Support most common SQL features

#### Week 1-2: DISTINCT & Set Operations
- [ ] Implement DISTINCT operator
- [ ] Implement UNION/UNION ALL
- [ ] Implement INTERSECT
- [ ] Implement EXCEPT

**Files to Create:**
```
pkg/execution/operators/
  ├── distinct.go
  ├── union.go
  ├── intersect.go
  └── except.go
```

#### Week 3-4: Subqueries
- [ ] Parse subquery syntax
- [ ] Scalar subqueries
- [ ] IN/NOT IN subqueries
- [ ] EXISTS/NOT EXISTS
- [ ] Correlated subqueries

**Files to Create:**
```
pkg/execution/operators/
  ├── subquery.go
  └── correlated_subquery.go

pkg/parser/
  └── subquery.go
```

#### Week 5-6: Window Functions (Optional)
- [ ] Parser support for OVER clause
- [ ] PARTITION BY implementation
- [ ] ORDER BY within windows
- [ ] Window aggregate functions (RANK, ROW_NUMBER, etc.)

#### Week 7-8: CTEs (Optional)
- [ ] Non-recursive CTEs
- [ ] Recursive CTEs
- [ ] Multiple CTEs in single query

**Acceptance Criteria:**
- All TPC-H queries execute successfully
- Subquery performance within 2x of manual rewrite
- Window functions produce correct results

---

### Phase 4: Data Integrity (4-6 weeks)
**Goal:** Ensure data quality with constraints

#### Week 1-2: Constraint Infrastructure
- [ ] Extend catalog for constraints
- [ ] Constraint validation framework
- [ ] Error reporting for violations

**Catalog Extension:**
```sql
CREATE TABLE CATALOG_CONSTRAINTS (
    constraint_id INT PRIMARY KEY,
    constraint_name VARCHAR,
    table_id INT,
    constraint_type VARCHAR,
    definition TEXT
);
```

#### Week 3: UNIQUE & CHECK Constraints
- [ ] UNIQUE constraint implementation
- [ ] CHECK constraint implementation
- [ ] Automatic index creation for UNIQUE

#### Week 4: DEFAULT Values
- [ ] DEFAULT value storage in catalog
- [ ] Apply defaults on INSERT
- [ ] Function defaults (CURRENT_TIMESTAMP, etc.)

#### Week 5-6: Foreign Keys
- [ ] Foreign key validation
- [ ] CASCADE operations
- [ ] SET NULL, SET DEFAULT, RESTRICT options

**Test Scenarios:**
- Insert violating constraint
- Update causing constraint violation
- Cascade delete with foreign keys
- Circular foreign key references

**Acceptance Criteria:**
- All constraint violations caught before commit
- Cascade operations work correctly
- Performance impact < 20% for constrained tables

---

### Phase 5: Concurrency & MVCC (8-10 weeks)
**Goal:** Modern concurrency control for better throughput

#### Week 1-2: MVCC Design
- [ ] Design tuple versioning schema
- [ ] Design snapshot structure
- [ ] Design visibility rules

#### Week 3-4: Tuple Versioning
- [ ] Extend heap tuple format (xmin, xmax)
- [ ] Version chain management
- [ ] Update old tuple visibility

**Heap Tuple Changes:**
```go
type HeapTuple struct {
    // Existing fields...
    Xmin        *TransactionID  // Creator
    Xmax        *TransactionID  // Deleter (nil if alive)
    NextVersion *TupleRecordID  // Newer version
}
```

#### Week 5-6: Snapshot Isolation
- [ ] Snapshot creation on transaction start
- [ ] Visibility checker implementation
- [ ] Read-only transaction optimization

#### Week 7-8: Vacuum Process
- [ ] Identify dead tuples
- [ ] Reclaim space from old versions
- [ ] Update indexes after vacuum

#### Week 9-10: Testing & Tuning
- [ ] Concurrent workload testing
- [ ] Performance comparison (2PL vs MVCC)
- [ ] Tuning vacuum parameters

**Acceptance Criteria:**
- Read queries don't block writes
- Write queries don't block reads (except conflicts)
- Throughput improvement > 3x on read-heavy workloads
- Vacuum maintains reasonable space overhead

---

### Phase 6: Advanced Features (8-12 weeks)
**Goal:** Add powerful database features

#### Views (2 weeks)
- [ ] CREATE VIEW syntax
- [ ] Query rewriting for views
- [ ] Materialized views
- [ ] REFRESH MATERIALIZED VIEW

#### Stored Procedures (3-4 weeks)
- [ ] PL/SQL parser
- [ ] Execution context (variables, control flow)
- [ ] Function registry
- [ ] CALL statement

#### Triggers (2-3 weeks)
- [ ] Trigger definition parsing
- [ ] BEFORE/AFTER trigger execution
- [ ] ROW/STATEMENT level triggers
- [ ] Trigger management

#### Full-Text Search (3-4 weeks)
- [ ] GIN index implementation
- [ ] Text processing (tokenization, stemming)
- [ ] to_tsvector and to_tsquery functions
- [ ] Relevance ranking

---

### Phase 7: Client/Server & Distribution (6-8 weeks)
**Goal:** Transform into client/server architecture

#### Week 1-2: Wire Protocol
- [ ] Design binary protocol
- [ ] Message serialization
- [ ] Authentication handshake

#### Week 3-4: TCP Server
- [ ] TCP listener implementation
- [ ] Connection pooling
- [ ] Multi-client support

#### Week 5-6: Client Library
- [ ] Client connection management
- [ ] Query execution API
- [ ] Result set handling

#### Week 7-8: Replication (Optional)
- [ ] WAL streaming
- [ ] Replica management
- [ ] Failover support

---

### Phase 8: Operations & Monitoring (4-6 weeks)
**Goal:** Production operational support

#### Backup & Restore (2 weeks)
- [ ] Logical backup (SQL dump)
- [ ] Binary backup
- [ ] Incremental backup
- [ ] Point-in-time recovery

#### Monitoring (2 weeks)
- [ ] Query performance metrics
- [ ] Slow query log
- [ ] System statistics (cache hit ratio, I/O, etc.)
- [ ] Monitoring API/dashboard

#### Administration (2 weeks)
- [ ] User authentication
- [ ] Role-based access control
- [ ] Permission management
- [ ] Configuration management

---

## Quick Wins

These are small features that can be implemented quickly (1-3 days each) but provide significant value:

### 1. EXPLAIN Command (1 day)
Show query execution plans to users

```sql
EXPLAIN SELECT * FROM users WHERE age > 18;
```

**Implementation:**
- Reuse existing plan structure
- Pretty-print operator tree
- Show cost estimates

---

### 2. TRUNCATE TABLE (1 day)
Fast table clearing without logging individual deletes

```sql
TRUNCATE TABLE logs;  -- Much faster than DELETE FROM logs
```

**Implementation:**
- Delete heap file
- Recreate empty heap file
- Clear all indexes
- Update statistics to zero

---

### 3. SHOW Commands (1 day)
Introspection commands for database objects

```sql
SHOW TABLES;
SHOW INDEXES FROM users;
SHOW COLUMNS FROM users;
```

**Implementation:**
- Query catalog tables
- Format output nicely

---

### 4. Transaction Isolation Level Selection (2 days)
Allow users to choose isolation level

```sql
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
BEGIN;
-- ...
COMMIT;
```

**Implementation:**
- Add isolation level to transaction context
- Modify lock acquisition based on level

---

### 5. Auto-Increment Columns (2 days)
SERIAL type for auto-incrementing primary keys

```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,  -- Auto-incrementing
    name VARCHAR(100)
);

INSERT INTO users (name) VALUES ('Alice');  -- id = 1
INSERT INTO users (name) VALUES ('Bob');    -- id = 2
```

**Implementation:**
- Maintain sequence counter in catalog
- Auto-generate value on INSERT
- Thread-safe counter increment

---

### 6. String Functions (2-3 days)
Common string manipulation functions

```sql
SELECT
    UPPER(name),
    LOWER(email),
    CONCAT(first_name, ' ', last_name),
    SUBSTRING(bio, 1, 100),
    LENGTH(description)
FROM users;
```

**Implementation:**
- Add functions to expression evaluator
- Implement in `pkg/types/string_functions.go`

---

### 7. Date/Time Functions (2-3 days)
Basic date/time support

```sql
SELECT
    CURRENT_DATE,
    CURRENT_TIME,
    CURRENT_TIMESTAMP,
    NOW()
FROM users;
```

**Implementation:**
- Add DATE, TIME, TIMESTAMP types
- Basic date arithmetic

---

### 8. COALESCE & NULLIF (1 day)
NULL handling functions

```sql
SELECT COALESCE(phone, email, 'No contact') as contact
FROM users;

SELECT NULLIF(status, 'pending') as non_pending_status
FROM orders;
```

---

### 9. CASE Expressions (2 days)
Conditional expressions

```sql
SELECT
    name,
    CASE
        WHEN age < 18 THEN 'Minor'
        WHEN age < 65 THEN 'Adult'
        ELSE 'Senior'
    END as age_category
FROM users;
```

---

### 10. Benchmarking Suite (3 days)
Standardized performance testing

```bash
storemy benchmark --workload tpch --scale 1
storemy benchmark --workload oltp --duration 60s
```

**Implementation:**
- TPC-H query templates
- OLTP workload simulation
- Result reporting

---

## Index-Heap Integration Analysis

### How Heap Page Architecture Affects Indexes

This section provides a detailed analysis of the relationship between heap storage and index structures in StoreMy.

---

### Core Relationship

**Indexes store pointers to heap tuples, not the actual data:**

```
Index Entry Structure:
┌─────────────┬─────────────────────┐
│     Key     │     RecordID        │
│  (Indexed   │  (PageID, TupleNum) │
│   Value)    │                     │
└─────────────┴─────────────────────┘
                      ↓
              Points to Heap Page
```

**Example:**
```go
type IndexEntry struct {
    Key types.Field          // e.g., age = 30
    RID *tuple.TupleRecordID // Points to (HeapPageID{1, 5}, TupleNum: 3)
}
```

This means:
- Index knows **where** the data is
- Index does NOT know **what** the data is (except indexed column)
- To get full tuple, must follow RID to heap page

---

### Critical Feature: Stable RecordIDs

**Heap Page Slotted Structure:**
```
┌─────────────────────────────────────────────────────┐
│ Slot Pointers Array                                 │
│ [0] → Offset: 200, Length: 64  (Tuple 0)           │
│ [1] → Offset: 264, Length: 64  (Tuple 1)           │
│ [2] → Offset:   0, Length:  0  (Empty slot)        │
│ [3] → Offset: 328, Length: 64  (Tuple 3)           │
├─────────────────────────────────────────────────────┤
│ Free Space                                          │
├─────────────────────────────────────────────────────┤
│                  ← Tuple Data (grows backward)      │
│ [Tuple 3 data at offset 328]                        │
│ [Tuple 1 data at offset 264]                        │
│ [Tuple 0 data at offset 200]                        │
└─────────────────────────────────────────────────────┘
```

**Key Insight:** Slot numbers never change, even when tuples move!

**Before Compaction:**
```
Slot 0 → Offset 200 (tuple physically at byte 200)
Slot 1 → Offset 300 (tuple physically at byte 300)
Slot 2 → Empty
Slot 3 → Offset 400 (tuple physically at byte 400)
```

**After Compaction:**
```
Slot 0 → Offset 200 (tuple physically at byte 200) ← Same slot!
Slot 1 → Offset 264 (tuple physically at byte 264) ← Moved!
Slot 2 → Empty
Slot 3 → Offset 328 (tuple physically at byte 328) ← Moved!
```

**Impact on Indexes:**
- Index entries still point to Slot 1
- Physical offset changed (300 → 264) but **hidden from index**
- **Indexes don't need updates** during compaction!

**Code Reference:**
```go
// From pkg/storage/heap/page.go
func (hp *HeapPage) Compact() int {
    // Repacks tuples but keeps slot numbers stable
    for _, td := range activeTuples {
        // Update slot pointer to new location
        hp.slotPointers[td.slotIndex] = SlotPointer{
            Offset: hp.freeSpacePtr,  // New offset
            Length: tupleSize,
        }
        // Slot index (td.slotIndex) remains unchanged!
    }
}
```

---

### Index Entry Structure Deep Dive

**Both BTree and Hash indexes use the same entry format:**

```go
// From pkg/storage/index/index.go
type IndexEntry struct {
    Key Field                 // Indexed column value
    RID *tuple.TupleRecordID  // Pointer to heap tuple
}

type TupleRecordID struct {
    PageID   primitives.PageID  // Can be HeapPageID, BTreePageID, etc.
    TupleNum int                // Slot number in page
}
```

**Example Index Entry:**
```go
entry := &IndexEntry{
    Key: types.NewIntField(100),  // Indexed value
    RID: &TupleRecordID{
        PageID:   heap.NewHeapPageID(tableID: 1, pageNum: 5),
        TupleNum: 3,  // Slot 3 in heap page 5
    },
}
```

**Following an Index Entry:**
```go
// 1. Index scan finds entry with key = 100
entry := btreeIndex.Search(100)

// 2. Extract heap location
heapPageID := entry.RID.PageID.(*heap.HeapPageID)
slotNum := entry.RID.TupleNum

// 3. Read heap page from buffer pool
heapPage := bufferPool.GetPage(heapPageID)

// 4. Retrieve tuple from slot
tuple := heapPage.GetTupleAt(slotNum)

// 5. Now have full tuple with all columns!
```

---

### DML Operations & Index Maintenance

**How different operations affect both heap and indexes:**

#### INSERT Operation
```sql
INSERT INTO users (id, name, age) VALUES (100, 'Alice', 30);
```

**Steps:**
1. **Heap:** Find empty slot in heap page
2. **Heap:** Write tuple to slot (e.g., slot 5)
3. **Heap:** Update slot pointer
4. **Index:** For each index on table:
   - Extract indexed column value
   - Create IndexEntry(value, RID)
   - Insert into index

**Code Flow:**
```go
// From pkg/execution/operators/insert.go
func (op *InsertOperator) Next() (*Tuple, error) {
    // 1. Insert into heap
    err := heapFile.InsertTuple(tuple)

    // 2. Update all indexes
    err = indexManager.OnInsert(tableID, tuple)

    // 3. Log to WAL
    wal.WriteInsertRecord(txn, tuple)
}
```

---

#### DELETE Operation
```sql
DELETE FROM users WHERE id = 100;
```

**Steps:**
1. **Index:** Use index to find tuple location (if indexed)
2. **Heap:** Mark slot as empty (offset = 0)
3. **Index:** For each index:
   - Remove IndexEntry(value, RID) from index

**Important:** Heap slot becomes reusable after delete!

**Code Reference:**
```go
// From pkg/storage/heap/page.go
func (hp *HeapPage) DeleteTuple(t *Tuple) error {
    slotIndex := t.RecordID.TupleNum

    // Invalidate slot pointer (offset=0 means empty)
    hp.slotPointers[slotIndex] = SlotPointer{
        Offset: 0,  // Mark as empty
        Length: 0,
    }
    hp.tuples[slotIndex] = nil
    t.RecordID = nil
}
```

---

#### UPDATE Operation (Non-Key Column)
```sql
UPDATE users SET name = 'Bob' WHERE id = 100;
```

**Steps:**
1. **Heap:** Modify tuple in-place
2. **Index:** **NO INDEX UPDATE NEEDED** (indexed columns unchanged)

**This is a huge optimization!**

---

#### UPDATE Operation (Key Column)
```sql
UPDATE users SET age = 31 WHERE id = 100;
-- Assuming index on 'age'
```

**Steps:**
1. **Index:** Delete old entry (age=30, RID)
2. **Heap:** Modify tuple in-place
3. **Index:** Insert new entry (age=31, RID)

**Note:** RID stays the same (slot not moved)

---

### Index Serialization & PageID Types

**Challenge:** Index pages need to store different types of PageIDs

**For BTree Internal Nodes:**
- Child pointers can be BTreePageID (another BTree page)
- Or HeapPageID (not typical, but theoretically possible)

**For Leaf Nodes:**
- RecordID almost always points to HeapPageID
- But could point to other page types in complex scenarios

**Serialization Solution:**
```go
// From pkg/storage/index/btree/btree_page.go
func (p *BTreePage) serializeEntry(w io.Writer, entry *index.IndexEntry) error {
    rid := entry.RID

    // Type tag: 0 = HeapPageID, 1 = BTreePageID, etc.
    var pageIDType byte
    switch rid.PageID.(type) {
    case *heap.HeapPageID:
        pageIDType = 0
    case *BTreePageID:
        pageIDType = 1
    default:
        return fmt.Errorf("unknown PageID type")
    }

    binary.Write(w, binary.BigEndian, pageIDType)
    // ... serialize table ID, page number, tuple number
}
```

**Deserialization:**
```go
func (p *BTreePage) deserializeEntry(r io.Reader) (*index.IndexEntry, error) {
    var pageIDType byte
    binary.Read(r, binary.BigEndian, &pageIDType)

    // Read table ID and page number
    var tableID, pageNum int32
    binary.Read(r, binary.BigEndian, &tableID)
    binary.Read(r, binary.BigEndian, &pageNum)

    // Create appropriate PageID type
    var pageID primitives.PageID
    switch pageIDType {
    case 0:
        pageID = heap.NewHeapPageID(int(tableID), int(pageNum))
    case 1:
        pageID = NewBTreePageID(int(tableID), int(pageNum))
    }

    // ... read tuple number and create IndexEntry
}
```

---

### Transaction Management Impact

**Both heap and index pages implement the same interface:**
```go
type Page interface {
    GetID() primitives.PageID
    IsDirty() *primitives.TransactionID
    MarkDirty(bool, *primitives.TransactionID)
    GetPageData() []byte
    GetBeforeImage() Page
    SetBeforeImage()
}
```

**Transaction Example:**
```
BEGIN TRANSACTION (TID = 123)
  ↓
INSERT INTO users VALUES (100, 'Alice', 30);
  ↓
1. Acquire X lock on heap page 5
2. Insert tuple into heap page 5 slot 3
3. Mark heap page 5 dirty (TID = 123)
4. Capture heap page before-image
5. Log INSERT record to WAL
  ↓
6. Acquire X lock on BTree index page 10
7. Insert (30, HeapPageID{1,5}, 3) into index
8. Mark index page 10 dirty (TID = 123)
9. Capture index page before-image
10. Log index change to WAL (optional)
  ↓
COMMIT
  ↓
1. Write COMMIT record to WAL
2. Flush WAL to disk
3. Flush heap page 5 to disk (FORCE policy)
4. Flush index page 10 to disk
5. Release all locks
6. Mark transaction COMMITTED
```

**Rollback Example:**
```
BEGIN TRANSACTION (TID = 124)
  ↓
DELETE FROM users WHERE id = 100;
  ↓
1. Delete from heap page
2. Delete from index pages
3. Both pages marked dirty (TID = 124)
  ↓
ABORT (maybe due to constraint violation)
  ↓
1. Restore heap page from before-image
2. Restore index pages from before-images
3. Write ABORT record to WAL
4. Release all locks
5. Mark transaction ABORTED
```

**Code Reference:**
```go
// From pkg/storage/heap/page.go
func (hp *HeapPage) MarkDirty(dirty bool, tid *primitives.TransactionID) {
    if dirty {
        hp.dirtier = tid
        // Capture before-image if not already captured
        if hp.oldData == nil {
            hp.oldData = hp.GetPageData()
        }
    } else {
        hp.dirtier = nil
    }
}

func (hp *HeapPage) GetBeforeImage() page.Page {
    // Recreate page from before-image data
    beforePage, _ := NewHeapPage(hp.pageID, hp.oldData, hp.tupleDesc)
    return beforePage
}
```

---

### Consistency Challenges

#### Problem 1: Partial Updates
**Scenario:** Transaction fails after heap update but before index update

```go
// This can fail midway:
heapFile.InsertTuple(tuple)  // ✓ Succeeds
btreeIndex.Insert(key, rid)  // ✗ Fails (out of memory, disk full, etc.)

// Now: Heap has tuple but index doesn't!
// Result: Tuple invisible to index scans
```

**Solution:** Transactions with rollback
```go
txn.Begin()
defer func() {
    if err != nil {
        txn.Abort()  // Restores both heap and index pages
    }
}()

err = heapFile.InsertTuple(tuple)
if err != nil { return err }

err = btreeIndex.Insert(key, rid)
if err != nil { return err }

txn.Commit()  // Atomically commits both changes
```

---

#### Problem 2: Slot Reuse with Stale Index Entries
**Scenario:**
```
1. Insert tuple at slot 3 (id=100, name='Alice')
2. Create index entry: (100, slot 3)
3. Delete tuple at slot 3
4. Insert NEW tuple at slot 3 (id=200, name='Bob')
5. Old index entry still points to slot 3!
```

**Result:** Index search for id=100 returns wrong tuple (id=200)!

**Solution:** Always update indexes before reusing slots
```go
// From pkg/indexmanager/index_manager.go
func (im *IndexManager) OnInsert(tableID int, tuple *Tuple) error {
    // Before inserting into heap, ensure all indexes updated
    for _, index := range im.getIndexesForTable(tableID) {
        key := tuple.GetField(index.columnIndex)
        err := index.Insert(key, tuple.RecordID)
        if err != nil { return err }
    }
}

func (im *IndexManager) OnDelete(tableID int, tuple *Tuple) error {
    // Before deleting from heap, remove from all indexes
    for _, index := range im.getIndexesForTable(tableID) {
        key := tuple.GetField(index.columnIndex)
        err := index.Delete(key, tuple.RecordID)
        if err != nil { return err }
    }
}
```

---

#### Problem 3: Index-Heap Divergence After Crash
**Scenario:**
```
1. Transaction commits
2. Heap page flushed to disk
3. **CRASH before index page flushed**
4. After recovery: Heap has tuple but index doesn't
```

**Solution:** Write-Ahead Logging ensures consistency
```
Recovery Algorithm:
1. REDO phase: Replay all operations (heap + index)
2. Result: Both heap and indexes restored to consistent state
```

---

### Performance Implications

#### Index Scan Cost
```
Total Cost = Index Read Cost + Heap Access Cost

Example:
- Index scan: 3 pages read (BTree traversal)
- Heap access: 100 RIDs → potentially 100 heap page reads
- Total: 103 page reads

Sequential scan:
- Heap pages: 50 (read all pages once)
- Total: 50 page reads

Conclusion: Index only faster if selectivity high (few tuples match)
```

**Selectivity Calculation:**
```
Selectivity = (# matching tuples) / (# total tuples)

Low selectivity (0.01): Index scan likely faster
High selectivity (0.90): Sequential scan likely faster
```

**Code Reference:**
```go
// From pkg/planner/select.go
func (p *SelectPlanner) chooseAccessPath(...) Operator {
    selectivity := estimateSelectivity(predicate)

    indexCost := indexHeight + (selectivity * numTuples)
    seqScanCost := numPages

    if indexCost < seqScanCost {
        return NewIndexScan(...)
    } else {
        return NewSeqScan(...)
    }
}
```

---

#### Spatial Locality
**Good:** Multiple index entries point to same heap page
```
Index entries:
- (age=30, page=5, slot=1)
- (age=30, page=5, slot=2)
- (age=30, page=5, slot=3)

Result: Read page 5 once, get 3 tuples (cached!)
```

**Bad:** Random heap page access
```
Index entries:
- (age=30, page=5, slot=1)
- (age=30, page=87, slot=2)
- (age=30, page=23, slot=3)
- (age=30, page=145, slot=4)

Result: 4 page reads, poor cache utilization
```

**Optimization:** Sort index results by (PageID, TupleNum) before heap access
```go
func sortByRID(rids []TupleRecordID) {
    sort.Slice(rids, func(i, j int) bool {
        if rids[i].PageID != rids[j].PageID {
            return rids[i].PageID.PageNo() < rids[j].PageID.PageNo()
        }
        return rids[i].TupleNum < rids[j].TupleNum
    })
}
```

---

#### Space Overhead
**Example Table:**
- 1000 tuples
- 64 bytes per tuple
- Total heap size: 64KB ≈ 16 pages (4KB each)

**Add BTree index on one column:**
- Index entry size: 12 bytes (8 for key + 4 for RID)
- 1000 index entries = 12KB
- BTree overhead (internal nodes): ~3KB
- Total index size: 15KB ≈ 4 pages

**Space Overhead:**
```
Total = Heap + Indexes
      = 16 pages + 4 pages (per index)
      = 16 + (4 × num_indexes) pages

For 3 indexes: 16 + 12 = 28 pages (75% overhead!)
```

---

### Summary of Index-Heap Integration

| Heap Feature | Index Impact | Why It Matters |
|--------------|--------------|----------------|
| **Slotted page structure** | Enables stable RecordIDs | Indexes don't break during compaction |
| **Slot number stability** | Indexes never need updates on compaction | Major performance benefit |
| **RecordID = (PageID, TupleNum)** | Indexes store pointers, not data | Space efficient, but requires heap access |
| **Before-image tracking** | Both heap and index pages support rollback | Transactional consistency |
| **Page interface** | Unified buffer pool management | Simpler implementation |
| **Write-ahead logging** | Both logged together | Crash recovery consistency |

---

### Key Takeaways

1. **Separation of Concerns:**
   - Indexes: Fast lookup of location
   - Heap: Actual tuple storage
   - Clear responsibility boundary

2. **Stable RecordIDs = Happy Indexes:**
   - Slot numbers never change
   - Physical compaction is transparent
   - No index maintenance on compaction

3. **Consistency is Hard:**
   - Must update heap + indexes atomically
   - Transactions essential for correctness
   - WAL ensures recovery consistency

4. **Performance Trade-offs:**
   - Index scan fast for selective queries
   - Sequential scan faster for large results
   - Heap access always required (no covering indexes yet)

5. **This is Industry Standard:**
   - PostgreSQL uses similar slotted pages
   - MySQL InnoDB uses clustered indexes (different approach)
   - Your design follows proven patterns

---

**This design is solid and production-ready in terms of heap-index integration!**

---

## Conclusion

StoreMy is a well-architected database system with strong fundamentals:

**Strengths:**
- ✅ Solid storage layer (slotted pages, indexes)
- ✅ Proper transaction management (2PL, WAL)
- ✅ Complete query execution engine
- ✅ Good separation of concerns
- ✅ Comprehensive test coverage

**Priority Areas for Development:**
1. **Crash Recovery** - Critical for production use
2. **Query Optimization** - Huge performance gains
3. **SQL Completeness** - Match industry standards
4. **MVCC** - Modern concurrency model

**Long-term Vision:**
- Client/server architecture
- Replication for HA
- Advanced query features (CTEs, window functions)
- Full-text search
- Production monitoring

This roadmap provides a clear path from educational database to production-ready system. Focus on reliability first, then performance, then features!

---

**Document Version:** 1.0
**Last Updated:** 2025-10-16
**Next Review:** After Phase 1 completion
