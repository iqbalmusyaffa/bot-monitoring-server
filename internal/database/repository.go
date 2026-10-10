package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"host-monitor/internal/monitor"
)

var (
	// ErrHostNotFound is returned when a host is not found in database.
	ErrHostNotFound = errors.New("host not found")
	// ErrHostAlreadyExists is returned when attempting to add a duplicate host.
	ErrHostAlreadyExists = errors.New("host already exists in monitoring list")
	// ErrUserNotFound is returned when an admin user is not found.
	ErrUserNotFound = errors.New("user not found")
)

// UserRole represents permission level.
type UserRole string

const (
	RoleOwner UserRole = "OWNER"
	RoleAdmin UserRole = "ADMIN"
	RoleUser  UserRole = "USER"
)

// Host represents a row in the hosts table.
type Host struct {
	ID               int64
	Host             string
	HostType         monitor.HostType
	Enabled          bool
	LastStatus       monitor.OverallStatus
	LastHTTPStatus   int
	LastResponseTime int64
	LastCheckedAt    *time.Time
	LastOnlineAt     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AdminUser represents a registered user in the database.
type AdminUser struct {
	UserID    int64
	FirstName string
	Username  string
	Role      UserRole
	IsOwner   bool
	CreatedAt time.Time
}

// Repository handles database operations for hosts, checks, and admin users.
type Repository struct {
	db *DB
}

// NewRepository creates a new Repository.
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// AddHost inserts a new host into the database.
func (r *Repository) AddHost(ctx context.Context, host string, hType monitor.HostType) (*Host, error) {
	query := `
	INSERT INTO hosts (host, host_type, enabled, last_status, created_at, updated_at)
	VALUES (?, ?, 1, 'UNKNOWN', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	RETURNING id, host, host_type, enabled, last_status, last_http_status, last_response_time, last_checked_at, last_online_at, created_at, updated_at;
	`
	var h Host
	var hTypeStr string
	var lastStatusStr string
	var lastCheckedAt, lastOnlineAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, host, string(hType)).Scan(
		&h.ID, &h.Host, &hTypeStr, &h.Enabled, &lastStatusStr,
		&h.LastHTTPStatus, &h.LastResponseTime, &lastCheckedAt, &lastOnlineAt,
		&h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrHostAlreadyExists
		}
		return nil, fmt.Errorf("failed to add host: %w", err)
	}

	h.HostType = monitor.HostType(hTypeStr)
	h.LastStatus = monitor.OverallStatus(lastStatusStr)
	if lastCheckedAt.Valid {
		h.LastCheckedAt = &lastCheckedAt.Time
	}
	if lastOnlineAt.Valid {
		h.LastOnlineAt = &lastOnlineAt.Time
	}

	return &h, nil
}

