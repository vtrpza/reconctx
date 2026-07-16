package profiles

import "testing"

func TestLoadEmbeddedWebBlackbox(t *testing.T) {
	profile, err := Load("web-blackbox")
	if err != nil || profile.Limits.ArjunMaxTargets != 25 || len(profile.Tools) != 3 {
		t.Fatalf("profile = %#v, %v", profile, err)
	}
	want := map[string]struct {
		request   int
		execution int
	}{
		"gau":    {request: 45, execution: 900},
		"katana": {request: 10, execution: 7200},
		"arjun":  {request: 15, execution: 7200},
	}
	for _, tool := range profile.Tools {
		expected, ok := want[tool.Name]
		if !ok || tool.RequestTimeoutSeconds != expected.request || tool.ExecutionTimeoutSeconds != expected.execution {
			t.Errorf("%s timeouts = request %ds, execution %ds; want request %ds, execution %ds", tool.Name, tool.RequestTimeoutSeconds, tool.ExecutionTimeoutSeconds, expected.request, expected.execution)
		}
	}
	if _, err := Load("future"); err == nil {
		t.Fatal("unsupported profile loaded")
	}
}
