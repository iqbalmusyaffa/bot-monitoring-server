package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	telegramAPIBase = "https://api.telegram.org/bot"
)

// User represents a Telegram User.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// Chat represents a Telegram Chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Message represents a Telegram Message.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
	Date      int64  `json:"date"`
}

// CallbackQuery represents a callback query from an inline keyboard.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// Update represents an incoming Telegram update.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// BotCommand represents a bot command displayed in Telegram's menu button.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// KeyboardButton represents a single button in a reply keyboard.
type KeyboardButton struct {
	Text string `json:"text"`
}

// ReplyKeyboardMarkup represents a custom keyboard with reply options.
type ReplyKeyboardMarkup struct {
	Keyboard       [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard bool               `json:"resize_keyboard"`
	IsPersistent   bool               `json:"is_persistent"`
}

// InlineKeyboardButton represents one button of an inline keyboard.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard that appears next to messages.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// APIResponse is the generic envelope returned by Telegram Bot API.
type APIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result,omitempty"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// Client interacts with the Telegram Bot API using net/http.
type Client struct {
	token      string
	apiBaseURL string
	httpClient *http.Client
}

// NewClient creates a new Telegram Bot API client.
func NewClient(token string, httpClient *http.Client) (*Client, error) {
	if token == "" {
		return nil, errors.New("telegram bot token cannot be empty")
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 35 * time.Second, // Long polling + buffer
		}
	}
	return &Client{
		token:      token,
		apiBaseURL: telegramAPIBase + token,
		httpClient: httpClient,
	}, nil
}

// SetBaseURL allows overriding API URL (useful for testing).
func (c *Client) SetBaseURL(url string) {
	c.apiBaseURL = url
}

// GetMe tests the token and returns the bot's identity.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	url := fmt.Sprintf("%s/getMe", c.apiBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getMe network error: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse[User]
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("getMe decode error: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("telegram api error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	return &apiResp.Result, nil
}

// GetUpdates fetches new updates using long polling.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.apiBaseURL, offset, timeoutSeconds)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getUpdates network error: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse[[]Update]
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1048576)).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("getUpdates decode error: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("telegram api error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	return apiResp.Result, nil
}

type sendMessagePayload struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

// SendMessage sends a plain text message to a given chat ID.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.SendMessageWithMarkup(ctx, chatID, text, nil)
}

// SendMessageWithMarkup sends a message with custom keyboard or inline buttons.
func (c *Client) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, replyMarkup any) error {
	url := fmt.Sprintf("%s/sendMessage", c.apiBaseURL)

	payload := sendMessagePayload{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: replyMarkup,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sendMessage network error: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse[Message]
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&apiResp); err != nil {
		return fmt.Errorf("sendMessage decode error: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("telegram api error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	return nil
}

type setMyCommandsPayload struct {
	Commands []BotCommand `json:"commands"`
}

// SetMyCommands configures the commands list shown in Telegram's Menu button.
func (c *Client) SetMyCommands(ctx context.Context, commands []BotCommand) error {
	url := fmt.Sprintf("%s/setMyCommands", c.apiBaseURL)

	payload := setMyCommandsPayload{
		Commands: commands,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("setMyCommands network error: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse[bool]
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&apiResp); err != nil {
		return fmt.Errorf("setMyCommands decode error: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("telegram api error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	return nil
}

type answerCallbackQueryPayload struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

// AnswerCallbackQuery sends an acknowledgment for an incoming callback query.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error {
	url := fmt.Sprintf("%s/answerCallbackQuery", c.apiBaseURL)

	payload := answerCallbackQueryPayload{
		CallbackQueryID: callbackQueryID,
		Text:            text,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("answerCallbackQuery network error: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

