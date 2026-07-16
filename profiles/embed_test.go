package profiles

import "testing"

func TestLoadEmbeddedWebBlackbox(t *testing.T) {
	profile, err := Load("web-blackbox")
	if err != nil || profile.Limits.ArjunMaxTargets != 25 || len(profile.Tools) != 3 {
		t.Fatalf("profile = %#v, %v", profile, err)
	}
	if _, err := Load("future"); err == nil {
		t.Fatal("unsupported profile loaded")
	}
}
