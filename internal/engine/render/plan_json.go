package render

import (
	"encoding/json"
	"io"
	"log/slog"
)

type planJSONRenderer struct{}

func (j *planJSONRenderer) Render(w io.Writer, r *PlanReport) error {
	slog.Debug("rendering plan json output")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
