package main

import (
	"context"
	"log"

	"fyne.io/fyne/v2/app"

	"github.com/Alexander-D-Karpov/amp/internal/config"
	"github.com/Alexander-D-Karpov/amp/internal/ui"
)

func main() {
	cfg := config.DefaultMobileConfig()
	cfg.Debug = false

	ctx := context.Background()

	fyneApp := app.New()
	fyneApp.Settings().SetTheme(nil)

	ampApp, err := ui.NewApp(ctx, fyneApp, cfg)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	ampApp.ShowAndRun()
}
