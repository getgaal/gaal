package render

import (
	"fmt"
	"io"
	"log/slog"
)

// OutputFormat controls how the status report is rendered.
type OutputFormat string

const (
	FormatText  OutputFormat = "text"
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
	case FormatText, "":
		return &textRenderer{}, nil
	case FormatTable:
		return &tableRenderer{}, nil
	case FormatJSON:
		return &jsonRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown output format %q (want: text, table, json)", f)
	}
}

// PlanRenderer formats a PlanReport and writes it to an io.Writer.
type PlanRenderer interface {
	Render(w io.Writer, r *PlanReport) error
}

// NewPlanRenderer returns a PlanRenderer for format f.
func NewPlanRenderer(f OutputFormat) (PlanRenderer, error) {
	slog.Debug("creating plan renderer", "format", f)
	switch f {
	case FormatText, "":
		return &planTextRenderer{}, nil
	case FormatTable:
		return &planTableRenderer{}, nil
	case FormatJSON:
		return &planJSONRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown output format %q (want: text, table, json)", f)
	}
}
