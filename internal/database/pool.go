package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrPoolClosed is returned when attempting to use a closed pool
	ErrPoolClosed = errors.New("connection pool is closed")

	// ErrPoolExhausted is returned when pool has no available connections
	ErrPoolExhausted = errors.New("connection pool exhausted")

	// ErrConnectionUnhealthy is returned when connection health check fails
	ErrConnectionUnhealthy = errors.New("connection is unhealthy")

	// ErrTenantNotFound is returned when tenant pool doesn't exist
	ErrTenantNotFound = errors.New("tenant pool not found")
)

// PoolConfig defines connection pool configuration
type PoolConfig struct {
	// MinConnections is the minimum number of connections to maintain
	MinConnections int

	// MaxConnections is the maximum number of connections allowed
	MaxConnections int

	// MaxIdleTime is the maximum time a connection can be idle
	MaxIdleTime time.Duration

	// MaxLifetime is the maximum lifetime of a connection
	MaxLifetime time.Duration

	// HealthCheckInterval is how often to check connection health
	HealthCheckInterval time.Duration

	// ConnectionTimeout is the timeout for acquiring a connection
	ConnectionTimeout time.Duration

	// ReconnectDelay is the initial delay before reconnecting
	ReconnectDelay time.Duration

	// MaxReconnectDelay is the maximum delay between reconnection attempts
	MaxReconnectDelay time.Duration

	// AutoScale enables dynamic pool sizing
	AutoScale bool

	// ScaleUpThreshold is the utilization percentage to trigger scale up
	ScaleUpThreshold float64

	// ScaleDownThreshold is the utilization percentage to trigger scale down
	ScaleDownThreshold float64
}

// DefaultPoolConfig returns default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MinConnections:      2,
		MaxConnections:      10,
		MaxIdleTime:         10 * time.Minute,
		MaxLifetime:         30 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		ReconnectDelay:      1 * time.Second,
		MaxReconnectDelay:   30 * time.Second,
		AutoScale:           true,
		ScaleUpThreshold:    0.8,  // Scale up at 80% utilization
		ScaleDownThreshold:  0.3,  // Scale down at 30% utilization
	}
}

// PoolStats represents pool statistics
type PoolStats struct {
	// TotalConnections is the total number of connections
	TotalConnections int64

	// ActiveConnections is the number of connections in use
	ActiveConnections int64

	// IdleConnections is the number of idle connections
	IdleConnections int64

	// WaitCount is the number of times waited for a connection
	WaitCount int64

	// WaitDuration is the total time spent waiting
	WaitDuration time.Duration

	// MaxLifetimeReached is the number of connections closed due to max lifetime
	MaxLifetimeReached int64

	// HealthCheckFailed is the number of failed health checks
	HealthCheckFailed int64

	// ReconnectAttempts is the number of reconnection attempts
	ReconnectAttempts int64

	// ReconnectSuccess is the number of successful reconnections
	ReconnectSuccess int64
}

// ConnectionPool manages a pool of database connections
type ConnectionPool struct {
	config   PoolConfig
	connFunc func(context.Context) (*sql.DB, error)

	mu          sync.RWMutex
	connections []*pooledConnection
	closed      bool

	stats       atomic.Value // *PoolStats

	healthTicker *time.Ticker
	scaleTicker  *time.Ticker
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// pooledConnection wraps a database connection with metadata
type pooledConnection struct {
	db         *sql.DB
	createdAt  time.Time
	lastUsed   time.Time
	inUse      bool
	healthy    bool
	failCount  int
	mu         sync.RWMutex
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config PoolConfig, connFunc func(context.Context) (*sql.DB, error)) (*ConnectionPool, error) {
	if config.MinConnections < 1 {
		return nil, errors.New("minimum connections must be at least 1")
	}
	if config.MaxConnections < config.MinConnections {
		return nil, errors.New("maximum connections must be >= minimum connections")
	}

	pool := &ConnectionPool{
		config:      config,
		connFunc:    connFunc,
		connections: make([]*pooledConnection, 0, config.MaxConnections),
		stopCh:      make(chan struct{}),
	}

	// Initialize stats
	pool.stats.Store(&PoolStats{})

	// Create minimum connections
	ctx := context.Background()
	for i := 0; i < config.MinConnections; i++ {
		if err := pool.createConnection(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to create initial connection: %w", err)
		}
	}

	// Start background workers
	pool.startHealthCheck()
	if config.AutoScale {
		pool.startAutoScaling()
	}

	return pool, nil
}

// Get acquires a connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (*sql.DB, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Try to find an idle healthy connection
	for _, conn := range p.connections {
		conn.mu.Lock()
		if !conn.inUse && conn.healthy {
			conn.inUse = true
			conn.lastUsed = time.Now()
			conn.mu.Unlock()

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.ActiveConnections, 1)
				atomic.AddInt64(&s.IdleConnections, -1)
			})

			p.mu.Unlock()
			return conn.db, nil
		}
		conn.mu.Unlock()
	}

	// No idle connection available - try to create new one
	if len(p.connections) < p.config.MaxConnections {
		if err := p.createConnection(ctx); err == nil {
			// Retry getting the newly created connection
			conn := p.connections[len(p.connections)-1]
			conn.mu.Lock()
			conn.inUse = true
			conn.lastUsed = time.Now()
			conn.mu.Unlock()

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.ActiveConnections, 1)
			})

			p.mu.Unlock()
			return conn.db, nil
		}
	}

	p.mu.Unlock()

	// Wait for a connection to become available
	return p.waitForConnection(ctx)
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(db *sql.DB) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return ErrPoolClosed
	}

	for _, conn := range p.connections {
		conn.mu.Lock()
		if conn.db == db && conn.inUse {
			conn.inUse = false
			conn.lastUsed = time.Now()
			conn.mu.Unlock()

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.ActiveConnections, -1)
				atomic.AddInt64(&s.IdleConnections, 1)
			})

			return nil
		}
		conn.mu.Unlock()
	}

	return errors.New("connection not found in pool")
}

