// Package logger wires zerolog: structured JSON logs in production, human-friendly
// console output in development. Exposes a package-level logger and a helper to
// derive context-scoped loggers (with request_id / order_id / etc.).
package logger

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init configures the global zerolog logger. Call once at startup, after config.Load.
func Init(level string, isDev bool) {
	zerolog.TimeFieldFormat = time.RFC3339

	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	if isDev {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Caller().Logger()
		return
	}

	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// FromContext returns a logger enriched with any fields stored under ctxKey.
// If none, returns the global logger.
func FromContext(ctx context.Context) *zerolog.Logger {
	if l := zerolog.Ctx(ctx); l != nil && l.GetLevel() != zerolog.Disabled {
		return l
	}
	return &log.Logger
}

// WithRequestID returns a context carrying a logger tagged with request_id.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	l := log.Logger.With().Str("request_id", requestID).Logger()
	return l.WithContext(ctx)
}
