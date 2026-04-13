package discover

import "testing"

func TestResourceTypeConstants(t *testing.T) {
	cases := []struct {
		got  ResourceType
		want string
	}{
		{ResourceSkill, "skill"},
		{ResourceRepo, "repo"},
		{ResourceMCP, "mcp"},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("ResourceType %q: got %q", tc.want, tc.got)
		}
	}
}

func TestScopeConstants(t *testing.T) {
	cases := []struct {
		got  Scope
		want string
	}{
		{ScopeGlobal, "global"},
		{ScopeWorkspace, "workspace"},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("Scope %q: got %q", tc.want, tc.got)
		}
	}
}

func TestDriftStateConstants(t *testing.T) {
	cases := []struct {
		got  DriftState
		want string
	}{
		{DriftOK, "ok"},
		{DriftModified, "modified"},
		{DriftMissing, "missing"},
		{DriftUnmanaged, "unmanaged"},
		{DriftUnknown, "unknown"},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("DriftState %q: got %q", tc.want, tc.got)
		}
	}
}

func TestResource_zeroValue(t *testing.T) {
	var r Resource
	if r.Type != "" || r.Scope != "" || r.Drift != "" {
		t.Error("zero-value Resource should have empty Type, Scope, and Drift")
	}
	if r.Managed {
		t.Error("zero-value Resource.Managed should be false")
	}
}
