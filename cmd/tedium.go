package main

import (
	"flag"
	"os"

	"github.com/markormesher/tedium/internal/entrypoints"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
)

var l = logging.Logger

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
		l.Error("Config file not provided")
		os.Exit(1)
	}

	conf, err := schema.LoadTediumConfig(*configFilePath)
	if err != nil {
		l.Error("Error loading configuration", "error", err)
		os.Exit(1)
	}

	entrypoints.Run(conf)
}
