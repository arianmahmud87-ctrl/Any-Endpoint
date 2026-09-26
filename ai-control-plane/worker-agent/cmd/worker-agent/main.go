package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/local/ai-control-plane/worker-agent/internal/runtime"
)

func main() {
	cfg, err := runtime.LoadConfig()
	if err == nil {
		err = runtime.Run(cfg)
	}
	if err != nil {
		slog.Error("worker_agent_exit", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
