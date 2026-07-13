package scope

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/vtrpza/reconctx/internal/canonical"
)

type Evaluator struct {
	rules []rule
}

type rule struct {
	id     string
	kind   string
	value  string
	origin string
	path   string
}

func NewEvaluator(config Config) (*Evaluator, error) {
	if config.Mode != "allowlist" {
		return nil, fmt.Errorf("unsupported scope mode %q", config.Mode)
	}
	if config.ExternalPolicy != "reject" && config.ExternalPolicy != "record_only" {
		return nil, fmt.Errorf("unsupported external policy %q", config.ExternalPolicy)
	}
	if len(config.Roots) == 0 {
		return nil, fmt.Errorf("scope requires at least one root")
	}
	evaluator := &Evaluator{rules: make([]rule, 0, len(config.Roots))}
	seenIDs := make(map[string]bool, len(config.Roots))
	for index, root := range config.Roots {
		compiled, err := compileRoot(root, index)
		if err != nil {
			return nil, err
		}
		if seenIDs[compiled.id] {
			return nil, fmt.Errorf("duplicate scope root ID %q", compiled.id)
		}
		seenIDs[compiled.id] = true
		evaluator.rules = append(evaluator.rules, compiled)
	}
	return evaluator, nil
}

func compileRoot(root Root, index int) (rule, error) {
	id := root.ID
	if id == "" {
		id = fmt.Sprintf("scope_root_%d", index+1)
	}
	compiled := rule{id: id, kind: root.Kind}
	switch root.Kind {
	case "origin":
		parsed, err := url.Parse(root.Value)
		if err != nil || parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
			return rule{}, fmt.Errorf("scope root %s: origin must not include a path", id)
		}
		value, err := canonical.CanonicalizeURL(root.Value)
		if err != nil {
			return rule{}, fmt.Errorf("scope root %s: %w", id, err)
		}
		if value.CanonicalRouteURL != value.Origin+"/" || value.QueryPresent || value.FragmentPresent {
			return rule{}, fmt.Errorf("scope root %s: origin must not include path, query, or fragment", id)
		}
		compiled.value = value.Origin
	case "host":
		if strings.ContainsAny(root.Value, "/?#@") {
			return rule{}, fmt.Errorf("scope root %s: invalid host root", id)
		}
		host := root.Value
		if address, err := netip.ParseAddr(host); err == nil && address.Is6() {
			host = "[" + host + "]"
		} else if strings.Contains(host, ":") {
			return rule{}, fmt.Errorf("scope root %s: invalid host root", id)
		}
		value, err := canonical.CanonicalizeURL("https://" + host + "/")
		if err != nil {
			return rule{}, fmt.Errorf("scope root %s: %w", id, err)
		}
		compiled.value = value.Host
	case "url_prefix":
		if hasDotSegment(root.Value) {
			return rule{}, fmt.Errorf("scope root %s: URL prefix contains a dot segment", id)
		}
		value, err := canonical.CanonicalizeURL(root.Value)
		if err != nil {
			return rule{}, fmt.Errorf("scope root %s: %w", id, err)
		}
		if hasAmbiguousPathEscape(value.Path) {
			return rule{}, fmt.Errorf("scope root %s: URL prefix contains an encoded separator", id)
		}
		if value.QueryPresent || value.FragmentPresent {
			return rule{}, fmt.Errorf("scope root %s: URL prefix must not include query or fragment", id)
		}
		compiled.origin, compiled.path = value.Origin, value.Path
	default:
		return rule{}, fmt.Errorf("scope root %s: unsupported kind %q", id, root.Kind)
	}
	return compiled, nil
}

func hasDotSegment(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." {
			return true
		}
	}
	return false
}

func (evaluator *Evaluator) EvaluateURL(rawURL string) Decision {
	value, err := canonical.CanonicalizeURL(rawURL)
	if err != nil {
		return Decision{Classification: Unknown, Reason: "URL could not be canonicalized: " + err.Error()}
	}
	return evaluator.evaluate(value)
}

func (evaluator *Evaluator) evaluate(value canonical.URL) Decision {
	if evaluator == nil {
		return Decision{Classification: Unknown, Reason: "scope evaluator is unavailable"}
	}
	for _, rule := range evaluator.rules {
		matched := false
		switch rule.kind {
		case "origin":
			matched = value.Origin == rule.value
		case "host":
			matched = value.Host == rule.value
		case "url_prefix":
			matched = value.Origin == rule.origin && pathPrefix(value.Path, rule.path)
		}
		if matched {
			id := rule.id
			return Decision{Classification: InScope, RuleID: &id, Reason: rule.kind + " allowlist root matched"}
		}
	}
	return Decision{Classification: OutOfScope, Reason: "no allowlist root matched"}
}

func pathPrefix(path, prefix string) bool {
	if hasAmbiguousPathEscape(path) {
		return false
	}
	if prefix == "/" || path == prefix {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(path, prefix)
	}
	return strings.HasPrefix(path, prefix+"/")
}

func hasAmbiguousPathEscape(path string) bool {
	return strings.Contains(path, "%2F") || strings.Contains(path, "%5C") || strings.Contains(path, "%25")
}
