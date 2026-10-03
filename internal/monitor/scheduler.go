package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"host-monitor/internal/config"
)

// HostRepository defines the interface for database operations needed by the scheduler.
type HostRepository interface {
	GetActiveHosts(ctx context.Context) ([]DatabaseHost, error)
	UpdateHostStatus(ctx context.Context, id int64, status OverallStatus, httpStatus int, responseTime int64, checkedAt time.Time, isOnline bool) error
	InsertCheck(ctx context.Context, check DatabaseCheck) error
	CleanOldChecks(ctx context.Context, retentionDays int) (int64, error)
}

// StatusNotifier defines the interface for alert dispatching.
type StatusNotifier interface {
	NotifyStatusChange(ctx context.Context, host string, oldStatus, newStatus OverallStatus, latencyMs int64, downSince *time.Time)
}

// DatabaseHost represents host data needed for monitoring.
type DatabaseHost struct {
	ID               int64
	Host             string
	HostType         HostType
	Enabled          bool
	LastStatus       OverallStatus
	LastHTTPStatus   int
	LastResponseTime int64
	LastCheckedAt    *time.Time
	LastOnlineAt     *time.Time
}

// DatabaseCheck represents check history payload.
type DatabaseCheck struct {
	HostID       int64
	Status       OverallStatus
	CheckType    string
	HTTPStatus   int
	LatencyMs    int64
	Port         int
	ErrorMessage string
	CheckedAt    time.Time
}

// Scheduler coordinates automatic recurring monitoring cycles.
type Scheduler struct {
	cfg      *config.Config
	checker  *Checker
	repo     HostRepository
	notifier StatusNotifier
}

// NewScheduler creates a new Scheduler instance.
func NewScheduler(cfg *config.Config, checker *Checker, repo HostRepository, notifier StatusNotifier) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		checker:  checker,
		repo:     repo,
		notifier: notifier,
	}
}

// Start runs the periodic monitoring ticker loop until context cancellation.
func (s *Scheduler) Start(ctx context.Context) {
	interval := 1 * time.Minute
	if s.cfg != nil && s.cfg.CheckInterval > 0 {
		interval = s.cfg.CheckInterval
	}

	slog.Info("Starting automatic monitoring scheduler", "interval", interval, "max_concurrency", s.cfg.MaxConcurrentChecks)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	retentionTicker := time.NewTicker(24 * time.Hour)
	defer retentionTicker.Stop()

	// Run first check immediately upon startup
	s.RunCheckCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping monitoring scheduler...")
			return
		case <-ticker.C:
			s.RunCheckCycle(ctx)
		case <-retentionTicker.C:
			s.runRetentionCleanup(ctx)
		}
	}
}

// RunCheckCycle executes one cycle of checks across all active hosts using a worker pool.
func (s *Scheduler) RunCheckCycle(ctx context.Context) {
	if s.repo == nil {
		return
	}

	hosts, err := s.repo.GetActiveHosts(ctx)
	if err != nil {
		slog.Error("Scheduler failed to load active hosts", "error", err)
		return
	}

	if len(hosts) == 0 {
		return
	}

	slog.Info("Running scheduled check cycle", "monitored_hosts_count", len(hosts))

	maxWorkers := 5
	if s.cfg != nil && s.cfg.MaxConcurrentChecks > 0 {
		maxWorkers = s.cfg.MaxConcurrentChecks
	}

	hostChan := make(chan DatabaseHost, len(hosts))
	for _, h := range hosts {
		hostChan <- h
	}
	close(hostChan)

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case h, ok := <-hostChan:
					if !ok {
						return
					}
					s.checkSingleHost(ctx, h)
				}
			}
		}()
	}

	wg.Wait()
}

func (s *Scheduler) checkSingleHost(ctx context.Context, h DatabaseHost) {
	result := s.checker.CheckHost(ctx, h.Host, h.HostType)
	now := time.Now()

	isOnline := result.Status == StatusOnline

	var primaryLatency int64
	var primaryHTTPStatus int

	if h.HostType == HostDomain {
		if result.HTTPS.Attempted && result.HTTPS.Latency > 0 {
			primaryLatency = result.HTTPS.Latency.Milliseconds()
			primaryHTTPStatus = result.HTTPS.StatusCode
		} else if result.HTTP.Attempted && result.HTTP.Latency > 0 {
			primaryLatency = result.HTTP.Latency.Milliseconds()
			primaryHTTPStatus = result.HTTP.StatusCode
		}
	} else {
		if result.Ping.Reachable {
			primaryLatency = result.Ping.Latency.Milliseconds()
		}
	}

	// 1. Notify status change if transition detected
	if s.notifier != nil && h.LastStatus != "" && h.LastStatus != "UNKNOWN" {
		if (h.LastStatus == StatusOnline && !isOnline) || (h.LastStatus != StatusOnline && isOnline) {
			s.notifier.NotifyStatusChange(ctx, h.Host, h.LastStatus, result.Status, primaryLatency, h.LastOnlineAt)
		}
	}

	// 2. Save result to DB
	_ = s.repo.UpdateHostStatus(ctx, h.ID, result.Status, primaryHTTPStatus, primaryLatency, now, isOnline)

	// 3. Save to checks audit log
	_ = s.repo.InsertCheck(ctx, DatabaseCheck{
		HostID:     h.ID,
		Status:     result.Status,
		CheckType:  string(h.HostType),
		HTTPStatus: primaryHTTPStatus,
		LatencyMs:  primaryLatency,
		CheckedAt:  now,
	})
}

func (s *Scheduler) runRetentionCleanup(ctx context.Context) {
	if s.repo == nil || s.cfg == nil {
		return
	}
	deleted, err := s.repo.CleanOldChecks(ctx, s.cfg.RetentionDays)
	if err != nil {
		slog.Error("Failed to clean old check history", "error", err)
	} else if deleted > 0 {
		slog.Info("Cleaned up old check logs", "deleted_rows", deleted, "retention_days", s.cfg.RetentionDays)
	}
}
