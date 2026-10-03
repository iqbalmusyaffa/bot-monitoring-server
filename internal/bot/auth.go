package bot

import (
	"context"
	"fmt"
	"log/slog"

	"host-monitor/internal/config"
	"host-monitor/internal/database"
)

// Authenticator handles authorization checks based on the configured whitelist and SQLite user table.
type Authenticator struct {
	cfg  *config.Config
	repo *database.Repository
}

// NewAuthenticator creates a new Authenticator.
func NewAuthenticator(cfg *config.Config, repo *database.Repository) *Authenticator {
	return &Authenticator{
		cfg:  cfg,
		repo: repo,
	}
}

// CheckAndAuthorize verifies user authorization and returns their specific role.
func (a *Authenticator) CheckAndAuthorize(ctx context.Context, userID int64, firstName, username string) (isAuth bool, role database.UserRole, isFirstClaim bool) {
	// 1. Check if explicitly in .env whitelist -> Default to ADMIN role
	if a.cfg != nil && a.cfg.AllowedUserIDs[userID] {
		return true, database.RoleAdmin, false
	}

	if a.repo == nil {
		return false, "", false
	}

	// 2. Check if in SQLite database table
	user, err := a.repo.GetAdminUser(ctx, userID)
	if err == nil && user != nil {
		return true, user.Role, false
	}

	// 3. AUTO-DETECT & AUTO-CLAIM: If NO admin/user exists anywhere, register first user as OWNER!
	hasEnvWhitelist := a.cfg != nil && len(a.cfg.AllowedUserIDs) > 0
	countUsers, err := a.repo.CountAdminUsers(ctx)
	if !hasEnvWhitelist && (err != nil || countUsers == 0) {
		slog.Info("First user detected! Automatically claiming as Bot Owner",
			"user_id", userID,
			"first_name", firstName,
			"username", username,
		)

		_ = a.repo.AddAdminUser(ctx, userID, firstName, username, true, database.RoleOwner)
		return true, database.RoleOwner, true
	}

	return false, "", false
}

// HandleUnauthorized logs the attempt and sends the rejection message along with the sender's Telegram User ID.
func (a *Authenticator) HandleUnauthorized(ctx context.Context, client *Client, chatID int64, userID int64, userName string, command string) {
	slog.Warn("Unauthorized access attempt blocked",
		"user_id", userID,
		"user_name", userName,
		"command", command,
		"chat_id", chatID,
	)

	reply := fmt.Sprintf(`❌ Unauthorized.

User ID Telegram Anda: %d

Hubungi Admin/Owner bot untuk menambahkan ID Anda agar diberikan izin akses.`, userID)

	if err := client.SendMessage(ctx, chatID, reply); err != nil {
		slog.Error("Failed to send unauthorized response", "chat_id", chatID, "error", err)
	}
}
