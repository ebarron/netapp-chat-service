package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ebarron/netapp-chat-service/internal/capability"
	"github.com/ebarron/netapp-chat-service/internal/config"
	"github.com/ebarron/netapp-chat-service/internal/interest"
	"github.com/ebarron/netapp-chat-service/internal/llm"
	"github.com/ebarron/netapp-chat-service/internal/mcpclient"
	"github.com/ebarron/netapp-chat-service/internal/server"
	"github.com/ebarron/netapp-chat-service/internal/session"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Set up structured logging.
	logLevel := new(slog.LevelVar)
	if os.Getenv("DEBUG") != "" {
		logLevel.Set(slog.LevelDebug)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Load configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Create LLM provider.
	provider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		logger.Error("failed to create LLM provider", "error", err)
		os.Exit(1)
	}

	// Connect to MCP servers.
	router := mcpclient.NewRouter(logger)
	defer router.Close()
	router.ConnectAll(cfg.ServerConfigs(), 5, 2*time.Second)

	// Build capabilities from config.
	caps := cfg.BuildCapabilities()

	// Build enabled-capability set for interest filtering.
	enabled := make(map[string]bool, len(caps))
	for _, c := range caps {
		if c.State != capability.StateOff {
			enabled[c.ID] = true
		}
	}

	// Load interest catalog.
	var catalog *interest.Catalog
	if len(cfg.Interests.Dirs) > 0 {
		catalog = interest.NewCatalog(logger)
		if err := catalog.Load(cfg.Interests.Dirs, enabled); err != nil {
			logger.Warn("failed to load interests", "error", err)
		}
	}

	// Assemble dependencies and start server.
	deps := &server.ChatDeps{
		Sessions:     session.NewManager(100),
		Provider:     provider,
		Router:       router,
		Logger:       logger,
		Model:        cfg.LLM.Model,
		Capabilities: caps,
		Catalog:      catalog,
		InterestsDir: firstDir(cfg.Interests.Dirs),
		PromptConfig: cfg.PromptConfig(),
	}

	srv := server.New(deps)

	logger.Info("starting chat service", "addr", cfg.Server.Addr)
	if err := http.ListenAndServe(cfg.Server.Addr, srv.Handler()); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func firstDir(dirs []string) string {
	if len(dirs) > 0 {
		return dirs[0]
	}
	return ""
}
