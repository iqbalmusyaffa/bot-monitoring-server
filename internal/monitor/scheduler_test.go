package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	"host-monitor/internal/config"
)

type mockRepo struct {
	mu     sync.Mutex
	hosts  []DatabaseHost
	checks []DatabaseCheck
}

func (m *mockRepo) GetActiveHosts(ctx context.Context) ([]DatabaseHost, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hosts, nil
}

func (m *mockRepo) UpdateHostStatus(ctx context.Context, id int64, status OverallStatus, httpStatus int, responseTime int64, checkedAt time.Time, isOnline bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.hosts {
		if m.hosts[i].ID == id {
			m.hosts[i].LastStatus = status
			m.hosts[i].LastHTTPStatus = httpStatus
			m.hosts[i].LastResponseTime = responseTime
			m.hosts[i].LastCheckedAt = &checkedAt
			if isOnline {
				m.hosts[i].LastOnlineAt = &checkedAt
			}
		}
	}
	return nil
}

func (m *mockRepo) InsertCheck(ctx context.Context, check DatabaseCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checks = append(m.checks, check)
	return nil
}

func (m *mockRepo) CleanOldChecks(ctx context.Context, retentionDays int) (int64, error) {
	return 0, nil
}

type mockNotifier struct {
	mu          sync.Mutex
	alertsCount int
	lastHost    string
	lastOld     OverallStatus
	lastNew     OverallStatus
}

func (n *mockNotifier) NotifyStatusChange(ctx context.Context, host string, oldStatus, newStatus OverallStatus, latencyMs int64, downSince *time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.alertsCount++
	n.lastHost = host
	n.lastOld = oldStatus
	n.lastNew = newStatus
}

func TestScheduler_TransitionAlert(t *testing.T) {
	cfg := &config.Config{
		CheckInterval:       100 * time.Millisecond,
		MaxConcurrentChecks: 2,
		RequestTimeout:      1 * time.Second,
	}

	repo := &mockRepo{
		hosts: []DatabaseHost{
			{
				ID:         1,
				Host:       "198.51.100.1", // Non-routable IP (DOWN)
				HostType:   HostIPv4,
				Enabled:    true,
				LastStatus: StatusOnline, // Previously ONLINE
			},
		},
	}

	notifier := &mockNotifier{}
	checker := NewChecker(cfg)
	scheduler := NewScheduler(cfg, checker, repo, notifier)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	scheduler.RunCheckCycle(ctx)

	notifier.mu.Lock()
	alerts := notifier.alertsCount
	host := notifier.lastHost
	oldSt := notifier.lastOld
	newSt := notifier.lastNew
	notifier.mu.Unlock()

	if alerts != 1 {
		t.Errorf("expected 1 alert triggered for ONLINE -> DOWN, got %d", alerts)
	}
	if host != "198.51.100.1" || oldSt != StatusOnline || newSt == StatusOnline {
		t.Errorf("unexpected transition alert details: host=%s, old=%s, new=%s", host, oldSt, newSt)
	}
}
