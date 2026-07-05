// Command expense_monitor scarica conti e transazioni da Enable Banking e li
// salva in SQLite. Sottocomandi:
//
//	auth       flusso di consenso una tantum -> salva session.json
//	serve      daemon: sincronizza periodicamente (dashboard/telegram: predisposti)
//	sync-once  esegue una singola sincronizzazione ed esce (utile in dev)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"expense_monitor/internal/auth"
	"expense_monitor/internal/config"
	"expense_monitor/internal/server"
	"expense_monitor/internal/syncer"
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
	if cmd != "auth" && cmd != "serve" && cmd != "sync-once" {
		fmt.Fprintf(os.Stderr, "comando sconosciuto: %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "percorso del file di configurazione YAML")
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
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `expense_monitor - monitoraggio spese via Enable Banking

Uso:
  expense_monitor <comando> [--config config.yaml]

Comandi:
  auth        Avvia il flusso di autorizzazione bancaria e salva la sessione
  serve       Avvia il daemon che sincronizza le transazioni periodicamente
  sync-once   Esegue una singola sincronizzazione ed esce
  help        Mostra questo messaggio
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
