package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/fyne/v2/app"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/ui"
)

var (
	configPath = flag.String("config", "", "Path to configuration file")
	debug      = flag.Bool("debug", false, "Enable debug mode - shows detailed logging for all components")
	Version    = "dev"
)

func main() {
	flag.Parse()

	if *debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("[MAIN] Debug mode enabled - all components will log detailed information")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[MAIN] Failed to load config: %v", err)
	}

	if *debug {
		cfg.Debug = true
		log.Printf("[MAIN] Configuration loaded successfully")
		log.Printf("[MAIN] - API Base URL: %s", cfg.API.BaseURL)
		log.Printf("[MAIN] - Database Path: %s", cfg.Storage.DatabasePath)
		log.Printf("[MAIN] - Cache Directory: %s", cfg.Storage.CacheDir)
		log.Printf("[MAIN] - Theme: %s", cfg.UI.Theme)
		log.Printf("[MAIN] - Window Size: %dx%d", cfg.UI.WindowWidth, cfg.UI.WindowHeight)
		log.Printf("[MAIN] - Sync Interval: %d seconds", cfg.Storage.SyncInterval)
		log.Printf("[MAIN] - User: %s (Anonymous: %v)", cfg.User.Username, cfg.User.IsAnonymous)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *debug {
		log.Printf("[MAIN] Creating Fyne application...")
	}

	fyneApp := app.New()

	if *debug {
		log.Printf("[MAIN] Initializing AMP application...")
	}

	ampApp, err := ui.NewApp(ctx, fyneApp, cfg)
	if err != nil {
		log.Fatalf("[MAIN] Failed to create app: %v", err)
	}

	setupGracefulShutdown(cancel, ampApp)
	ampApp.ShowAndRun()
}

func setupGracefulShutdown(cancel context.CancelFunc, ampApp *ui.App) {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		sig := <-c
		log.Printf("[MAIN] Received signal: %v", sig)
		log.Printf("[MAIN] Initiating graceful shutdown...")

		cancel()
		ampApp.Close()

		log.Printf("[MAIN] Graceful shutdown completed")
		os.Exit(0)
	}()
}