// Stats returns current pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	stats := p.stats.Load().(*PoolStats)

	p.mu.RLock()
	defer p.mu.RUnlock()

	var total, active, idle int64
	for _, conn := range p.connections {
		conn.mu.RLock()
		total++
		if conn.inUse {
			active++
		} else {
			idle++
		}
		conn.mu.RUnlock()
	}

	// Update current connection counts
	result := *stats
	result.TotalConnections = total
	result.ActiveConnections = active
	result.IdleConnections = idle

	return result
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.stopCh)

	// Stop background workers
	if p.healthTicker != nil {
		p.healthTicker.Stop()
	}
	if p.scaleTicker != nil {
		p.scaleTicker.Stop()
	}

	// Wait for background workers to finish
	p.wg.Wait()

	// Close all connections
	var errs []error
	for _, conn := range p.connections {
		if err := conn.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	p.connections = nil

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}

// createConnection creates a new connection and adds it to the pool
func (p *ConnectionPool) createConnection(ctx context.Context) error {
	db, err := p.connFunc(ctx)
	if err != nil {
		return err
	}

	conn := &pooledConnection{
		db:        db,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		healthy:   true,
	}

	p.connections = append(p.connections, conn)

	p.updateStats(func(s *PoolStats) {
		atomic.AddInt64(&s.TotalConnections, 1)
		atomic.AddInt64(&s.IdleConnections, 1)
	})

	return nil
}

// waitForConnection waits for a connection to become available
func (p *ConnectionPool) waitForConnection(ctx context.Context) (*sql.DB, error) {
	waitStart := time.Now()

	p.updateStats(func(s *PoolStats) {
		atomic.AddInt64(&s.WaitCount, 1)
	})

	timeout := p.config.ConnectionTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, ErrPoolExhausted
		case <-ticker.C:
			db, err := p.tryGetConnection()
			if err == nil {
				waitDuration := time.Since(waitStart)
				p.updateStats(func(s *PoolStats) {
					s.WaitDuration += waitDuration
				})
				return db, nil
			}
		}
	}
}

// tryGetConnection attempts to get a connection without waiting
func (p *ConnectionPool) tryGetConnection() (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrPoolClosed
	}

	for _, conn := range p.connections {
		conn.mu.Lock()
		if !conn.inUse && conn.healthy {
			conn.inUse = true
			conn.lastUsed = time.Now()
			conn.mu.Unlock()

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.ActiveConnections, 1)
				atomic.AddInt64(&s.IdleConnections, -1)
			})

			return conn.db, nil
		}
		conn.mu.Unlock()
	}

	return nil, ErrPoolExhausted
}

// startHealthCheck starts the health check worker
func (p *ConnectionPool) startHealthCheck() {
	p.healthTicker = time.NewTicker(p.config.HealthCheckInterval)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			select {
			case <-p.stopCh:
				return
			case <-p.healthTicker.C:
				p.healthCheck()
			}
		}
	}()
}

// healthCheck performs health checks on all connections
func (p *ConnectionPool) healthCheck() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var toRemove []*pooledConnection

	for _, conn := range p.connections {
		conn.mu.Lock()

		// Skip connections in use
		if conn.inUse {
			conn.mu.Unlock()
			continue
		}

		// Check max lifetime (but keep minimum connections)
		if p.config.MaxLifetime > 0 && now.Sub(conn.createdAt) > p.config.MaxLifetime {
			if len(p.connections)-len(toRemove) > p.config.MinConnections {
				toRemove = append(toRemove, conn)
				p.updateStats(func(s *PoolStats) {
					atomic.AddInt64(&s.MaxLifetimeReached, 1)
				})
				conn.mu.Unlock()
				continue
			} else {
				// Replace the connection instead of removing it
				toRemove = append(toRemove, conn)
				p.updateStats(func(s *PoolStats) {
					atomic.AddInt64(&s.MaxLifetimeReached, 1)
				})
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					p.createConnection(ctx)
				}()
				conn.mu.Unlock()
				continue
			}
		}

		// Check max idle time
		if p.config.MaxIdleTime > 0 && now.Sub(conn.lastUsed) > p.config.MaxIdleTime {
			if len(p.connections)-len(toRemove) > p.config.MinConnections {
				toRemove = append(toRemove, conn)
				conn.mu.Unlock()
				continue
			}
		}

		// Perform ping health check
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := conn.db.PingContext(ctx)
		cancel()

		if err != nil {
			conn.healthy = false
			conn.failCount++

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.HealthCheckFailed, 1)
			})

			// Try to reconnect if fail count is high
			if conn.failCount >= 3 {
				toRemove = append(toRemove, conn)

				// Attempt to create replacement connection
				go p.reconnect(conn)
			}
		} else {
			conn.healthy = true
			conn.failCount = 0
		}

		conn.mu.Unlock()
	}

	// Remove unhealthy connections
	for _, conn := range toRemove {
		p.removeConnection(conn)
	}
}

