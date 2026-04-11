package ops

import (
	"strings"
	"testing"
)

// TestInit_TemplateHasAllSections verifies the embedded template contains
// all required top-level YAML keys.
func TestInit_TemplateHasAllSections(t *testing.T) {
	for _, section := range []string{"repositories:", "skills:", "mcps:"} {
		if !strings.Contains(string(InitTemplate), section) {
			t.Errorf("InitTemplate missing section %q", section)
		}
	}
}
