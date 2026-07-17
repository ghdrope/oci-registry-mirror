package main

import (
	"os"

	"go.uber.org/zap"
	"k8s.io/sample-controller/pkg/signals"
)

// main bootstraps the OCI Registry Mirror process.
//
// Responsibilities:
//   - initialize logging
//   - install OS signal handling for graceful shutdown (SIGINT/SIGTERM)
//   - delegate execution to the CLI root command
//
// The process exits with status 1 if any fatal error occurs during runtime.
func main() {

	ctx := signals.SetupSignalHandler()

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = logger.Sync()
	}()

	// Set global logger
	zap.ReplaceGlobals(logger)

	if err := Execute(ctx); err != nil {
		zap.L().Error("fatal error during execution", zap.Error(err))
		os.Exit(1)
	}

	zap.L().Info("shutdown complete")
}
