// Package logger provides a stage-aware, structured logger for azlift built on
// top of the standard library log/slog package. Two output formats are
// supported:
//
//   - "text" (default) — human-readable, ANSI-coloured, stage-prefixed lines
//   - "json"           — machine-readable slog JSON, suitable for log pipelines
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Stage identifies which pipeline stage is currently running.
type Stage string

const (
	StageRoot      Stage = ""
	StageScan      Stage = "SCAN"
	StageExport    Stage = "EXPORT"
	StageRefine    Stage = "REFINE"
	StageBootstrap Stage = "BOOTSTRAP"
	StageRun       Stage = "RUN"
)

// Format selects the log output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Options configures a Logger.
type Options struct {
	// Verbose enables debug-level output when true.
	Verbose bool
	// Format selects output encoding (text or json).
	Format Format
	// Writer is the output destination; defaults to os.Stderr.
	Writer io.Writer
}

// Logger wraps slog.Logger with a fixed pipeline stage label.
type Logger struct {
	inner *slog.Logger
	stage Stage
}

// New creates a Logger for the given stage.
func New(stage Stage, opts Options) *Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}

	level := slog.LevelInfo
	if opts.Verbose {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	switch opts.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	default:
		handler = newTextHandler(w, level, stage)
	}

	return &Logger{
		inner: slog.New(handler),
		stage: stage,
	}
}

// WithStage returns a new Logger stamped with a different stage label,
// inheriting the format and level of the parent.
func (l *Logger) WithStage(stage Stage) *Logger {
	// Re-attach stage to the underlying handler if it supports it.
	if h, ok := l.inner.Handler().(*textHandler); ok {
		newH := &textHandler{
			w:     h.w,
			level: h.level,
			stage: stage,
			attrs: append([]slog.Attr{}, h.attrs...),
		}
		return &Logger{inner: slog.New(newH), stage: stage}
	}
	// JSON handler: add stage as a permanent attribute.
	return &Logger{
		inner: l.inner.With(slog.String("stage", string(stage))),
		stage: stage,
	}
}

// Info logs at INFO level.
func (l *Logger) Info(msg string, args ...any) {
	l.inner.Info(msg, args...)
}

// Debug logs at DEBUG level (only visible when Verbose is true).
func (l *Logger) Debug(msg string, args ...any) {
	l.inner.Debug(msg, args...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(msg string, args ...any) {
	l.inner.Warn(msg, args...)
}

// Error logs at ERROR level.
func (l *Logger) Error(msg string, args ...any) {
	l.inner.Error(msg, args...)
}

// With returns a new Logger with the given attributes pre-attached.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{inner: l.inner.With(args...), stage: l.stage}
}

// Enabled reports whether the given level is enabled.
func (l *Logger) Enabled(level slog.Level) bool {
	return l.inner.Enabled(context.Background(), level)
}

// Slog returns the underlying *slog.Logger for use with libraries that
// accept a *slog.Logger directly.
func (l *Logger) Slog() *slog.Logger { return l.inner }
