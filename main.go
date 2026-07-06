// Command expense_monitor downloads accounts and transactions from Enable
// Banking and stores them in SQLite. Subcommands:
//
//	auth       one-time consent flow -> saves session.json
//	serve      daemon: syncs periodically, serves the dashboard and Telegram alerts
//	sync-once  runs a single sync and exits (handy in dev)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"expense_monitor/internal/auth"
	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/enablebanking"
	"expense_monitor/internal/server"
	"expense_monitor/internal/store"
	"expense_monitor/internal/syncer"
	"expense_monitor/internal/telegram"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	switch cmd {
	case "-h", "--help", "help":
		usage()
		return
	}
	if cmd != "auth" && cmd != "serve" && cmd != "sync-once" && cmd != "aspsps" && cmd != "dashboard" && cmd != "report" {
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "path to the YAML configuration file")
	month := fs.String("month", "", "month YYYY-MM for the `report` command (default: current month)")
	_ = fs.Parse(os.Args[2:])

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatalf("config: %v", err)
	}

	switch cmd {
	case "auth":
		if err := auth.Run(ctx, cfg); err != nil {
			fatalf("auth: %v", err)
		}
	case "serve":
		if err := server.Run(ctx, cfg); err != nil {
			fatalf("serve: %v", err)
		}
	case "sync-once":
		if err := syncer.RunOnce(ctx, cfg); err != nil {
			fatalf("sync-once: %v", err)
		}
	case "aspsps":
		if err := runASPSPs(ctx, cfg); err != nil {
			fatalf("aspsps: %v", err)
		}
	case "dashboard":
		if err := server.RunDashboard(ctx, cfg); err != nil {
			fatalf("dashboard: %v", err)
		}
	case "report":
		if err := runReport(ctx, cfg, *month); err != nil {
			fatalf("report: %v", err)
		}
	}
}

// runReport sends the Telegram monthly report for a given month immediately,
// bypassing the daemon's month-rollover gate (useful for testing / re-sends).
func runReport(ctx context.Context, cfg *config.Config, month string) error {
	if !cfg.Telegram.Enabled {
		return fmt.Errorf("telegram.enabled is false: set enabled + bot_token + chat_id to send reports")
	}
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	st, err := store.Open(cfg.Storage.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	cat := category.Build(cfg.Dashboard.Categories, cfg.Dashboard.Budgets)
	mon := telegram.NewMonitor(cfg.Telegram, st, cat)
	fmt.Printf("Sending Telegram monthly report for %s ...\n", month)
	mon.SendReport(ctx, month)
	fmt.Println("Done. Check your Telegram chat (send errors, if any, are logged above).")
	return nil
}

// runASPSPs lists the banks available for the configured country, so you can
// find the exact value to put in enablebanking.aspsp_name.
func runASPSPs(ctx context.Context, cfg *config.Config) error {
	eb := cfg.EnableBanking
	client, err := enablebanking.New(eb.BaseURL, eb.ApplicationID, eb.PrivateKeyPath)
	if err != nil {
		return err
	}
	banks, err := client.ListASPSPs(ctx, eb.Country)
	if err != nil {
		return err
	}
	fmt.Printf("Banks available for %q (%d):\n", eb.Country, len(banks))
	for _, b := range banks {
		fmt.Printf("  %-40s [%s]\n", b.Name, b.Country)
	}
	fmt.Println("\nSet enablebanking.aspsp_name to the exact name of your bank.")
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `expense_monitor - expense monitoring via Enable Banking

Usage:
  expense_monitor <command> [--config config.yaml]

Commands:
  aspsps      List the banks available for the configured country
  auth        Start the bank authorization flow and save the session
  serve       Start the daemon (periodic sync + web dashboard + Telegram)
  sync-once   Run a single sync and exit
  dashboard   Serve only the web dashboard (no sync)
  report      Send the Telegram monthly report now [--month YYYY-MM]
  help        Show this message
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
