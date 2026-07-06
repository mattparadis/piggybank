// Package notify defines the notification interface. This package provides
// LogNotifier; the Telegram notifier is implemented in internal/telegram.
package notify

import "log"

// Notifier receives informational events and alerts.
type Notifier interface {
	Info(msg string)
	Alert(msg string)
}

// LogNotifier writes notifications to the standard log.
type LogNotifier struct{}

func (LogNotifier) Info(msg string)  { log.Printf("[info] %s", msg) }
func (LogNotifier) Alert(msg string) { log.Printf("[ALERT] %s", msg) }

// LogNotifier is a simple fallback Notifier that logs every event; the
// Telegram-based Notifier lives in internal/telegram.
