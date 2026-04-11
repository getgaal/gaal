package render

import (
	"encoding/json"
	"io"
	"log/slog"
)

// jsonRenderer implements Renderer by encoding the report as indented JSON.
type jsonRenderer struct{}

func (j *jsonRenderer) Render(w io.Writer, r *StatusReport) error {
	slog.Debug("rendering json output")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
