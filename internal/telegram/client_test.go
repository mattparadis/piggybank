package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSendMessage(t *testing.T) {
	var gotPath, gotChat, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotChat = r.FormValue("chat_id")
		gotText = r.FormValue("text")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("TOKEN", "CHAT")
	c.base = srv.URL
	if err := c.SendMessage(context.Background(), "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if gotPath != "/botTOKEN/sendMessage" {
		t.Errorf("path = %q, want /botTOKEN/sendMessage", gotPath)
	}
	if gotChat != "CHAT" {
		t.Errorf("chat_id = %q, want CHAT", gotChat)
	}
	if gotText != "hello" {
		t.Errorf("text = %q, want hello", gotText)
	}
}

func TestClientSendPhoto(t *testing.T) {
	var gotPath, gotCaption string
	var photoLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotCaption = r.FormValue("caption")
		if f, _, err := r.FormFile("photo"); err == nil {
			b, _ := io.ReadAll(f)
			photoLen = len(b)
			f.Close()
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("T", "C")
	c.base = srv.URL
	if err := c.SendPhoto(context.Background(), "cap", []byte("PNGDATA")); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if gotPath != "/botT/sendPhoto" {
		t.Errorf("path = %q, want /botT/sendPhoto", gotPath)
	}
	if gotCaption != "cap" {
		t.Errorf("caption = %q, want cap", gotCaption)
	}
	if photoLen != len("PNGDATA") {
		t.Errorf("photo length = %d, want %d", photoLen, len("PNGDATA"))
	}
}

func TestClientAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer srv.Close()

	c := NewClient("T", "C")
	c.base = srv.URL
	if err := c.SendMessage(context.Background(), "x"); err == nil {
		t.Fatal("expected error on ok:false response")
	}
}
