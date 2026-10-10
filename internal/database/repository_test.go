package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"host-monitor/internal/monitor"
)

func TestRepository_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_monitor.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	// 1. Test AddHost
	h1, err := repo.AddHost(ctx, "example.com", monitor.HostDomain)
	if err != nil {
		t.Fatalf("failed to add host: %v", err)
	}
	if h1.Host != "example.com" || h1.HostType != monitor.HostDomain || !h1.Enabled {
		t.Errorf("unexpected host data: %+v", h1)
	}

	// 2. Test Duplicate AddHost
	_, err = repo.AddHost(ctx, "example.com", monitor.HostDomain)
	if err != ErrHostAlreadyExists {
		t.Errorf("expected ErrHostAlreadyExists, got: %v", err)
	}

	// 3. Test CountHosts
	count, err := repo.CountHosts(ctx)
	if err != nil || count != 1 {
		t.Errorf("expected count 1, got %d (err: %v)", count, err)
	}

	// 4. Test Add Second Host
	h2, err := repo.AddHost(ctx, "1.1.1.1", monitor.HostIPv4)
	if err != nil {
		t.Fatalf("failed to add second host: %v", err)
	}
	if h2.Host != "1.1.1.1" {
		t.Errorf("unexpected host: %+v", h2)
	}

	// 5. Test ListHosts
	hosts, err := repo.ListHosts(ctx)
	if err != nil || len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d (err: %v)", len(hosts), err)
	}

	// 6. Test UpdateHostStatus
	now := time.Now()
	err = repo.UpdateHostStatus(ctx, h1.ID, monitor.StatusOnline, 200, 120, now, true)
	if err != nil {
		t.Fatalf("failed to update host status: %v", err)
	}

	fetched, err := repo.GetHost(ctx, "example.com")
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	if fetched.LastStatus != monitor.StatusOnline || fetched.LastHTTPStatus != 200 || fetched.LastResponseTime != 120 {
		t.Errorf("unexpected fetched status: %+v", fetched)
	}

	// 7. Test InsertCheck & Retention Clean
	err = repo.InsertCheck(ctx, monitor.DatabaseCheck{
		HostID:     h1.ID,
		Status:     monitor.StatusOnline,
		CheckType:  "HTTP",
		HTTPStatus: 200,
		LatencyMs:  120,
		CheckedAt:  time.Now().AddDate(0, 0, -10), // 10 days ago
	})
	if err != nil {
		t.Fatalf("failed to insert check history: %v", err)
	}

	deleted, err := repo.CleanOldChecks(ctx, 7)
	if err != nil {
		t.Fatalf("failed to clean old checks: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 row deleted by retention policy, got %d", deleted)
	}

	// 8. Test SetHostEnabled (unmonitor)
	err = repo.SetHostEnabled(ctx, "example.com", false)
	if err != nil {
		t.Fatalf("failed to set host enabled: %v", err)
	}

	activeHosts, err := repo.GetActiveHosts(ctx)
	if err != nil || len(activeHosts) != 1 {
		t.Fatalf("expected 1 active host, got %d", len(activeHosts))
	}
	if activeHosts[0].Host != "1.1.1.1" {
		t.Errorf("expected active host to be 1.1.1.1, got %s", activeHosts[0].Host)
	}

	// 9. Test RemoveHost
	err = repo.RemoveHost(ctx, "example.com")
	if err != nil {
		t.Fatalf("failed to remove host: %v", err)
	}

	_, err = repo.GetHost(ctx, "example.com")
	if err != ErrHostNotFound {
		t.Errorf("expected ErrHostNotFound, got %v", err)
	}
}

func TestRepository_Uptime(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "uptime_test.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	h, err := repo.AddHost(ctx, "uptime.example.com", monitor.HostDomain)
	if err != nil {
		t.Fatalf("failed to add host: %v", err)
	}

	// Insert 9 ONLINE checks and 1 DOWN check (total 10)
	now := time.Now()
	for i := 0; i < 9; i++ {
		err = repo.InsertCheck(ctx, monitor.DatabaseCheck{
			HostID:     h.ID,
			Status:     monitor.StatusOnline,
			CheckType:  "HTTP",
			HTTPStatus: 200,
			LatencyMs:  50,
			CheckedAt:  now.Add(-time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("failed to insert check: %v", err)
		}
	}

	err = repo.InsertCheck(ctx, monitor.DatabaseCheck{
		HostID:     h.ID,
		Status:     monitor.StatusDown,
		CheckType:  "HTTP",
		HTTPStatus: 500,
		LatencyMs:  0,
		CheckedAt:  now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to insert down check: %v", err)
	}

	// Query uptime for last 24h
	stats, err := repo.GetHostUptime(ctx, h.ID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("GetHostUptime failed: %v", err)
	}

	if stats.TotalChecks != 10 {
		t.Errorf("expected 10 checks, got %d", stats.TotalChecks)
	}
	if stats.OnlineChecks != 9 {
		t.Errorf("expected 9 online checks, got %d", stats.OnlineChecks)
	}
	if stats.DownChecks != 1 {
		t.Errorf("expected 1 down check, got %d", stats.DownChecks)
	}
	if stats.UptimePct != 90.0 {
		t.Errorf("expected 90.0%% uptime, got %.2f%%", stats.UptimePct)
	}
	if stats.AvgLatencyMs != 50 {
		t.Errorf("expected 50ms avg latency, got %d", stats.AvgLatencyMs)
	}

	// Test GetAllHostsUptime
	allStats, err := repo.GetAllHostsUptime(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("GetAllHostsUptime failed: %v", err)
	}
	if len(allStats) != 1 {
		t.Fatalf("expected 1 host in allStats, got %d", len(allStats))
	}
	if allStats[0].UptimePct != 90.0 {
		t.Errorf("expected 90.0%% uptime in allStats, got %.2f%%", allStats[0].UptimePct)
	}
}
