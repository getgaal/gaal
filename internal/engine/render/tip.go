package render

import (
	"fmt"
	"io"
)

// WriteTip appends a hint about output format options. Call with isTTY=true
// to actually print; piped output gets no tip.
func WriteTip(w io.Writer, isTTY bool) {
	if !isTTY {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Tip: use -o table for details, -o json for machine-readable output.")
}
