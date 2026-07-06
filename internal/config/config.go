// Package config carica e valida la configurazione YAML dell'applicazione.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config rispecchia config.example.yaml.
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
// substrings) is found in its description/counterparty text.
type Category struct {
	Name     string   `yaml:"name"`
	Color    string   `yaml:"color"`
	Icon     string   `yaml:"icon"`
	MatchAny []string `yaml:"match_any"`
}

// Budget sets a monthly spending limit for a category.
type Budget struct {
	Category     string  `yaml:"category"`
	MonthlyLimit float64 `yaml:"monthly_limit"`
}

// TelegramConfig è predisposto per la prossima iterazione (non usato ora).
type TelegramConfig struct {
	Enabled  bool           `yaml:"enabled"`
	BotToken string         `yaml:"bot_token"`
	ChatID   string         `yaml:"chat_id"`
	Rules    []TelegramRule `yaml:"rules"`
}

type TelegramRule struct {
	Name        string  `yaml:"name"`
	MinAmount   float64 `yaml:"min_amount"`
	Contains    string  `yaml:"contains"`
	CreditDebit string  `yaml:"credit_debit"`
}

// Load legge, applica i default e valida la configurazione.
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
		return fmt.Errorf("enablebanking.application_id mancante")
	}
	if eb.PrivateKeyPath == "" {
		return fmt.Errorf("enablebanking.private_key_path mancante")
	}
	if c.Sync.TimesPerDay < 1 || c.Sync.TimesPerDay > 24 {
		return fmt.Errorf("sync.times_per_day deve essere tra 1 e 24")
	}
	if c.Dashboard.Enabled {
		if c.Dashboard.BasicAuth.Username == "" || c.Dashboard.BasicAuth.Password == "" {
			return fmt.Errorf("dashboard.basic_auth.username e password sono richiesti quando dashboard.enabled è true")
		}
		if c.AuthServer.TLSCertPath == "" || c.AuthServer.TLSKeyPath == "" {
			return fmt.Errorf("auth_server.tls_cert_path e tls_key_path sono richiesti per la dashboard HTTPS")
		}
		if c.AuthServer.ListenAddr == "" {
			return fmt.Errorf("auth_server.listen_addr è richiesto per la dashboard")
		}
	}
	return nil
}