// reconnect attempts to reconnect a failed connection
func (p *ConnectionPool) reconnect(oldConn *pooledConnection) {
	p.updateStats(func(s *PoolStats) {
		atomic.AddInt64(&s.ReconnectAttempts, 1)
	})

	delay := p.config.ReconnectDelay
	maxDelay := p.config.MaxReconnectDelay

	for attempt := 0; attempt < 5; attempt++ {
		select {
		case <-p.stopCh:
			return
		case <-time.After(delay):
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := p.createConnection(ctx)
			cancel()

			if err == nil {
				p.updateStats(func(s *PoolStats) {
					atomic.AddInt64(&s.ReconnectSuccess, 1)
				})
				return
			}

			// Exponential backoff
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// removeConnection removes a connection from the pool
func (p *ConnectionPool) removeConnection(conn *pooledConnection) {
	for i, c := range p.connections {
		if c == conn {
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			conn.db.Close()

			p.updateStats(func(s *PoolStats) {
				atomic.AddInt64(&s.TotalConnections, -1)
				atomic.AddInt64(&s.IdleConnections, -1)
			})

			break
		}
	}
}

// startAutoScaling starts the auto-scaling worker
func (p *ConnectionPool) startAutoScaling() {
	p.scaleTicker = time.NewTicker(30 * time.Second)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			select {
			case <-p.stopCh:
				return
			case <-p.scaleTicker.C:
				p.autoScale()
			}
		}
	}()
}

// autoScale automatically adjusts pool size based on utilization
func (p *ConnectionPool) autoScale() {
	stats := p.Stats()

	if stats.TotalConnections == 0 {
		return
	}

	utilization := float64(stats.ActiveConnections) / float64(stats.TotalConnections)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Scale up if utilization is high
	if utilization >= p.config.ScaleUpThreshold && len(p.connections) < p.config.MaxConnections {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		p.createConnection(ctx)
		cancel()
	}

	// Scale down if utilization is low
	if utilization <= p.config.ScaleDownThreshold && len(p.connections) > p.config.MinConnections {
		// Find an idle connection to remove
		for _, conn := range p.connections {
			conn.mu.RLock()
			inUse := conn.inUse
			conn.mu.RUnlock()

			if !inUse {
				p.removeConnection(conn)
				break
			}
		}
	}
}

// updateStats updates pool statistics atomically
func (p *ConnectionPool) updateStats(fn func(*PoolStats)) {
	for {
		oldStats := p.stats.Load().(*PoolStats)
		newStats := *oldStats
		fn(&newStats)
		if p.stats.CompareAndSwap(oldStats, &newStats) {
			break
		}
	}
}

// MultiTenantPool manages separate connection pools for multiple tenants
type MultiTenantPool struct {
	mu     sync.RWMutex
	pools  map[string]*ConnectionPool
	config PoolConfig
}

// NewMultiTenantPool creates a new multi-tenant pool manager
func NewMultiTenantPool(config PoolConfig) *MultiTenantPool {
	return &MultiTenantPool{
		pools:  make(map[string]*ConnectionPool),
		config: config,
	}
}

// GetPool returns the connection pool for a tenant, creating it if needed
func (m *MultiTenantPool) GetPool(tenantID string, connFunc func(context.Context) (*sql.DB, error)) (*ConnectionPool, error) {
	m.mu.RLock()
	pool, exists := m.pools[tenantID]
	m.mu.RUnlock()

	if exists {
		return pool, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, exists := m.pools[tenantID]; exists {
		return pool, nil
	}

	// Create new pool for tenant
	pool, err := NewConnectionPool(m.config, connFunc)
	if err != nil {
		return nil, err
	}

	m.pools[tenantID] = pool
	return pool, nil
}

// RemovePool removes a tenant's connection pool
func (m *MultiTenantPool) RemovePool(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.pools[tenantID]
	if !exists {
		return ErrTenantNotFound
	}

	delete(m.pools, tenantID)
	return pool.Close()
}

// Close closes all tenant pools
func (m *MultiTenantPool) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for tenantID, pool := range m.pools {
		if err := pool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenantID, err))
		}
	}

	m.pools = make(map[string]*ConnectionPool)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing pools: %v", errs)
	}

	return nil
}

// Stats returns statistics for all tenant pools
func (m *MultiTenantPool) Stats() map[string]PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for tenantID, pool := range m.pools {
		stats[tenantID] = pool.Stats()
	}

	return stats
}
