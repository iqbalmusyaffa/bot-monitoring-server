package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"host-monitor/internal/config"
	"host-monitor/internal/database"
)

func TestBotRoleManagementSuite(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "role_test.db")
	db, err := database.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()
	repo := database.NewRepository(db)

	var sentMessages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var payload sendMessagePayload
			json.NewDecoder(r.Body).Decode(&payload)
			sentMessages = append(sentMessages, payload.Text)
			json.NewEncoder(w).Encode(APIResponse[Message]{
				OK:     true,
				Result: Message{MessageID: 1, Text: payload.Text},
			})
			return
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		TelegramBotToken:  "dummy_token",
		AllowedUserIDs:    map[int64]bool{},
		CommandRateLimit:  50,
		CommandRateWindow: 1 * time.Minute,
	}
	client, _ := NewClient(cfg.TelegramBotToken, server.Client())
	client.SetBaseURL(server.URL)

	bot := NewBot(cfg, client, repo)
	ctx := context.Background()

	// 1. First user connects -> Auto claimed as OWNER (ID: 10001)
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 10001},
		From: &User{ID: 10001, FirstName: "OwnerBoss"},
		Text: "/start",
	})

	// 2. Owner adds an ADMIN (ID: 20002)
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 10001},
		From: &User{ID: 10001},
		Text: "/adduser 20002 admin",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "ADMIN") {
		t.Errorf("expected add admin success, got: %v", sentMessages)
	}

	// 3. Owner adds a regular USER (ID: 30003)
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 10001},
		From: &User{ID: 10001},
		Text: "/adduser 30003 user",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "USER") {
		t.Errorf("expected add user success, got: %v", sentMessages)
	}

	// 4. Regular USER (ID: 30003) tries /list -> Allowed
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 30003},
		From: &User{ID: 30003},
		Text: "/list",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "DAFTAR HOST") && !strings.Contains(sentMessages[0], "Belum ada host") {
		t.Errorf("expected USER to access /list, got: %v", sentMessages)
	}

	// 5. Regular USER (ID: 30003) tries /add -> FORBIDDEN!
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 30003},
		From: &User{ID: 30003},
		Text: "/add 1.1.1.1",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "Akses Ditolak") {
		t.Errorf("expected USER /add to be forbidden, got: %v", sentMessages)
	}

	// 6. ADMIN (ID: 20002) tries /add -> ALLOWED
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 20002},
		From: &User{ID: 20002},
		Text: "/add 1.1.1.1",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "Berhasil menambahkan") {
		t.Errorf("expected ADMIN /add to succeed, got: %v", sentMessages)
	}

	// 7. ADMIN (ID: 20002) tries /adduser -> FORBIDDEN (Owner only)
	sentMessages = nil
	bot.handleMessage(ctx, &Message{
		Chat: Chat{ID: 20002},
		From: &User{ID: 20002},
		Text: "/adduser 40004",
	})
	if len(sentMessages) == 0 || !strings.Contains(sentMessages[0], "Akses Ditolak") {
		t.Errorf("expected ADMIN /adduser to be forbidden, got: %v", sentMessages)
	}
}
