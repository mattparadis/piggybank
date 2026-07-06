// Package config loads and validates the application's YAML configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config mirrors config.example.yaml.
type Config struct {
	EnableBanking EnableBankingConfig `yaml:"enablebanking"`
	AuthServer    AuthServerConfig    `yaml:"auth_server"`
	Storage       StorageConfig       `yaml:"storage"`
	Sync          SyncConfig          `yaml:"sync"`
	Dashboard     DashboardConfig     `yaml:"dashboard"`
	Telegram      TelegramConfig      `yaml:"telegram"`
}

type EnableBankingConfig struct {
	BaseURL        string `yaml:"base_url"`
	ApplicationID  string `yaml:"application_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	Country        string `yaml:"country"`
	ASPSPName      string `yaml:"aspsp_name"`
	PSUType        string `yaml:"psu_type"`
	ConsentDays    int    `yaml:"consent_days"`
}

type AuthServerConfig struct {
	ListenAddr  string `yaml:"listen_addr"`
	RedirectURL string `yaml:"redirect_url"`
	TLSCertPath string `yaml:"tls_cert_path"`
	TLSKeyPath  string `yaml:"tls_key_path"`
}

type StorageConfig struct {
	DBPath      string `yaml:"db_path"`
	SessionPath string `yaml:"session_path"`
}

type SyncConfig struct {
	TimesPerDay         int `yaml:"times_per_day"`
	InitialLookbackDays int `yaml:"initial_lookback_days"`
}

// DashboardConfig configures the web dashboard served by the `serve` daemon.
// The dashboard listens on AuthServer.ListenAddr over HTTPS, reusing the
// AuthServer TLS certificate (same Tailscale host).
type DashboardConfig struct {
	Enabled    bool       `yaml:"enabled"`
	BasicAuth  BasicAuth  `yaml:"basic_auth"`
	Categories []Category `yaml:"categories"`
	Budgets    []Budget   `yaml:"budgets"`
}

// BasicAuth holds the credentials required to access the dashboard.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Category classifies a transaction when any of MatchAny (case-insensitive
// substrings) is found in its description/counterparty text. When Savings is
// true, matching transactions are treated as money moved to savings (a
// transfer) rather than an expense: they are excluded from spending totals and
// reported separately as an amount saved.
type Category struct {
	Name     string   `yaml:"name"`
	Color    string   `yaml:"color"`
	Icon     string   `yaml:"icon"`
	MatchAny []string `yaml:"match_any"`
	Savings  bool     `yaml:"savings"`
}

// Budget sets a monthly spending limit for a category.
type Budget struct {
	Category     string  `yaml:"category"`
	MonthlyLimit float64 `yaml:"monthly_limit"`
}

// TelegramConfig configures push notifications sent by the `serve` daemon via
// the Telegram Bot API.
type TelegramConfig struct {
	Enabled       bool                `yaml:"enabled"`
	BotToken      string              `yaml:"bot_token"`
	ChatID        string              `yaml:"chat_id"`
	StartupPing   bool                `yaml:"startup_ping"`   // one-time "daemon started" message
	SessionAlerts bool                `yaml:"session_alerts"` // session expired + sync failure
	SpendingAlert SpendingAlertConfig `yaml:"spending_alert"`
	MonthlyReport MonthlyReportConfig `yaml:"monthly_report"`
}

// SpendingAlertConfig triggers a notification when cumulative spending in the
// current month crosses Threshold, and again every Step above it (e.g.
// Threshold 200, Step 30 -> alerts at 200, 230, 260, ...). Savings-category
// transactions are excluded from the running total.
type SpendingAlertConfig struct {
	Enabled   bool    `yaml:"enabled"`
	Threshold float64 `yaml:"threshold"`
	Step      float64 `yaml:"step"`
}

// MonthlyReportConfig sends a spending report at the start of each new month,
// summarizing the month that just ended.
type MonthlyReportConfig struct {
	Enabled   bool `yaml:"enabled"`
	WithChart bool `yaml:"with_chart"` // attach a category bar chart image
}

// Load reads, applies defaults and validates the configuration.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.EnableBanking.BaseURL == "" {
		c.EnableBanking.BaseURL = "https://api.enablebanking.com"
	}
	if c.EnableBanking.PSUType == "" {
		c.EnableBanking.PSUType = "personal"
	}
	if c.EnableBanking.ConsentDays == 0 {
		c.EnableBanking.ConsentDays = 90
	}
	if c.Storage.DBPath == "" {
		c.Storage.DBPath = "expense_monitor.db"
	}
	if c.Storage.SessionPath == "" {
		c.Storage.SessionPath = "session.json"
	}
	if c.Sync.TimesPerDay == 0 {
		c.Sync.TimesPerDay = 2
	}
	if c.Sync.InitialLookbackDays == 0 {
		c.Sync.InitialLookbackDays = 90
	}
}

func (c *Config) validate() error {
	eb := c.EnableBanking
	if eb.ApplicationID == "" {
		return fmt.Errorf("enablebanking.application_id is missing")
	}
	if eb.PrivateKeyPath == "" {
		return fmt.Errorf("enablebanking.private_key_path is missing")
	}
	if c.Sync.TimesPerDay < 1 || c.Sync.TimesPerDay > 24 {
		return fmt.Errorf("sync.times_per_day must be between 1 and 24")
	}
	if c.Dashboard.Enabled {
		if c.Dashboard.BasicAuth.Username == "" || c.Dashboard.BasicAuth.Password == "" {
			return fmt.Errorf("dashboard.basic_auth.username and password are required when dashboard.enabled is true")
		}
		if c.AuthServer.TLSCertPath == "" || c.AuthServer.TLSKeyPath == "" {
			return fmt.Errorf("auth_server.tls_cert_path and tls_key_path are required for the HTTPS dashboard")
		}
		if c.AuthServer.ListenAddr == "" {
			return fmt.Errorf("auth_server.listen_addr is required for the dashboard")
		}
	}
	if c.Telegram.Enabled {
		if c.Telegram.BotToken == "" || c.Telegram.ChatID == "" {
			return fmt.Errorf("telegram.bot_token and telegram.chat_id are required when telegram.enabled is true")
		}
		if c.Telegram.SpendingAlert.Enabled {
			if c.Telegram.SpendingAlert.Threshold <= 0 || c.Telegram.SpendingAlert.Step <= 0 {
				return fmt.Errorf("telegram.spending_alert.threshold and step must be greater than 0")
			}
		}
	}
	return nil
}