// RemoveHost removes a host from database by hostname.
func (r *Repository) RemoveHost(ctx context.Context, host string) error {
	query := `DELETE FROM hosts WHERE host = ?`
	res, err := r.db.ExecContext(ctx, query, host)
	if err != nil {
		return fmt.Errorf("failed to remove host: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrHostNotFound
	}
	return nil
}

// GetHost fetches a host by hostname.
func (r *Repository) GetHost(ctx context.Context, host string) (*Host, error) {
	query := `
	SELECT id, host, host_type, enabled, last_status, last_http_status, last_response_time, last_checked_at, last_online_at, created_at, updated_at
	FROM hosts WHERE host = ?;
	`
	return r.querySingleHost(ctx, query, host)
}

// ListHosts fetches all monitored hosts.
func (r *Repository) ListHosts(ctx context.Context) ([]Host, error) {
	query := `
	SELECT id, host, host_type, enabled, last_status, last_http_status, last_response_time, last_checked_at, last_online_at, created_at, updated_at
	FROM hosts ORDER BY id ASC;
	`
	return r.queryHostList(ctx, query)
}

// GetActiveHosts fetches only enabled hosts for automatic monitoring (implements monitor.HostRepository).
func (r *Repository) GetActiveHosts(ctx context.Context) ([]monitor.DatabaseHost, error) {
	query := `
	SELECT id, host, host_type, enabled, last_status, last_http_status, last_response_time, last_checked_at, last_online_at
	FROM hosts WHERE enabled = 1 ORDER BY id ASC;
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []monitor.DatabaseHost
	for rows.Next() {
		var h monitor.DatabaseHost
		var hTypeStr string
		var lastStatusStr string
		var lastCheckedAt, lastOnlineAt sql.NullTime

		if err := rows.Scan(
			&h.ID, &h.Host, &hTypeStr, &h.Enabled, &lastStatusStr,
			&h.LastHTTPStatus, &h.LastResponseTime, &lastCheckedAt, &lastOnlineAt,
		); err != nil {
			return nil, err
		}

		h.HostType = monitor.HostType(hTypeStr)
		h.LastStatus = monitor.OverallStatus(lastStatusStr)
		if lastCheckedAt.Valid {
			h.LastCheckedAt = &lastCheckedAt.Time
		}
		if lastOnlineAt.Valid {
			h.LastOnlineAt = &lastOnlineAt.Time
		}

		result = append(result, h)
	}

	return result, rows.Err()
}

// SetHostEnabled enables or disables automatic monitoring for a host.
func (r *Repository) SetHostEnabled(ctx context.Context, host string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	query := `UPDATE hosts SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE host = ?`
	res, err := r.db.ExecContext(ctx, query, enabledInt, host)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrHostNotFound
	}
	return nil
}

// CountHosts returns the total number of registered hosts.
func (r *Repository) CountHosts(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM hosts`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// UpdateHostStatus updates the last diagnostic results of a host (implements monitor.HostRepository).
func (r *Repository) UpdateHostStatus(ctx context.Context, id int64, status monitor.OverallStatus, httpStatus int, responseTime int64, checkedAt time.Time, isOnline bool) error {
	var query string
	var args []any

	if isOnline {
		query = `
		UPDATE hosts 
		SET last_status = ?, last_http_status = ?, last_response_time = ?, last_checked_at = ?, last_online_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?;
		`
		args = []any{string(status), httpStatus, responseTime, checkedAt, checkedAt, id}
	} else {
		query = `
		UPDATE hosts 
		SET last_status = ?, last_http_status = ?, last_response_time = ?, last_checked_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?;
		`
		args = []any{string(status), httpStatus, responseTime, checkedAt, id}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// InsertCheck inserts a check history entry (implements monitor.HostRepository).
func (r *Repository) InsertCheck(ctx context.Context, check monitor.DatabaseCheck) error {
	query := `
	INSERT INTO checks (host_id, status, check_type, http_status, latency_ms, port, error_message, checked_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, query,
		check.HostID, string(check.Status), check.CheckType, check.HTTPStatus,
		check.LatencyMs, check.Port, check.ErrorMessage, check.CheckedAt,
	)
	return err
}

// CleanOldChecks removes check histories older than retentionDays (implements monitor.HostRepository).
func (r *Repository) CleanOldChecks(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	query := `DELETE FROM checks WHERE checked_at < ?;`
	res, err := r.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AddAdminUser inserts or updates a user with a specific role (OWNER, ADMIN, or USER).
func (r *Repository) AddAdminUser(ctx context.Context, userID int64, firstName, username string, isOwner bool, role UserRole) error {
	ownerInt := 0
	if isOwner {
		ownerInt = 1
		role = RoleOwner
	}
	if role == "" {
		role = RoleUser
	}
	role = UserRole(strings.ToUpper(string(role)))

	query := `
	INSERT INTO admin_users (user_id, first_name, username, is_owner, role, created_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(user_id) DO UPDATE SET first_name = excluded.first_name, username = excluded.username, is_owner = excluded.is_owner, role = excluded.role;
	`
	_, err := r.db.ExecContext(ctx, query, userID, firstName, username, ownerInt, string(role))
	return err
}

// RemoveAdminUser deletes a user from admin_users (cannot delete owner).
func (r *Repository) RemoveAdminUser(ctx context.Context, userID int64) error {
	query := `DELETE FROM admin_users WHERE user_id = ? AND is_owner = 0`
	res, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

// GetAdminUser returns user details including role.
func (r *Repository) GetAdminUser(ctx context.Context, userID int64) (*AdminUser, error) {
	query := `SELECT user_id, first_name, username, is_owner, role, created_at FROM admin_users WHERE user_id = ?`
	var u AdminUser
	var isOwnerInt int
	var roleStr string
	var fn, un sql.NullString
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&u.UserID, &fn, &un, &isOwnerInt, &roleStr, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.FirstName = fn.String
	u.Username = un.String
	u.IsOwner = isOwnerInt == 1
	if u.IsOwner {
		u.Role = RoleOwner
	} else {
		u.Role = UserRole(strings.ToUpper(roleStr))
		if u.Role == "" {
			u.Role = RoleUser
		}
	}
	return &u, nil
}

// IsAdminUser checks if a userID is registered.
func (r *Repository) IsAdminUser(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM admin_users WHERE user_id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count > 0, err
}

// CountAdminUsers returns total registered users in SQLite.
func (r *Repository) CountAdminUsers(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM admin_users`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// ListAdminUsers returns all user records with roles.
func (r *Repository) ListAdminUsers(ctx context.Context) ([]AdminUser, error) {
	query := `SELECT user_id, first_name, username, is_owner, role, created_at FROM admin_users ORDER BY is_owner DESC, created_at ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AdminUser
	for rows.Next() {
		var u AdminUser
		var isOwnerInt int
		var roleStr string
		var fn, un sql.NullString
		if err := rows.Scan(&u.UserID, &fn, &un, &isOwnerInt, &roleStr, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.FirstName = fn.String
		u.Username = un.String
		u.IsOwner = isOwnerInt == 1
		if u.IsOwner {
			u.Role = RoleOwner
		} else {
			u.Role = UserRole(strings.ToUpper(roleStr))
			if u.Role == "" {
				u.Role = RoleUser
			}
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListAdminUserIDs returns a slice of all registered user IDs.
func (r *Repository) ListAdminUserIDs(ctx context.Context) ([]int64, error) {
	query := `SELECT user_id FROM admin_users`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (r *Repository) querySingleHost(ctx context.Context, query string, args ...any) (*Host, error) {
	var h Host
	var hTypeStr string
	var lastStatusStr string
	var lastCheckedAt, lastOnlineAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&h.ID, &h.Host, &hTypeStr, &h.Enabled, &lastStatusStr,
		&h.LastHTTPStatus, &h.LastResponseTime, &lastCheckedAt, &lastOnlineAt,
		&h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}

	h.HostType = monitor.HostType(hTypeStr)
	h.LastStatus = monitor.OverallStatus(lastStatusStr)
	if lastCheckedAt.Valid {
		h.LastCheckedAt = &lastCheckedAt.Time
	}
	if lastOnlineAt.Valid {
		h.LastOnlineAt = &lastOnlineAt.Time
	}

	return &h, nil
}

func (r *Repository) queryHostList(ctx context.Context, query string, args ...any) ([]Host, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		var hTypeStr string
		var lastStatusStr string
		var lastCheckedAt, lastOnlineAt sql.NullTime

		if err := rows.Scan(
			&h.ID, &h.Host, &hTypeStr, &h.Enabled, &lastStatusStr,
			&h.LastHTTPStatus, &h.LastResponseTime, &lastCheckedAt, &lastOnlineAt,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}

		h.HostType = monitor.HostType(hTypeStr)
		h.LastStatus = monitor.OverallStatus(lastStatusStr)
		if lastCheckedAt.Valid {
			h.LastCheckedAt = &lastCheckedAt.Time
		}
		if lastOnlineAt.Valid {
			h.LastOnlineAt = &lastOnlineAt.Time
		}

		hosts = append(hosts, h)
	}

	return hosts, rows.Err()
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE") || strings.Contains(errStr, "constraint failed")
}

// HostUptimeStats contains aggregated uptime metrics over a given time window.
type HostUptimeStats struct {
	HostID        int64
	Host          string
	HostType      monitor.HostType
	CurrentStatus monitor.OverallStatus
	Enabled       bool
	TotalChecks   int
	OnlineChecks  int
	DownChecks    int
	AvgLatencyMs  int64
	UptimePct     float64
	Since         time.Time
}

// GetHostUptime calculates uptime statistics for a specific host since a given timestamp.
func (r *Repository) GetHostUptime(ctx context.Context, hostID int64, since time.Time) (*HostUptimeStats, error) {
	queryHost := `SELECT id, host, host_type, last_status, enabled FROM hosts WHERE id = ?;`
	var stats HostUptimeStats
	var hTypeStr, statusStr string
	var enabledInt int
	err := r.db.QueryRowContext(ctx, queryHost, hostID).Scan(&stats.HostID, &stats.Host, &hTypeStr, &statusStr, &enabledInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHostNotFound
		}
		return nil, err
	}
	stats.HostType = monitor.HostType(hTypeStr)
	stats.CurrentStatus = monitor.OverallStatus(statusStr)
	stats.Enabled = enabledInt == 1
	stats.Since = since

	queryChecks := `
	SELECT 
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'ONLINE' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status != 'ONLINE' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms ELSE NULL END), 0)
	FROM checks
	WHERE host_id = ? AND checked_at >= ?;
	`
	var avgLatency sql.NullFloat64
	err = r.db.QueryRowContext(ctx, queryChecks, hostID, since).Scan(
		&stats.TotalChecks,
		&stats.OnlineChecks,
		&stats.DownChecks,
		&avgLatency,
	)
	if err != nil {
		return nil, err
	}

	if avgLatency.Valid {
		stats.AvgLatencyMs = int64(avgLatency.Float64)
	}

	if stats.TotalChecks > 0 {
		stats.UptimePct = (float64(stats.OnlineChecks) / float64(stats.TotalChecks)) * 100.0
	} else {
		if stats.CurrentStatus == monitor.StatusOnline {
			stats.UptimePct = 100.0
		} else {
			stats.UptimePct = 0.0
		}
	}

	return &stats, nil
}

// GetAllHostsUptime calculates uptime statistics for all active hosts since a given timestamp.
func (r *Repository) GetAllHostsUptime(ctx context.Context, since time.Time) ([]HostUptimeStats, error) {
	query := `
	SELECT 
		h.id, h.host, h.host_type, h.last_status, h.enabled,
		COUNT(c.id) AS total_checks,
		COALESCE(SUM(CASE WHEN c.status = 'ONLINE' THEN 1 ELSE 0 END), 0) AS online_checks,
		COALESCE(SUM(CASE WHEN c.status != 'ONLINE' THEN 1 ELSE 0 END), 0) AS down_checks,
		COALESCE(AVG(CASE WHEN c.latency_ms > 0 THEN c.latency_ms ELSE NULL END), 0) AS avg_latency
	FROM hosts h
	LEFT JOIN checks c ON h.id = c.host_id AND c.checked_at >= ?
	WHERE h.enabled = 1
	GROUP BY h.id, h.host, h.host_type, h.last_status, h.enabled
	ORDER BY h.id ASC;
	`
	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HostUptimeStats
	for rows.Next() {
		var s HostUptimeStats
		var hTypeStr, statusStr string
		var enabledInt int
		var avgLatency sql.NullFloat64

		if err := rows.Scan(
			&s.HostID, &s.Host, &hTypeStr, &statusStr, &enabledInt,
			&s.TotalChecks, &s.OnlineChecks, &s.DownChecks, &avgLatency,
		); err != nil {
			return nil, err
		}

		s.HostType = monitor.HostType(hTypeStr)
		s.CurrentStatus = monitor.OverallStatus(statusStr)
		s.Enabled = enabledInt == 1
		s.Since = since
		if avgLatency.Valid {
			s.AvgLatencyMs = int64(avgLatency.Float64)
		}

		if s.TotalChecks > 0 {
			s.UptimePct = (float64(s.OnlineChecks) / float64(s.TotalChecks)) * 100.0
		} else {
			if s.CurrentStatus == monitor.StatusOnline {
				s.UptimePct = 100.0
			} else {
				s.UptimePct = 0.0
			}
		}

		result = append(result, s)
	}

	return result, rows.Err()
}
