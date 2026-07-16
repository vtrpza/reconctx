package profiles

import (
	"bytes"
	_ "embed"
	"errors"
	"io"

	"go.yaml.in/yaml/v3"
)

//go:embed web-blackbox.yaml
var webBlackbox []byte

type Profile struct {
	ProfileVersion       string   `yaml:"profile_version"`
	Name                 string   `yaml:"name"`
	EnvironmentAllowlist []string `yaml:"environment_allowlist"`
	Limits               Limits   `yaml:"limits"`
	Tools                []Tool   `yaml:"tools"`
}

type Limits struct {
	ArjunMaxTargets int `yaml:"arjun_max_targets"`
}

type Tool struct {
	Name                    string `yaml:"name"`
	ActivityClass           string `yaml:"activity_class"`
	RatePerSecond           int    `yaml:"rate_limit_per_second"`
	Concurrency             int    `yaml:"concurrency"`
	Parallelism             int    `yaml:"parallelism"`
	RequestTimeoutSeconds   int    `yaml:"timeout_seconds"`
	ExecutionTimeoutSeconds int    `yaml:"execution_timeout_seconds"`
}

func Load(name string) (Profile, error) {
	if name != "web-blackbox" {
		return Profile{}, errors.New("unsupported profile")
	}
	var profile Profile
	decoder := yaml.NewDecoder(bytes.NewReader(webBlackbox))
	decoder.KnownFields(true)
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Profile{}, errors.New("invalid embedded profile document")
	}
	if profile.ProfileVersion != "reconctx-profile/v0" || profile.Name != name || profile.Limits.ArjunMaxTargets <= 0 || len(profile.EnvironmentAllowlist) == 0 || len(profile.Tools) != 3 {
		return Profile{}, errors.New("invalid embedded profile")
	}
	seen := map[string]bool{}
	for _, tool := range profile.Tools {
		if seen[tool.Name] || tool.Name != "gau" && tool.Name != "katana" && tool.Name != "arjun" || tool.ActivityClass == "" || tool.RatePerSecond <= 0 || tool.Concurrency <= 0 || tool.Parallelism <= 0 || tool.RequestTimeoutSeconds <= 0 || tool.ExecutionTimeoutSeconds <= tool.RequestTimeoutSeconds {
			return Profile{}, errors.New("invalid embedded tool profile")
		}
		seen[tool.Name] = true
	}
	return profile, nil
}
