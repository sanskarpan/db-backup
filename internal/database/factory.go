package database

import (
	"fmt"
	"sync"
)

// DriverFactory is a function that creates a new driver instance
type DriverFactory func() Driver

var (
	driversMu    sync.RWMutex
	driverRegistry = make(map[DatabaseType]DriverFactory)
)

// RegisterDriver registers a driver factory for a database type
func RegisterDriver(dbType DatabaseType, factory DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	driverRegistry[dbType] = factory
}

// CreateDriver creates a driver instance based on database type
func CreateDriver(dbType DatabaseType) (Driver, error) {
	driversMu.RLock()
	factory, ok := driverRegistry[dbType]
	driversMu.RUnlock()

	if !ok {
		return nil, &DriverError{
			Type:    dbType,
			Message: fmt.Sprintf("no driver registered for database type: %s", dbType),
		}
	}

	return factory(), nil
}
