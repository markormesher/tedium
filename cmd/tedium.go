package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/markormesher/tedium/internal/entrypoints"
	"github.com/markormesher/tedium/internal/schema"
)

var version string // populated via ldflags

func main() {
	internalCommand := flag.String("internal-command", "", "Internal command to perform when Tedium is running itself inside an executor")
	configFilePath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// special cases: internal commands
	switch *internalCommand {
	case "initChore":
		entrypoints.InitChore()
		return

	case "finaliseChore":
		entrypoints.FinaliseChore()
		return
	}

	// normal case: user invocation
	if *configFilePath == "" {
		slog.Error("Config file not provided")
		os.Exit(1)
	}

	conf, err := schema.LoadTediumConfig(*configFilePath, version)
	if err != nil {
		slog.Error("Error loading configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("Tedium version: " + conf.Version)

	entrypoints.Run(conf)
}
