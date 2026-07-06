// Package telegram sends push notifications (spending alerts, session/sync
// alerts and monthly reports) to a chat via the Telegram Bot API.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

// Client is a minimal Telegram Bot API client (sendMessage, sendPhoto,
// getUpdates, setMyCommands).
type Client struct {
	token    string
	chatID   string
	http     *http.Client
	pollHTTP *http.Client // longer timeout for long-polling getUpdates
	base     string
}

// NewClient returns a Client for the given bot token and target chat.
func NewClient(token, chatID string) *Client {
	return &Client{
		token:    token,
		chatID:   chatID,
		http:     &http.Client{Timeout: 30 * time.Second},
		pollHTTP: &http.Client{Timeout: 90 * time.Second},
		base:     apiBase,
	}
}

// Update is a single incoming update (we only care about text messages).
type Update struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int    `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// BotCommand is a command advertised in the client's command menu.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// GetUpdates long-polls for new updates starting at offset, blocking up to
// timeoutSec seconds server-side (or until ctx is canceled).
func (c *Client) GetUpdates(ctx context.Context, offset, timeoutSec int) ([]Update, error) {
	q := url.Values{}
	q.Set("offset", strconv.Itoa(offset))
	q.Set("timeout", strconv.Itoa(timeoutSec))
	q.Set("allowed_updates", `["message"]`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.method("getUpdates")+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.pollHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var body struct {
		OK          bool     `json:"ok"`
		Description string   `json:"description"`
		Result      []Update `json:"result"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	if !body.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", body.Description)
	}
	return body.Result, nil
}

// SetMyCommands registers the bot's command menu (best-effort).
func (c *Client) SetMyCommands(ctx context.Context, cmds []BotCommand) error {
	payload, err := json.Marshal(cmds)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("commands", string(payload))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method("setMyCommands"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

// SendMessage sends a plain-text message to the configured chat.
func (c *Client) SendMessage(ctx context.Context, text string) error {
	form := url.Values{}
	form.Set("chat_id", c.chatID)
	form.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method("sendMessage"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

// SendPhoto uploads a PNG image with an optional caption to the configured chat.
func (c *Client) SendPhoto(ctx context.Context, caption string, png []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("chat_id", c.chatID); err != nil {
		return err
	}
	if caption != "" {
		if err := mw.WriteField("caption", caption); err != nil {
			return err
		}
	}
	fw, err := mw.CreateFormFile("photo", "chart.png")
	if err != nil {
		return err
	}
	if _, err := fw.Write(png); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method("sendPhoto"), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.do(req)
}

func (c *Client) method(m string) string {
	return c.base + "/bot" + c.token + "/" + m
}

func (c *Client) do(req *http.Request) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var body struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &body)
	if !body.OK {
		desc := body.Description
		if desc == "" {
			desc = strings.TrimSpace(string(data))
		}
		return fmt.Errorf("telegram api (status %d): %s", resp.StatusCode, desc)
	}
	return nil
}
