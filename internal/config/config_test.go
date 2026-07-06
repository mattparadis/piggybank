package config

import "testing"

func baseValid() *Config {
	return &Config{
		EnableBanking: EnableBankingConfig{ApplicationID: "app", PrivateKeyPath: "/keys/app.pem"},
		Sync:          SyncConfig{TimesPerDay: 2},
	}
}

func TestValidateTelegramRequiresTokenAndChat(t *testing.T) {
	c := baseValid()
	c.Telegram = TelegramConfig{Enabled: true}
	if err := c.validate(); err == nil {
		t.Fatal("expected error when bot_token/chat_id are missing")
	}
	c.Telegram.BotToken = "token"
	c.Telegram.ChatID = "chat"
	if err := c.validate(); err != nil {
		t.Fatalf("valid telegram config rejected: %v", err)
	}
}

func TestValidateSpendingAlertRequiresThresholdAndStep(t *testing.T) {
	c := baseValid()
	c.Telegram = TelegramConfig{
		Enabled:       true,
		BotToken:      "token",
		ChatID:        "chat",
		SpendingAlert: SpendingAlertConfig{Enabled: true},
	}
	if err := c.validate(); err == nil {
		t.Fatal("expected error when threshold/step are unset")
	}
	c.Telegram.SpendingAlert.Threshold = 200
	c.Telegram.SpendingAlert.Step = 30
	if err := c.validate(); err != nil {
		t.Fatalf("valid spending_alert rejected: %v", err)
	}
}

func TestValidateTelegramDisabledSkipsChecks(t *testing.T) {
	c := baseValid()
	c.Telegram = TelegramConfig{Enabled: false} // no token/chat, but disabled
	if err := c.validate(); err != nil {
		t.Fatalf("disabled telegram should not be validated: %v", err)
	}
}
