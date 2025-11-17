# StoreMy Database - Strategic Development Roadmap

**Generated:** 2025-11-17
**Version:** 2.0
**Status:** Active Development

---

## 📋 Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current State Analysis](#current-state-analysis)
3. [Strategic Phases](#strategic-phases)
4. [Implementation Priorities](#implementation-priorities)
5. [Resource Requirements](#resource-requirements)
6. [Success Metrics](#success-metrics)

---

## Executive Summary

StoreMy is a **highly mature educational database system** with ~40K lines of production code and ~64K lines of test code. The project has successfully implemented:

- ✅ Complete storage layer with indexes
- ✅ ACID transactions with 2PL
- ✅ Full query execution engine
- ✅ Cost-based optimizer
- ✅ Comprehensive SQL parser

**Key Strengths:**
- Excellent architecture following industry patterns (PostgreSQL-inspired)
- Outstanding test coverage (64K lines)
- Production-grade concurrency control
- Beautiful user interface

**Strategic Gaps:**
1. **Crash recovery** (critical for production)
2. **Advanced SQL features** (subqueries, CTEs, window functions)
3. **MVCC** (modern concurrency model)
4. **Client/server architecture** (currently single-process)

This roadmap prioritizes features to transform StoreMy from an educational database into a **production-ready system** suitable for real-world applications.

---

## Current State Analysis

### Code Statistics
- **Production Code:** ~40,000 lines of Go
- **Test Code:** ~64,000 lines (160% test coverage!)
- **Source Files:** 339 implementation files + 103 test files
- **Packages:** 25+ major components
- **Maturity:** Educational → Pre-Production

### Component Completeness Matrix

| Component | Completeness | Status | Notes |
|-----------|--------------|--------|-------|
| **Storage Layer** | 95% | ✅ Production | Slotted pages, B+Tree, Hash indexes |
| **Buffer Pool** | 95% | ✅ Production | LRU eviction, page pinning |
| **Transaction Mgmt** | 80% | ⚠️ Needs Work | Missing crash recovery |
| **Lock Manager** | 100% | ✅ Production | 2PL with deadlock detection |
| **WAL System** | 70% | ⚠️ Needs Work | Logging works, recovery missing |
| **Query Execution** | 90% | ✅ Production | All join algorithms implemented |
| **Query Optimizer** | 70% | ⚠️ Needs Work | Basic cost model, needs histograms |
| **SQL Parser** | 85% | ✅ Good | Missing subqueries, CTEs |
| **System Catalog** | 95% | ✅ Production | Complete metadata management |
| **User Interface** | 85% | ✅ Production | Beautiful terminal UI |

### Performance Characteristics (Current)

**Strengths:**
- Join algorithm selection: O(n+m) for hash joins
- Index lookups: O(log n) for B+Tree
- Lock granularity: Page-level (fine-grained)
- Buffer pool: 4MB default, configurable

**Bottlenecks:**
- Sequential scans on large tables (no parallel execution)
- Join ordering not optimal (basic cost model)
- Statistics not histogram-based
- No query plan caching

---

## Strategic Phases

### 🎯 **Phase 1: RELIABILITY & DURABILITY** (4-6 weeks)
**Priority:** 🔴 **CRITICAL**
**Goal:** Implement crash recovery for production readiness

#### Week 1-2: Recovery Manager Foundation
**Deliverables:**
- [ ] Create `pkg/recovery/recovery_manager.go`
- [ ] Implement Analysis Phase (scan WAL, identify uncommitted txns)
- [ ] Implement REDO algorithm (replay all operations)
- [ ] Implement UNDO algorithm (rollback uncommitted txns)

**Technical Details:**
```go
// Recovery phases
type RecoveryManager struct {
    wal          *WAL
    bufferPool   *BufferPool
    catalog      *CatalogManager
    dirtyPages   map[PageID]LSN     // Analysis phase
    activeTxns   map[TransactionID]*TxnInfo
}

// Analysis: Scan WAL from last checkpoint
func (rm *RecoveryManager) AnalysisPhase() error {
    // Build active transaction table
    // Build dirty page table
    // Identify undo/redo starting points
}

// Redo: Replay all logged operations
func (rm *RecoveryManager) RedoPhase() error {
    // For each log record from redo point:
    //   - Check if page LSN < record LSN
    //   - If yes, replay operation
}

// Undo: Rollback uncommitted transactions
func (rm *RecoveryManager) UndoPhase() error {
    // For each active transaction:
    //   - Follow undo chain backward
    //   - Apply before-images
    //   - Write CLR (Compensation Log Records)
}
```

**Test Scenarios:**
1. Crash during active transaction
2. Crash after COMMIT logged but before page flush
3. Multiple concurrent transactions at crash
4. Recovery with partial checkpoint

**Acceptance Criteria:**
- ✅ All committed transactions preserved after crash
- ✅ All uncommitted transactions rolled back
- ✅ Database consistent after recovery
- ✅ Performance overhead < 10%

#### Week 3-4: Checkpoint Mechanism
**Deliverables:**
- [ ] Implement fuzzy checkpoints (non-blocking)
- [ ] Checkpoint triggering strategy
- [ ] Log truncation after successful checkpoint
- [ ] Recovery from checkpoint instead of log start

**Technical Details:**
```go
// Checkpoint record contains:
type CheckpointRecord struct {
    ActiveTxns  map[TransactionID]*TxnInfo
    DirtyPages  map[PageID]LSN
    Timestamp   time.Time
}

// Periodic checkpointing
func (wal *WAL) StartCheckpointDaemon(interval time.Duration) {
    ticker := time.NewTicker(interval)
    for range ticker.C {
        wal.WriteCheckpoint()
    }
}
```

**Checkpoint Strategy:**
- Trigger every 10 minutes OR 10MB of WAL
- Fuzzy checkpoint (non-blocking writes)
- Log truncation after successful checkpoint

**Estimated Impact:**
- Recovery time: 10-100x faster
- Disk usage: Bounded WAL size
- Availability: Checkpoints don't block writes

#### Week 5-6: Testing & Documentation
**Deliverables:**
- [ ] Comprehensive recovery test suite
- [ ] Crash injection framework
- [ ] Performance benchmarks
- [ ] Recovery documentation

**Test Framework:**
```bash
# Crash injection testing
go test ./pkg/recovery/ -crash-injection \
    -scenarios "commit-before-flush,mid-transaction,checkpoint" \
    -runs 1000

# Recovery benchmarks
go test ./pkg/recovery/ -bench . -benchtime 10s
```

**Documentation:**
- Recovery algorithm explanation
- Checkpoint configuration guide
- Troubleshooting common recovery issues

---

### 🎯 **Phase 2: QUERY OPTIMIZATION** (4-6 weeks)
**Priority:** 🟡 **HIGH**
**Goal:** 10-100x performance improvement on complex queries

#### Week 1-2: Histogram-Based Statistics
**Deliverables:**
- [ ] Integrate existing histogram code into query optimizer
- [ ] Implement ANALYZE command
- [ ] Auto-update statistics on significant DML
- [ ] Multi-column correlation statistics

**Technical Details:**
```sql
-- Manual statistics collection
ANALYZE users;
ANALYZE orders (customer_id, order_date);

-- Automatic updates
-- After 10% of table modified
```

**Histogram Integration:**
```go
// Use histogram for selectivity estimation
func (opt *Optimizer) EstimateSelectivity(pred Predicate, table Table) float64 {
    // Get column histogram from catalog
    histogram := opt.catalog.GetHistogram(table, pred.Column)

    // Estimate using histogram buckets
    return histogram.EstimateSelectivity(pred.Operator, pred.Value)
}
```

**Expected Improvement:**
- Cardinality estimates: Within 2x of actual (vs 10x currently)
- Join order selection: 5-10x better on complex queries

#### Week 3-4: Advanced Cost Model
**Deliverables:**
- [ ] I/O cost improvements (sequential vs random)
- [ ] CPU cost modeling
- [ ] Memory cost consideration
- [ ] Network cost (preparation for distributed)

**Cost Model Formula:**
```
TotalCost = α·IO_Cost + β·CPU_Cost + γ·Memory_Cost

where:
  IO_Cost = Sequential_Reads × 1.0 + Random_Reads × 4.0
  CPU_Cost = Tuples_Processed × CPU_COST_PER_TUPLE
  Memory_Cost = Hash_Tables + Sort_Buffers
```

**Implementation:**
```go
type CostEstimate struct {
    IOCost      float64  // Disk I/O cost
    CPUCost     float64  // Processing cost
    MemoryCost  float64  // Memory usage
    TotalCost   float64  // Weighted sum
    Cardinality int      // Estimated rows
}

const (
    SEQUENTIAL_READ_COST = 1.0
    RANDOM_READ_COST     = 4.0
    CPU_COST_PER_TUPLE   = 0.01
)
```

#### Week 5-6: Query Plan Caching
**Deliverables:**
- [ ] Implement LRU plan cache
- [ ] Cache invalidation on schema changes
- [ ] Parameterized query support
- [ ] Cache hit rate monitoring

**Plan Cache Design:**
```go
type PlanCache struct {
    cache     map[string]*CachedPlan  // SQL hash → plan
    lru       *list.List
    capacity  int
    mu        sync.RWMutex
    hits      atomic.Int64
    misses    atomic.Int64
}

// Cache lookup
func (pc *PlanCache) Get(sql string) (*Plan, bool) {
    key := hash(sql)

    pc.mu.RLock()
    plan, found := pc.cache[key]
    pc.mu.RUnlock()

    if found {
        pc.hits.Add(1)
        pc.updateLRU(plan)
        return plan.Clone(), true
    }

    pc.misses.Add(1)
    return nil, false
}

// Invalidation on schema changes
func (pc *PlanCache) InvalidateTable(tableID int) {
    // Remove all plans referencing this table
}
```

**Expected Performance:**
- Plan parsing time: 50-90% reduction on cache hits
- Throughput: 2-5x for repeated queries
- Memory usage: ~1MB per 100 cached plans

**Acceptance Criteria:**
- ✅ Cache hit rate > 80% in production workloads
- ✅ Plan invalidation correct on schema changes
- ✅ No memory leaks under sustained load

---

### 🎯 **Phase 3: SQL COMPLETENESS** (6-8 weeks)
**Priority:** 🟡 **HIGH**
**Goal:** Support 95% of common SQL features

#### Week 1-2: DISTINCT & Set Operations
**Deliverables:**
- [ ] DISTINCT operator (hash-based deduplication)
- [ ] UNION / UNION ALL
- [ ] INTERSECT
- [ ] EXCEPT (MINUS)

**Implementation:**
```go
// pkg/execution/operators/distinct.go
type DistinctOperator struct {
    child       Operator
    seenHashes  map[uint64]struct{}
    hashFields  []int  // Which fields to hash
}

func (d *DistinctOperator) Next() (*Tuple, error) {
    for {
        tuple, err := d.child.Next()
        if err != nil {
            return nil, err
        }

        hash := tuple.HashFields(d.hashFields)
        if _, seen := d.seenHashes[hash]; !seen {
            d.seenHashes[hash] = struct{}{}
            return tuple, nil
        }
        // Skip duplicate, continue loop
    }
}
```

**SQL Examples:**
```sql
-- DISTINCT
SELECT DISTINCT department FROM employees;

-- UNION
SELECT id FROM employees UNION SELECT id FROM managers;

-- INTERSECT
SELECT email FROM users INTERSECT SELECT email FROM subscribers;

-- EXCEPT
SELECT user_id FROM all_users EXCEPT SELECT user_id FROM banned_users;
```

#### Week 3-5: Subquery Support
**Deliverables:**
- [ ] Scalar subqueries
- [ ] IN / NOT IN subqueries
- [ ] EXISTS / NOT EXISTS
- [ ] Correlated subqueries
- [ ] Subquery decorrelation optimization

**Subquery Types:**

**1. Scalar Subqueries:**
```sql
SELECT name, salary
FROM employees
WHERE salary > (SELECT AVG(salary) FROM employees);
```

**2. IN Subqueries:**
```sql
SELECT name FROM employees
WHERE department_id IN (SELECT id FROM departments WHERE active = true);
```

**3. EXISTS:**
```sql
SELECT name FROM users u
WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id);
```

**4. Correlated:**
```sql
SELECT e.name,
       (SELECT AVG(salary) FROM employees e2 WHERE e2.dept = e.dept) as avg_dept_salary
FROM employees e;
```

**Implementation Strategy:**
```go
// pkg/execution/operators/subquery.go
type SubqueryOperator struct {
    outer       Operator
    inner       Operator  // Subquery plan
    correlation []int     // Correlation variables
    evalType    SubqueryType  // SCALAR, IN, EXISTS
}

// Decorrelation optimization
func (opt *Optimizer) DecorrelateSubquery(subquery *SelectStmt) *Plan {
    // Transform correlated subquery to join
    // WHERE EXISTS (SELECT ... WHERE outer.id = inner.id)
    // →
    // SEMI JOIN ON outer.id = inner.id
}
```

**Acceptance Criteria:**
- ✅ All TPC-H queries execute correctly
- ✅ Correlated subqueries within 2x of manual join
- ✅ Subquery result caching for uncorrelated queries

#### Week 6-7: Common Table Expressions (CTEs)
**Deliverables:**
- [ ] Non-recursive CTEs
- [ ] Recursive CTEs
- [ ] Multiple CTEs in single query
- [ ] CTE materialization vs inlining decision

**SQL Examples:**
```sql
-- Non-recursive CTE
WITH high_earners AS (
    SELECT * FROM employees WHERE salary > 100000
)
SELECT department, COUNT(*) FROM high_earners GROUP BY department;

-- Recursive CTE (org chart)
WITH RECURSIVE org_tree AS (
    SELECT id, name, manager_id, 1 as level
    FROM employees
    WHERE manager_id IS NULL

    UNION ALL

    SELECT e.id, e.name, e.manager_id, ot.level + 1
    FROM employees e
    JOIN org_tree ot ON e.manager_id = ot.id
)
SELECT * FROM org_tree ORDER BY level;
```

**Implementation:**
```go
// CTE execution plan
type CTENode struct {
    Name         string
    Plan         Operator
    Materialized bool
    Recursive    bool
}

// Recursive CTE
type RecursiveCTEOperator struct {
    basePlan      Operator  // Initial seed
    recursivePlan Operator  // Recursive part
    workingTable  []Tuple   // Intermediate results
    iteration     int
    maxIterations int       // Prevent infinite loops
}
```

#### Week 8: Window Functions (Optional, if time permits)
**Deliverables:**
- [ ] ROW_NUMBER(), RANK(), DENSE_RANK()
- [ ] Aggregate window functions (SUM, AVG, etc. OVER)
- [ ] PARTITION BY support
- [ ] Frame specifications (ROWS/RANGE)

```sql
SELECT
    name,
    salary,
    department,
    AVG(salary) OVER (PARTITION BY department) as dept_avg,
    RANK() OVER (PARTITION BY department ORDER BY salary DESC) as dept_rank
FROM employees;
```

---

### 🎯 **Phase 4: DATA INTEGRITY** (4-6 weeks)
**Priority:** 🟡 **MEDIUM-HIGH**
**Goal:** Ensure data quality with constraints

#### Week 1-2: Constraint Infrastructure
**Deliverables:**
- [ ] Extend catalog with CATALOG_CONSTRAINTS table
- [ ] Constraint validation framework
- [ ] Error messages for constraint violations
- [ ] Constraint metadata API

**Catalog Schema:**
```sql
CREATE TABLE CATALOG_CONSTRAINTS (
    constraint_id INT PRIMARY KEY,
    constraint_name VARCHAR,
    table_id INT,
    constraint_type VARCHAR,  -- UNIQUE, CHECK, FK, NOT_NULL
    column_names VARCHAR,      -- Comma-separated
    referenced_table INT,      -- For foreign keys
    delete_action VARCHAR,     -- CASCADE, SET_NULL, RESTRICT
    check_expression TEXT      -- For CHECK constraints
);
```

#### Week 3: UNIQUE & CHECK Constraints
**Deliverables:**
- [ ] UNIQUE constraint implementation
- [ ] Automatic unique index creation
- [ ] CHECK constraint evaluation
- [ ] Constraint violation error handling

**SQL Examples:**
```sql
-- UNIQUE
CREATE TABLE users (
    id INT PRIMARY KEY,
    email VARCHAR UNIQUE,
    username VARCHAR UNIQUE
);

-- CHECK
CREATE TABLE employees (
    id INT PRIMARY KEY,
    age INT CHECK (age >= 18 AND age <= 100),
    salary FLOAT CHECK (salary > 0),
    email VARCHAR CHECK (email LIKE '%@%')
);
```

**Implementation:**
```go
// Constraint validation on INSERT/UPDATE
func (table *Table) ValidateConstraints(tuple *Tuple) error {
    for _, constraint := range table.constraints {
        switch constraint.Type {
        case UNIQUE:
            if err := table.checkUnique(tuple, constraint); err != nil {
                return err
            }
        case CHECK:
            if err := table.checkExpression(tuple, constraint); err != nil {
                return err
            }
        }
    }
    return nil
}
```

#### Week 4: DEFAULT Values & NOT NULL
**Deliverables:**
- [ ] DEFAULT value storage in column metadata
- [ ] Apply defaults on INSERT
- [ ] Function defaults (CURRENT_TIMESTAMP, etc.)
- [ ] NOT NULL constraint enforcement

**SQL Examples:**
```sql
CREATE TABLE orders (
    id INT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR DEFAULT 'pending' NOT NULL,
    quantity INT DEFAULT 1,
    user_id INT NOT NULL
);

-- INSERT without all columns
INSERT INTO orders (id, user_id) VALUES (1, 100);
-- Auto-filled: created_at=now, status='pending', quantity=1
```

#### Week 5-6: Foreign Key Constraints
**Deliverables:**
- [ ] Foreign key validation on INSERT/UPDATE
- [ ] CASCADE operations (DELETE, UPDATE)
- [ ] SET NULL, SET DEFAULT, RESTRICT actions
- [ ] Circular FK detection
- [ ] Performance optimization (FK indexes)

**SQL Examples:**
```sql
CREATE TABLE orders (
    id INT PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE CASCADE,
    product_id INT REFERENCES products(id) ON DELETE RESTRICT,
    coupon_id INT REFERENCES coupons(id) ON DELETE SET NULL
);

-- Cascade delete example:
DELETE FROM users WHERE id = 1;
-- Also deletes all orders where user_id = 1
```

**Implementation Challenges:**
1. **Performance:** FK checks on every insert (solution: index foreign keys)
2. **Deadlocks:** Cascade operations can create deadlocks (solution: careful lock ordering)
3. **Circular references:** Prevent circular FK chains (solution: validation at CREATE TABLE)

**Acceptance Criteria:**
- ✅ All constraint violations caught before commit
- ✅ CASCADE operations transactional
- ✅ Performance impact < 20% on constrained tables

---

### 🎯 **Phase 5: MVCC & ADVANCED CONCURRENCY** (8-10 weeks)
**Priority:** 🟠 **MEDIUM**
**Goal:** Modern concurrency control for 3-10x better read throughput

#### Week 1-2: MVCC Design & Planning
**Deliverables:**
- [ ] Detailed MVCC architecture document
- [ ] Tuple version chain design
- [ ] Snapshot isolation semantics
- [ ] Garbage collection strategy

**MVCC Core Concepts:**
```
Traditional 2PL:
- Readers block writers
- Writers block readers
- Serial execution for conflicting txns

MVCC:
- Readers never block writers
- Writers never block readers (except conflicts)
- Multiple versions of same row
- Snapshot isolation
```

**Design Decisions:**

| Aspect | PostgreSQL-style (Chosen) | MySQL InnoDB |
|--------|---------------------------|--------------|
| **Version Storage** | In-heap with chains | Separate rollback segment |
| **Garbage Collection** | VACUUM process | Purge threads |
| **Visibility** | Snapshot + xmin/xmax | Transaction ID comparison |
| **Complexity** | High | Very High |

#### Week 3-4: Tuple Versioning Implementation
**Deliverables:**
- [ ] Extend heap tuple format with xmin, xmax, next_version
- [ ] Version chain management
- [ ] UPDATE creates new version
- [ ] DELETE marks version as deleted

**Tuple Format Changes:**
```go
// Current tuple
type HeapTuple struct {
    Data     []byte
    RecordID *TupleRecordID
}

// MVCC tuple
type HeapTupleMVCC struct {
    Data        []byte
    RecordID    *TupleRecordID

    // MVCC metadata
    Xmin        TransactionID     // Transaction that created this version
    Xmax        TransactionID     // Transaction that deleted/updated (0 if alive)
    NextVersion *TupleRecordID    // Pointer to newer version

    // Status flags
    Committed   bool              // Is creating txn committed?
    Deleted     bool              // Is this version deleted?
}
```

**Version Chain Example:**
```
Transaction timeline:
T1: INSERT (id=1, name='Alice')
T2: UPDATE SET name='Bob' WHERE id=1
T3: UPDATE SET name='Charlie' WHERE id=1

Physical storage:
┌─────────────────────────────────────────┐
│ Version 1 (oldest)                      │
│ xmin=T1, xmax=T2, name='Alice'          │
│ next_version → Version 2                │
└───────────────┬─────────────────────────┘
                ↓
┌─────────────────────────────────────────┐
│ Version 2                               │
│ xmin=T2, xmax=T3, name='Bob'            │
│ next_version → Version 3                │
└───────────────┬─────────────────────────┘
                ↓
┌─────────────────────────────────────────┐
│ Version 3 (newest)                      │
│ xmin=T3, xmax=NULL, name='Charlie'      │
│ next_version → NULL                     │
└─────────────────────────────────────────┘
```

#### Week 5-6: Snapshot Isolation
**Deliverables:**
- [ ] Snapshot creation on transaction start
- [ ] Visibility rules implementation
- [ ] Read-only transaction optimization
- [ ] Serialization anomaly detection

**Snapshot Structure:**
```go
type Snapshot struct {
    SnapshotID  int64
    XminAll     TransactionID    // Oldest active transaction
    XmaxAll     TransactionID    // Newest transaction at snapshot time
    ActiveTxns  map[TransactionID]struct{}
    Timestamp   time.Time
}

// Visibility check
func (s *Snapshot) IsVisible(tuple *HeapTupleMVCC) bool {
    // Rule 1: Created by current transaction → visible
    if tuple.Xmin == s.CurrentTxnID {
        return tuple.Xmax == 0 || tuple.Xmax != s.CurrentTxnID
    }

    // Rule 2: Created by committed txn before snapshot → visible
    if tuple.Xmin < s.XminAll && tuple.Committed {
        // Check if deleted by txn in our snapshot
        if tuple.Xmax == 0 {
            return true  // Not deleted
        }
        if tuple.Xmax >= s.XmaxAll {
            return true  // Deleted after our snapshot
        }
        _, wasActive := s.ActiveTxns[tuple.Xmax]
        return wasActive  // Visible if deleter was active
    }

    // Rule 3: Created by active/future txn → not visible
    return false
}
```

**Isolation Levels:**
```
READ COMMITTED:
- Take new snapshot for each statement
- See committed changes within transaction

REPEATABLE READ:
- Single snapshot for entire transaction
- Consistent view throughout

SERIALIZABLE:
- REPEATABLE READ + conflict detection
- Abort on serialization anomalies
```

#### Week 7-8: Vacuum & Garbage Collection
**Deliverables:**
- [ ] VACUUM command implementation
- [ ] Dead tuple identification
- [ ] Version chain cleanup
- [ ] Index cleanup
- [ ] Auto-vacuum daemon

**Vacuum Process:**
```sql
-- Manual vacuum
VACUUM users;

-- Aggressive vacuum (reclaim space)
VACUUM FULL users;

-- Auto-vacuum configuration
SET autovacuum = true;
SET autovacuum_vacuum_threshold = 50;  -- Min tuples
SET autovacuum_vacuum_scale_factor = 0.2;  -- 20% change
```

**Implementation:**
```go
type VacuumManager struct {
    db          *Database
    enabled     bool
    threshold   int
    scaleFactor float64
}

func (vm *VacuumManager) Vacuum(table *Table) error {
    // 1. Identify dead tuples (xmax committed, no active snapshots)
    deadTuples := vm.findDeadTuples(table)

    // 2. Remove from indexes
    for _, index := range table.indexes {
        index.RemoveTuples(deadTuples)
    }

    // 3. Compact heap pages
    for _, page := range table.pages {
        page.RemoveDeadTuples(deadTuples)
        page.Compact()
    }

    // 4. Update statistics
    table.UpdateStatistics()
}

// Auto-vacuum trigger
func (vm *VacuumManager) shouldVacuum(table *Table) bool {
    stats := table.GetStatistics()
    threshold := vm.threshold + int(float64(stats.NumTuples) * vm.scaleFactor)
    return stats.DeadTuples >= threshold
}
```

#### Week 9-10: Testing & Performance Tuning
**Deliverables:**
- [ ] Concurrent workload testing
- [ ] Performance comparison (2PL vs MVCC)
- [ ] Memory overhead measurement
- [ ] Vacuum tuning guidelines

**Test Scenarios:**
1. **Read-heavy workload:** 90% reads, 10% writes
2. **Write-heavy workload:** 10% reads, 90% writes
3. **Mixed workload:** 50% reads, 50% writes
4. **Long-running transactions:** Hours-long analytics queries

**Performance Benchmarks:**
```bash
# Benchmark MVCC vs 2PL
go test ./pkg/mvcc/ -bench . -benchtime 30s \
    -cpuprofile cpu.prof -memprofile mem.prof

# Sysbench-style OLTP
storemy benchmark \
    --workload oltp-read-write \
    --threads 16 \
    --duration 300s
```

**Expected Results:**
- Read throughput: 3-10x improvement
- Write throughput: 0.9-1.1x (slight overhead)
- Memory usage: +30-50% (version storage)
- Vacuum overhead: 5-10% background CPU

**Acceptance Criteria:**
- ✅ Readers don't block writers
- ✅ Writers don't block readers
- ✅ Throughput > 3x on read-heavy workloads
- ✅ Vacuum keeps space overhead < 30%

---

### 🎯 **Phase 6: CLIENT/SERVER ARCHITECTURE** (6-8 weeks)
**Priority:** 🟢 **MEDIUM-LOW**
**Goal:** Transform into multi-user database server

#### Week 1-2: Wire Protocol Design
**Deliverables:**
- [ ] Binary protocol specification
- [ ] Message framing
- [ ] Authentication handshake
- [ ] TLS/SSL support

**Protocol Design:**
```
Message Format (inspired by PostgreSQL):
┌─────────────────────────────────────────┐
│ Message Type (1 byte)                   │
├─────────────────────────────────────────┤
│ Message Length (4 bytes, big-endian)    │
├─────────────────────────────────────────┤
│ Message Payload (variable length)       │
└─────────────────────────────────────────┘

Message Types:
'Q' - Query
'P' - Parse (prepared statement)
'B' - Bind
'E' - Execute
'D' - Describe
'S' - Sync
'T' - RowDescription
'D' - DataRow
'C' - CommandComplete
'E' - ErrorResponse
```

**Implementation:**
```go
// pkg/protocol/message.go
type Message interface {
    Type() byte
    Serialize(w io.Writer) error
    Deserialize(r io.Reader) error
}

type QueryMessage struct {
    SQL string
}

func (qm *QueryMessage) Type() byte { return 'Q' }

func (qm *QueryMessage) Serialize(w io.Writer) error {
    // Write message type
    w.Write([]byte{qm.Type()})

    // Write length (4 bytes + SQL length)
    length := 4 + len(qm.SQL)
    binary.Write(w, binary.BigEndian, uint32(length))

    // Write SQL
    w.Write([]byte(qm.SQL))
    return nil
}
```

#### Week 3-4: TCP Server Implementation
**Deliverables:**
- [ ] TCP listener
- [ ] Connection pooling
- [ ] Session management
- [ ] Multi-client concurrent access

**Server Architecture:**
```go
// pkg/server/server.go
type DatabaseServer struct {
    listener    net.Listener
    database    *Database
    sessions    map[string]*Session
    config      *ServerConfig
    mu          sync.RWMutex
}

type ServerConfig struct {
    Host            string
    Port            int
    MaxConnections  int
    TLSEnabled      bool
    TLSCertFile     string
    TLSKeyFile      string
}

func (s *DatabaseServer) Start() error {
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.config.Host, s.config.Port))
    if err != nil {
        return err
    }
    s.listener = listener

    log.Printf("StoreMy server listening on %s:%d", s.config.Host, s.config.Port)

    for {
        conn, err := s.listener.Accept()
        if err != nil {
            log.Printf("Accept error: %v", err)
            continue
        }

        // Check connection limit
        if len(s.sessions) >= s.config.MaxConnections {
            conn.Close()
            continue
        }

        go s.handleClient(conn)
    }
}

func (s *DatabaseServer) handleClient(conn net.Conn) {
    defer conn.Close()

    // Create session
    session := NewSession(conn, s.database)
    s.registerSession(session)
    defer s.unregisterSession(session)

    // Message loop
    for {
        msg, err := protocol.ReadMessage(conn)
        if err != nil {
            if err != io.EOF {
                log.Printf("Read error: %v", err)
            }
            return
        }

        response := session.HandleMessage(msg)
        protocol.WriteMessage(conn, response)
    }
}
```

**Session Management:**
```go
type Session struct {
    ID           string
    conn         net.Conn
    database     *Database
    transaction  *Transaction
    preparedStmts map[string]*PreparedStatement
    createdAt    time.Time
    lastActivity time.Time
}

func (s *Session) HandleMessage(msg protocol.Message) protocol.Message {
    switch m := msg.(type) {
    case *protocol.QueryMessage:
        return s.executeQuery(m.SQL)
    case *protocol.ParseMessage:
        return s.prepareStatement(m.Name, m.SQL)
    case *protocol.BindMessage:
        return s.bindStatement(m.Name, m.Params)
    // ... other message types
    }
}
```

#### Week 5-6: Client Library & CLI
**Deliverables:**
- [ ] Go client library
- [ ] Connection pooling
- [ ] Transaction support
- [ ] Command-line client

**Client Library:**
```go
// pkg/client/client.go
type Client struct {
    conn   net.Conn
    config ClientConfig
}

type ClientConfig struct {
    Host     string
    Port     int
    Database string
    User     string
    Password string
    Timeout  time.Duration
}

func Connect(config ClientConfig) (*Client, error) {
    conn, err := net.DialTimeout("tcp",
        fmt.Sprintf("%s:%d", config.Host, config.Port),
        config.Timeout)
    if err != nil {
        return nil, err
    }

    client := &Client{conn: conn, config: config}

    // Authentication handshake
    if err := client.authenticate(); err != nil {
        conn.Close()
        return nil, err
    }

    return client, nil
}

func (c *Client) Query(sql string) (*ResultSet, error) {
    // Send query message
    msg := &protocol.QueryMessage{SQL: sql}
    if err := protocol.WriteMessage(c.conn, msg); err != nil {
        return nil, err
    }

    // Read response
    response, err := protocol.ReadMessage(c.conn)
    if err != nil {
        return nil, err
    }

    return parseResultSet(response)
}

func (c *Client) Begin() (*Tx, error) {
    _, err := c.Query("BEGIN")
    return &Tx{client: c}, err
}

type Tx struct {
    client *Client
}

func (tx *Tx) Commit() error {
    _, err := tx.client.Query("COMMIT")
    return err
}

func (tx *Tx) Rollback() error {
    _, err := tx.client.Query("ROLLBACK")
    return err
}
```

**Command-Line Client:**
```bash
# Connect to server
$ storemy-cli -h localhost -p 5433 -d mydb -u admin

storemy> SELECT * FROM users;
┌────┬───────┬─────┐
│ ID │ Name  │ Age │
├────┼───────┼─────┤
│  1 │ Alice │  30 │
│  2 │ Bob   │  25 │
└────┴───────┴─────┘
2 rows in set (0.05 sec)

storemy> \dt
Tables in 'mydb':
- users
- orders
- products

storemy> \d users
Table: users
┌────────────┬──────────┬─────────────┐
│ Column     │ Type     │ Constraints │
├────────────┼──────────┼─────────────┤
│ id         │ INT      │ PRIMARY KEY │
│ name       │ VARCHAR  │             │
│ age        │ INT      │             │
└────────────┴──────────┴─────────────┘
```

#### Week 7-8: Replication (Optional)
**Deliverables:**
- [ ] WAL streaming to replicas
- [ ] Replica lag monitoring
- [ ] Failover support (manual)
- [ ] Read replica load balancing

**Replication Architecture:**
```
┌─────────────┐                  ┌─────────────┐
│  Primary    │                  │  Replica 1  │
│  (Read/Write│ ──WAL Stream───→ │  (Read-Only)│
│             │                  │             │
└─────────────┘                  └─────────────┘
       │                                │
       │                         ┌─────────────┐
       │                         │  Replica 2  │
       └────────WAL Stream──────→│  (Read-Only)│
                                 │             │
                                 └─────────────┘
```

**Implementation:**
```go
type ReplicationManager struct {
    primary     *Database
    replicas    []*ReplicaConnection
    walStreamer *WALStreamer
}

type ReplicaConnection struct {
    conn      net.Conn
    lastLSN   LSN
    lagBytes  int64
    lagTime   time.Duration
}

func (rm *ReplicationManager) StreamWAL() {
    for {
        // Wait for new WAL records
        record := <-rm.wal.NewRecordChan()

        // Send to all replicas
        for _, replica := range rm.replicas {
            if err := replica.SendWAL(record); err != nil {
                log.Printf("Replica %s failed: %v", replica.ID, err)
                rm.handleReplicaFailure(replica)
            }
        }
    }
}
```

---

### 🎯 **Phase 7: ADVANCED SQL FEATURES** (8-12 weeks)
**Priority:** 🟢 **LOW-MEDIUM**
**Goal:** Support advanced database features

#### Views (2 weeks)
**Deliverables:**
- [ ] CREATE VIEW / DROP VIEW
- [ ] Query rewriting for views
- [ ] Materialized views
- [ ] REFRESH MATERIALIZED VIEW

```sql
-- Regular view
CREATE VIEW active_users AS
SELECT * FROM users WHERE active = true;

-- Materialized view
CREATE MATERIALIZED VIEW user_stats AS
SELECT department, COUNT(*) as count, AVG(salary) as avg_salary
FROM employees
GROUP BY department;

REFRESH MATERIALIZED VIEW user_stats;
```

#### Stored Procedures & UDFs (3-4 weeks)
**Deliverables:**
- [ ] PL/SQL-like language parser
- [ ] Variable declarations
- [ ] Control flow (IF, WHILE, FOR)
- [ ] Exception handling
- [ ] Function registry

```sql
-- User-Defined Function
CREATE FUNCTION calculate_tax(income FLOAT, rate FLOAT)
RETURNS FLOAT AS $$
BEGIN
    IF income < 0 THEN
        RAISE EXCEPTION 'Income cannot be negative';
    END IF;
    RETURN income * rate;
END;
$$ LANGUAGE plsql;

-- Stored Procedure
CREATE PROCEDURE process_orders()
AS $$
DECLARE
    order_count INT;
BEGIN
    SELECT COUNT(*) INTO order_count FROM orders WHERE status = 'pending';

    UPDATE orders SET status = 'processing'
    WHERE status = 'pending';

    RAISE NOTICE 'Processed % orders', order_count;
END;
$$ LANGUAGE plsql;
```

#### Triggers (2-3 weeks)
**Deliverables:**
- [ ] CREATE TRIGGER / DROP TRIGGER
- [ ] BEFORE/AFTER triggers
- [ ] ROW/STATEMENT level
- [ ] Trigger execution engine

```sql
CREATE TRIGGER update_modified_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION update_timestamp();

CREATE TRIGGER audit_changes
AFTER INSERT OR UPDATE OR DELETE ON sensitive_table
FOR EACH ROW
EXECUTE FUNCTION log_audit_trail();
```

#### Full-Text Search (3-4 weeks)
**Deliverables:**
- [ ] GIN (Generalized Inverted Index) implementation
- [ ] Text processing (tokenization, stemming)
- [ ] to_tsvector() and to_tsquery() functions
- [ ] Relevance ranking

```sql
CREATE INDEX users_bio_search ON users
USING GIN(to_tsvector('english', bio));

SELECT name, ts_rank(to_tsvector(bio), query) as rank
FROM users, to_tsquery('database & postgresql') query
WHERE to_tsvector(bio) @@ query
ORDER BY rank DESC;
```

---

### 🎯 **Phase 8: OPERATIONS & MONITORING** (4-6 weeks)
**Priority:** 🟢 **LOW**
**Goal:** Production operational support

#### Backup & Restore (2 weeks)
**Deliverables:**
- [ ] Logical backup (SQL dump)
- [ ] Binary backup
- [ ] Incremental backup
- [ ] Point-in-time recovery (PITR)

```bash
# Full backup
storemy-admin backup --database mydb --output /backups/mydb-full.tar.gz

# Incremental backup
storemy-admin backup --database mydb --incremental --output /backups/mydb-incr.tar.gz

# Restore
storemy-admin restore --backup /backups/mydb-full.tar.gz --database mydb_restored

# Point-in-time recovery
storemy-admin restore --backup /backups/mydb-full.tar.gz \
    --until "2025-11-15 14:30:00" --database mydb_restored
```

#### Monitoring & Metrics (2 weeks)
**Deliverables:**
- [ ] Query performance metrics
- [ ] Slow query log
- [ ] System statistics (cache hit ratio, I/O, locks)
- [ ] Prometheus metrics export
- [ ] Web-based dashboard

```sql
-- Performance views
SELECT * FROM pg_stat_activity;
SELECT * FROM pg_stat_database;
SELECT * FROM pg_stat_user_tables;

-- Slow query log
SET log_min_duration_statement = 1000;  -- Log queries > 1s
```

#### Administration Tools (2 weeks)
**Deliverables:**
- [ ] User authentication
- [ ] Role-based access control (RBAC)
- [ ] Permission management (GRANT/REVOKE)
- [ ] Configuration management
- [ ] Database maintenance utilities

```sql
-- User management
CREATE USER alice WITH PASSWORD 'secret';
CREATE ROLE admin;
GRANT admin TO alice;

-- Permissions
GRANT SELECT, INSERT ON users TO alice;
REVOKE DELETE ON orders FROM bob;

-- Role hierarchy
GRANT developer TO junior_dev;
GRANT ALL ON DATABASE mydb TO admin;
```

---

## Implementation Priorities

### Critical Path (Must Have for v1.0)
1. ✅ **Crash Recovery** (Phase 1) - Required for data durability
2. ✅ **Query Optimization** (Phase 2) - Required for acceptable performance
3. ✅ **Basic Constraints** (Phase 4, Week 1-4) - Required for data integrity

**Timeline:** 12-16 weeks (~3-4 months)

### High Priority (Should Have for v1.5)
4. ✅ **Subqueries & CTEs** (Phase 3) - Common SQL features
5. ✅ **Foreign Keys** (Phase 4, Week 5-6) - Referential integrity
6. ✅ **Client/Server** (Phase 6) - Multi-user support

**Timeline:** +16-20 weeks (~4-5 months)
**Total:** 28-36 weeks (~7-9 months)

### Medium Priority (Nice to Have for v2.0)
7. ⭕ **MVCC** (Phase 5) - Better concurrency
8. ⭕ **Views & Triggers** (Phase 7) - Advanced features
9. ⭕ **Replication** (Phase 6, optional) - High availability

**Timeline:** +16-20 weeks (~4-5 months)
**Total:** 44-56 weeks (~11-14 months)

### Low Priority (Future Enhancements)
10. ⭕ **Full-Text Search** (Phase 7) - Specialized feature
11. ⭕ **Stored Procedures** (Phase 7) - Programming in DB
12. ⭕ **Backup/Restore** (Phase 8) - Operational tools

**Timeline:** +12-16 weeks (~3-4 months)
**Total:** 56-72 weeks (~14-18 months)

---

## Resource Requirements

### Development Team
**Minimum viable:**
- 1 Senior Database Engineer (full-time)
- 1 QA Engineer (part-time)

**Optimal:**
- 2 Senior Database Engineers
- 1 DevOps Engineer
- 1 QA Engineer
- 1 Technical Writer (documentation)

### Infrastructure
- **Development:** Local machines + Docker
- **Testing:** CI/CD pipeline (GitHub Actions)
- **Staging:** 2-4 VMs for integration testing
- **Documentation:** GitHub Pages or similar

### Tools & Technologies
- **Language:** Go 1.21+
- **Testing:** Go test framework + custom benchmarks
- **Profiling:** pprof, trace, benchstat
- **Monitoring:** Prometheus + Grafana (Phase 8)
- **Documentation:** Markdown + MkDocs

---

## Success Metrics

### Phase 1 (Recovery) Success Criteria
- [ ] 100% data recovery after crash
- [ ] Recovery time < 10 seconds for typical workloads
- [ ] WAL overhead < 10% on writes
- [ ] Zero false negatives (lost committed data)
- [ ] Zero false positives (recovered uncommitted data)

### Phase 2 (Optimization) Success Criteria
- [ ] 10x+ speedup on complex TPC-H queries
- [ ] Cardinality estimates within 2x of actual
- [ ] Query plan cache hit rate > 80%
- [ ] < 5% regression on simple queries

### Phase 3 (SQL Completeness) Success Criteria
- [ ] 95%+ of TPC-H queries supported
- [ ] Subquery performance within 2x of manual joins
- [ ] CTE support for common use cases
- [ ] DISTINCT/UNION correct on all test cases

### Phase 4 (Integrity) Success Criteria
- [ ] All constraint violations caught
- [ ] Foreign key cascades transactional
- [ ] Performance overhead < 20%
- [ ] Zero constraint bypass bugs

### Phase 5 (MVCC) Success Criteria
- [ ] 3-10x read throughput improvement
- [ ] Readers never block writers
- [ ] Memory overhead < 50%
- [ ] Vacuum maintains space < 30% overhead

### Phase 6 (Client/Server) Success Criteria
- [ ] Support 100+ concurrent connections
- [ ] Protocol overhead < 5%
- [ ] Connection pooling works correctly
- [ ] Graceful shutdown with no data loss

### Overall v1.0 Success Criteria
- [ ] Pass all TPC-H queries
- [ ] Sustain 1000+ transactions/sec
- [ ] Handle 10GB+ databases
- [ ] Zero data loss on crash
- [ ] < 1 critical bug per quarter

---

## Risk Mitigation

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **MVCC complexity** | Medium | High | Incremental implementation, extensive testing |
| **Recovery bugs** | Medium | Critical | Crash injection testing, formal verification |
| **Performance regressions** | High | Medium | Continuous benchmarking, automated alerts |
| **Concurrency bugs** | Medium | High | Race detector, stress testing, code review |
| **WAL corruption** | Low | Critical | Checksums, redundant logging |

### Project Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Scope creep** | High | Medium | Strict phase gates, MVP focus |
| **Timeline slippage** | Medium | Medium | Buffer time, regular checkpoints |
| **Testing gaps** | Medium | High | TDD approach, coverage targets |
| **Documentation debt** | High | Low | Documentation in parallel with code |

---

## Quick Wins (1-3 days each)

Before starting major phases, implement these high-value, low-effort features:

### 1. EXPLAIN Command (1 day)
```sql
EXPLAIN SELECT * FROM users WHERE age > 18;
-- Output:
-- SeqScan on users (cost=0.00..50.00 rows=500)
--   Filter: (age > 18)
```

### 2. TRUNCATE TABLE (1 day)
```sql
TRUNCATE TABLE logs;  -- Much faster than DELETE
```

### 3. String Functions (2 days)
```sql
SELECT UPPER(name), LOWER(email), LENGTH(bio) FROM users;
```

### 4. CASE Expressions (2 days)
```sql
SELECT name,
    CASE
        WHEN age < 18 THEN 'Minor'
        WHEN age < 65 THEN 'Adult'
        ELSE 'Senior'
    END as category
FROM users;
```

### 5. Transaction Isolation Levels (2 days)
```sql
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
```

### 6. SHOW Commands (1 day)
```sql
SHOW TABLES;
SHOW INDEXES FROM users;
SHOW COLUMNS FROM users;
```

### 7. COALESCE & NULLIF (1 day)
```sql
SELECT COALESCE(phone, email, 'No contact') FROM users;
```

### 8. Date/Time Functions (3 days)
```sql
SELECT CURRENT_DATE, CURRENT_TIME, CURRENT_TIMESTAMP, NOW();
```

---

## Conclusion

StoreMy is a **highly mature educational database** with excellent foundations. This roadmap provides a clear path to transform it into a **production-ready system** over 14-18 months.

### Recommended Focus

**Next 6 Months:**
1. Crash Recovery (critical)
2. Query Optimization (high impact)
3. Basic Constraints (data integrity)
4. Subqueries (common SQL feature)

**Following 6-12 Months:**
5. Client/Server Architecture
6. MVCC (optional, but high value)
7. Advanced SQL (views, triggers)
8. Operational tools

### Final Recommendation

Start with **Phase 1 (Recovery)** immediately. This is the single most important feature for production use. Without crash recovery, the database cannot be trusted with real data, regardless of other features.

Once recovery is solid, move to **Phase 2 (Optimization)** to ensure good performance, then **Phase 3 (SQL Completeness)** to match user expectations.

**The project is in excellent shape to become a real production database!**

---

**Document Version:** 2.0
**Last Updated:** 2025-11-17
**Next Review:** After Phase 1 completion
**Maintained by:** Development Team
