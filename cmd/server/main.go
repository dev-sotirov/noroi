package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/dev-sotirov/noroi/internal/body"
	"github.com/dev-sotirov/noroi/internal/config"
	"github.com/dev-sotirov/noroi/internal/handler"
	"github.com/dev-sotirov/noroi/internal/metrics"
	mw "github.com/dev-sotirov/noroi/internal/middleware"
	"github.com/dev-sotirov/noroi/internal/scenario"
	"github.com/dev-sotirov/noroi/internal/ui"
)

var Version = "development"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "noroi: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// -------------------------------------------------------------------------
	// CLI flags
	// -------------------------------------------------------------------------
	var (
		flagVersion   = flag.Bool("version", false, "Print version information and quit")
		flagConfig    = flag.String("config", ".noroi.yaml", "Path to .noroi.yaml config file")
		flagPort      = flag.Int("port", 0, "HTTP port (0 = use config)")
		flagDelay     = flag.String("delay", "", "Default response delay (e.g. 100ms)")
		flagErrorRate = flag.Float64("error-rate", -1, "Default error rate 0.0–1.0 (-1 = use config)")
		flagLogLevel  = flag.String("log-level", "", "Log level: debug|info|warn|error")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("noroi version %s\n", Version)
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// Config
	// -------------------------------------------------------------------------
	cfg, err := config.Load(*flagConfig)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// CLI flag overrides
	if *flagPort > 0 {
		cfg.Server.Port = *flagPort
	}
	if *flagDelay != "" {
		d, err := time.ParseDuration(*flagDelay)
		if err != nil {
			return fmt.Errorf("invalid --delay %q: %w", *flagDelay, err)
		}
		cfg.Defaults.Delay = d
	}
	if *flagErrorRate >= 0 {
		cfg.Defaults.ErrorRate = *flagErrorRate
	}
	if *flagLogLevel != "" {
		cfg.Logging.Level = config.LogLevel(*flagLogLevel)
	}

	// -------------------------------------------------------------------------
	// Logger
	// -------------------------------------------------------------------------
	logger := buildLogger(cfg.Logging.Level, cfg.Logging.Format)
	log.Logger = logger

	// -------------------------------------------------------------------------
	// Metrics registry
	// -------------------------------------------------------------------------
	reg, err := metrics.New()
	if err != nil {
		return fmt.Errorf("init metrics: %w", err)
	}

	// -------------------------------------------------------------------------
	// Handlers
	// -------------------------------------------------------------------------
	bodyGen := body.New()
	scenarioStore := scenario.New()

	respondH := &handler.Respond{
		Defaults: &cfg.Defaults,
		Body:     bodyGen,
		Metrics:  reg,
	}
	healthH := &handler.Health{}
	readyH := &handler.Ready{}
	echoH := &handler.Echo{}
	streamH := &handler.Stream{Body: bodyGen}
	scenarioSaveH := &handler.ScenarioSave{Store: scenarioStore}
	scenarioReplayH := &handler.ScenarioReplay{Store: scenarioStore, Respond: respondH}
	metricsJSONH := &handler.MetricsJSON{Metrics: reg}
	uiH := &ui.Handler{}

	// -------------------------------------------------------------------------
	// Router + middleware stack (outermost → innermost per PRD §6)
	//
	//   RequestID → Logger → Metrics → Recoverer → Handler
	//
	// RequestID is outermost so every subsequent middleware and handler can
	// read the ID via r.Header.Get("X-Request-Id").
	// -------------------------------------------------------------------------
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Logger(logger))
	r.Use(mw.Metrics(reg))
	if cfg.RateLimit.Enabled {
		r.Use(mw.RateLimit(cfg.RateLimit.RPS, cfg.RateLimit.Burst, cfg.RateLimit.TTL, cfg.RateLimit.TrustForwarded))
	}
	r.Use(chimw.Recoverer)

	r.Get("/health", healthH.ServeHTTP)
	r.Get("/ready", readyH.ServeHTTP)

	r.Get("/respond", respondH.ServeHTTP)
	r.Post("/respond", respondH.ServeHTTP)

	r.Get("/echo", echoH.ServeHTTP)
	r.Post("/echo", echoH.ServeHTTP)

	r.Get("/stream", streamH.ServeHTTP)

	r.Post("/scenario", scenarioSaveH.ServeHTTP)
	r.Get("/scenario/{id}", scenarioReplayH.ServeHTTP)

	r.Get("/metrics/json", metricsJSONH.ServeHTTP)
	r.Get("/ui", uiH.ServeHTTP)

	if cfg.Metrics.Enabled {
		metricsH := promhttp.HandlerFor(reg.Prometheus, promhttp.HandlerOpts{})
		r.Handle(cfg.Metrics.Path, metricsH)
	}

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	readyH.MarkReady()

	// -------------------------------------------------------------------------
	// Graceful shutdown
	// -------------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", srv.Addr).Msg("noroi starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server: %w", err)
	case sig := <-quit:
		logger.Info().Str("signal", sig.String()).Msg("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info().Msg("server stopped")
	}

	return nil
}

func buildLogger(level config.LogLevel, format config.LogFormat) zerolog.Logger {
	l := parseLevel(level)
	if format == config.LogFormatPretty {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(l).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stderr).Level(l).With().Timestamp().Logger()
}

func parseLevel(level config.LogLevel) zerolog.Level {
	switch level {
	case config.LogLevelDebug:
		return zerolog.DebugLevel
	case config.LogLevelWarn:
		return zerolog.WarnLevel
	case config.LogLevelError:
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
