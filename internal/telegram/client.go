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
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

// Client is a minimal Telegram Bot API client (sendMessage + sendPhoto).
type Client struct {
	token  string
	chatID string
	http   *http.Client
	base   string
}

// NewClient returns a Client for the given bot token and target chat.
func NewClient(token, chatID string) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		http:   &http.Client{Timeout: 30 * time.Second},
		base:   apiBase,
	}
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
