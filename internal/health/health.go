package health

import (
	"sync"
	"time"
)

// HealthStatus represents health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// Check represents a health check
type Check struct {
	Name      string
	Status    HealthStatus
	Message   string
	LastCheck time.Time
	mu        sync.RWMutex
}

// HealthChecker manages health checks
type HealthChecker struct {
	checks map[string]*Check
	mu     sync.RWMutex
}

var (
	globalHealthChecker = &HealthChecker{
		checks: make(map[string]*Check),
	}
)

// GetHealthChecker returns global health checker
func GetHealthChecker() *HealthChecker {
	return globalHealthChecker
}

// RegisterCheck registers a health check
func (hc *HealthChecker) RegisterCheck(name string) *Check {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	check := &Check{
		Name:      name,
		Status:    HealthStatusHealthy,
		LastCheck: time.Now(),
	}
	
	hc.checks[name] = check
	return check
}

// GetCheck gets a health check
func (hc *HealthChecker) GetCheck(name string) (*Check, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	check, ok := hc.checks[name]
	return check, ok
}

// UpdateCheck updates a health check
func (c *Check) UpdateCheck(status HealthStatus, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.Status = status
	c.Message = message
	c.LastCheck = time.Now()
}

// GetStatus returns check status
func (c *Check) GetStatus() (HealthStatus, string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.Status, c.Message, c.LastCheck
}

// GetOverallStatus returns overall health status
func (hc *HealthChecker) GetOverallStatus() HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	if len(hc.checks) == 0 {
		return HealthStatusHealthy
	}
	
	hasUnhealthy := false
	hasDegraded := false
	
	for _, check := range hc.checks {
		status, _, _ := check.GetStatus()
		switch status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}
	
	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	
	return HealthStatusHealthy
}

// GetAllChecks returns all health checks
func (hc *HealthChecker) GetAllChecks() map[string]*Check {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	result := make(map[string]*Check)
	for name, check := range hc.checks {
		result[name] = check
	}
	return result
}

