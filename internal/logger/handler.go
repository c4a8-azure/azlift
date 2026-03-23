package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ANSI colour codes used for human-readable output.
const (
	colReset  = "\033[0m"
	colBold   = "\033[1m"
	colGray   = "\033[90m"
	colRed    = "\033[31m"
	colYellow = "\033[33m"
	colCyan   = "\033[36m"
	colBlue   = "\033[34m"
	colGreen  = "\033[32m"
	colPurple = "\033[35m"
	colWhite  = "\033[97m"
)

// stageColour maps a Stage to an ANSI colour prefix.
var stageColour = map[Stage]string{
	StageScan:      colCyan,
	StageExport:    colBlue,
	StageRefine:    colPurple,
	StageBootstrap: colGreen,
	StageRun:       colYellow,
	StageRoot:      colWhite,
}

// textHandler is a custom slog.Handler that produces coloured, stage-prefixed,
// human-readable lines. Format:
//
//	HH:MM:SS [STAGE] LEVEL  message   key=value key=value
type textHandler struct {
	mu    sync.Mutex
	w     io.Writer
	level slog.Level
	stage Stage
	attrs []slog.Attr
}

func newTextHandler(w io.Writer, level slog.Level, stage Stage) *textHandler {
	return &textHandler{w: w, level: level, stage: stage}
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	// Timestamp
	buf.WriteString(colGray)
	buf.WriteString(r.Time.Format(time.TimeOnly))
	buf.WriteString(colReset)
	buf.WriteByte(' ')

	// Stage prefix
	if h.stage != StageRoot {
		col := stageColour[h.stage]
		fmt.Fprintf(&buf, "%s%s[%-9s]%s ", colBold, col, string(h.stage), colReset)
	}

	// Level
	buf.WriteString(levelColour(r.Level))
	buf.WriteString(levelLabel(r.Level))
	buf.WriteString(colReset)
	buf.WriteByte(' ')

	// Message
	buf.WriteString(r.Message)

	// Pre-attached attrs (from With calls)
	for _, a := range h.attrs {
		writeAttr(&buf, a)
	}

	// Record attrs
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&buf, a)
		return true
	})

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &textHandler{
		w:     h.w,
		level: h.level,
		stage: h.stage,
		attrs: append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	// Groups are flattened — azlift logs are simple k=v lines.
	_ = name
	return h
}

func writeAttr(buf *bytes.Buffer, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprintf(buf, "  %s%s%s=%s", colGray, a.Key, colReset, formatValue(a.Value))
}

func formatValue(v slog.Value) string {
	s := v.String()
	if strings.ContainsAny(s, " \t") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

func levelColour(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return colRed
	case l >= slog.LevelWarn:
		return colYellow
	case l >= slog.LevelInfo:
		return colGreen
	default:
		return colGray
	}
}

func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERR "
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DBG "
	}
}
