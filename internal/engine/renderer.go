package engine

import (
	"fmt"
	"io"
	"log/slog"
)

// OutputFormat controls how the status report is rendered.
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
)

// Renderer formats a StatusReport and writes it to an io.Writer.
type Renderer interface {
	Render(w io.Writer, r *StatusReport) error
}

// NewRenderer returns a Renderer for format f, or an error for unknown formats.
func NewRenderer(f OutputFormat) (Renderer, error) {
	slog.Debug("creating renderer", "format", f)
	switch f {
	case FormatTable:
		return &tableRenderer{}, nil
	case FormatJSON:
		return &jsonRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown output format %q (want: table, json)", f)
	}
}
