package wal

import (
	"fmt"
	"storemy/pkg/primitives"
	"sync"
	"sync/atomic"
	"time"
)

// CheckpointDaemon manages automatic checkpoint triggering
type CheckpointDaemon struct {
	wal           *WAL
	config        CheckpointConfig
	stopChan      chan struct{}
	wg            sync.WaitGroup
	running       atomic.Bool
	lastCheckpoint atomic.Value // stores time.Time
	stats         CheckpointDaemonStats
	statsMutex    sync.RWMutex
}

// CheckpointConfig configures checkpoint triggering behavior
type CheckpointConfig struct {
	// Time-based trigger: checkpoint every Interval
	Interval time.Duration

	// Size-based trigger: checkpoint when WAL exceeds MaxWALSize bytes
	MaxWALSize int64

	// Transaction-based trigger: checkpoint every MaxTransactions commits
	MaxTransactions int64

	// Enable automatic checkpointing
	Enabled bool
}

// DefaultCheckpointConfig returns a sensible default configuration
func DefaultCheckpointConfig() CheckpointConfig {
	return CheckpointConfig{
		Interval:        10 * time.Minute,
		MaxWALSize:      10 * 1024 * 1024, // 10MB
		MaxTransactions: 1000,
		Enabled:         true,
	}
}

// CheckpointDaemonStats tracks daemon statistics
type CheckpointDaemonStats struct {
	TotalCheckpoints     int64
	TimeBasedTriggers    int64
	SizeBasedTriggers    int64
	ManualTriggers       int64
	FailedCheckpoints    int64
	LastCheckpointTime   time.Time
	LastCheckpointLSN    primitives.LSN
	LastCheckpointDuration time.Duration
}

// NewCheckpointDaemon creates a new checkpoint daemon
func NewCheckpointDaemon(wal *WAL, config CheckpointConfig) *CheckpointDaemon {
	daemon := &CheckpointDaemon{
		wal:      wal,
		config:   config,
		stopChan: make(chan struct{}),
	}
	daemon.lastCheckpoint.Store(time.Now())
	return daemon
}

// Start begins the checkpoint daemon
func (cd *CheckpointDaemon) Start() error {
	if !cd.config.Enabled {
		fmt.Println("Checkpoint daemon disabled")
		return nil
	}

	if !cd.running.CompareAndSwap(false, true) {
		return fmt.Errorf("checkpoint daemon already running")
	}

	fmt.Printf("Starting checkpoint daemon (interval=%v, maxWALSize=%d bytes)\n",
		cd.config.Interval, cd.config.MaxWALSize)

	cd.wg.Add(1)
	go cd.run()

	return nil
}

// Stop gracefully stops the checkpoint daemon
func (cd *CheckpointDaemon) Stop() error {
	if !cd.running.Load() {
		return nil
	}

	fmt.Println("Stopping checkpoint daemon...")
	close(cd.stopChan)
	cd.wg.Wait()
	cd.running.Store(false)
	fmt.Println("Checkpoint daemon stopped")

	return nil
}

// run is the main daemon loop
func (cd *CheckpointDaemon) run() {
	defer cd.wg.Done()

	ticker := time.NewTicker(cd.config.Interval)
	defer ticker.Stop()

	// Also check more frequently for size-based triggers
	checkTicker := time.NewTicker(30 * time.Second)
	defer checkTicker.Stop()

	for {
		select {
		case <-cd.stopChan:
			return

		case <-ticker.C:
			// Time-based trigger
			if cd.shouldCheckpointByTime() {
				cd.triggerCheckpoint("time-based")
				cd.statsMutex.Lock()
				cd.stats.TimeBasedTriggers++
				cd.statsMutex.Unlock()
			}

		case <-checkTicker.C:
			// Check size-based trigger
			if cd.shouldCheckpointBySize() {
				cd.triggerCheckpoint("size-based")
				cd.statsMutex.Lock()
				cd.stats.SizeBasedTriggers++
				cd.statsMutex.Unlock()
			}
		}
	}
}

// shouldCheckpointByTime checks if enough time has passed since last checkpoint
func (cd *CheckpointDaemon) shouldCheckpointByTime() bool {
	lastCheckpoint := cd.lastCheckpoint.Load().(time.Time)
	return time.Since(lastCheckpoint) >= cd.config.Interval
}

// shouldCheckpointBySize checks if WAL has grown too large
func (cd *CheckpointDaemon) shouldCheckpointBySize() bool {
	if cd.config.MaxWALSize <= 0 {
		return false
	}

	return cd.wal.ShouldCheckpoint(cd.config.MaxWALSize, 0)
}

// triggerCheckpoint performs a checkpoint
func (cd *CheckpointDaemon) triggerCheckpoint(reason string) {
	fmt.Printf("Triggering checkpoint (reason: %s)...\n", reason)
	startTime := time.Now()

	lsn, err := cd.wal.WriteCheckpoint()
	duration := time.Since(startTime)

	cd.statsMutex.Lock()
	defer cd.statsMutex.Unlock()

	if err != nil {
		fmt.Printf("Checkpoint failed: %v\n", err)
		cd.stats.FailedCheckpoints++
		return
	}

	// Update statistics
	cd.stats.TotalCheckpoints++
	cd.stats.LastCheckpointTime = startTime
	cd.stats.LastCheckpointLSN = lsn
	cd.stats.LastCheckpointDuration = duration
	cd.lastCheckpoint.Store(startTime)

	fmt.Printf("Checkpoint completed in %v (LSN=%d)\n", duration, lsn)
}

// TriggerManualCheckpoint manually triggers a checkpoint
// This is useful for testing or administrative operations
func (cd *CheckpointDaemon) TriggerManualCheckpoint() (primitives.LSN, error) {
	fmt.Println("Manual checkpoint triggered")

	startTime := time.Now()
	lsn, err := cd.wal.WriteCheckpoint()
	duration := time.Since(startTime)

	cd.statsMutex.Lock()
	defer cd.statsMutex.Unlock()

	if err != nil {
		cd.stats.FailedCheckpoints++
		return 0, fmt.Errorf("manual checkpoint failed: %w", err)
	}

	cd.stats.TotalCheckpoints++
	cd.stats.ManualTriggers++
	cd.stats.LastCheckpointTime = startTime
	cd.stats.LastCheckpointLSN = lsn
	cd.stats.LastCheckpointDuration = duration
	cd.lastCheckpoint.Store(startTime)

	return lsn, nil
}

// GetStats returns current daemon statistics
func (cd *CheckpointDaemon) GetStats() CheckpointDaemonStats {
	cd.statsMutex.RLock()
	defer cd.statsMutex.RUnlock()
	return cd.stats
}

// IsRunning returns true if the daemon is currently running
func (cd *CheckpointDaemon) IsRunning() bool {
	return cd.running.Load()
}

// GetConfig returns the current configuration
func (cd *CheckpointDaemon) GetConfig() CheckpointConfig {
	return cd.config
}

// UpdateConfig updates the daemon configuration
// Note: This does not affect the running daemon - you must restart it
func (cd *CheckpointDaemon) UpdateConfig(config CheckpointConfig) {
	cd.config = config
}
