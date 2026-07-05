// Package notify definisce l'interfaccia di notifica. Per questa iterazione è
// implementato solo LogNotifier; il bot Telegram è predisposto (vedi TODO).
package notify

import "log"

// Notifier riceve eventi informativi e allarmi.
type Notifier interface {
	Info(msg string)
	Alert(msg string)
}

// LogNotifier scrive le notifiche sul log standard.
type LogNotifier struct{}

func (LogNotifier) Info(msg string)  { log.Printf("[info] %s", msg) }
func (LogNotifier) Alert(msg string) { log.Printf("[ALERT] %s", msg) }

// TODO(telegram): TelegramNotifier implementerà Notifier inviando messaggi via
// Bot API, applicando le regole configurate in config.Telegram.Rules.
