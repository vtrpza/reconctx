package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

const URLPolicyVersion = "url-canonicalization/v0"

var (
	methodToken = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)
	endpointID  = regexp.MustCompile(`^ep_sha256_[0-9a-f]{64}$`)
	hostLower   = cases.Lower(language.Und)
	idnaProfile = idna.New(
		idna.MapForLookup(),
		idna.Transitional(false),
		idna.StrictDomainName(true),
		idna.ValidateLabels(true),
		idna.VerifyDNSLength(true),
		idna.BidiRule(),
	)
)

var parameterLocations = map[string]bool{
	"query": true, "form": true, "json": true, "header": true,
	"cookie": true, "path": true, "unknown": true,
}

type QueryPair struct {
	Index     int     `json:"index"`
	RawName   string  `json:"raw_name"`
	RawValue  *string `json:"raw_value"`
	Name      string  `json:"name"`
	Value     *string `json:"value"`
	HasEquals bool    `json:"has_equals"`
}

type URL struct {
	RawURL                  string      `json:"raw_url"`
	Scheme                  string      `json:"scheme"`
	Host                    string      `json:"host"`
	Port                    *int        `json:"port"`
	Origin                  string      `json:"origin"`
	Path                    string      `json:"path"`
	QueryPresent            bool        `json:"query_present"`
	QueryRaw                *string     `json:"query_raw"`
	QueryCanonical          *string     `json:"query_canonical"`
	QueryPairs              []QueryPair `json:"query_pairs"`
	FragmentPresent         bool        `json:"fragment_present"`
	FragmentRaw             *string     `json:"fragment_raw"`
	CanonicalRouteURL       string      `json:"canonical_route_url"`
	CanonicalObservationURL string      `json:"canonical_observation_url"`
	Warnings                []string    `json:"warnings"`
}

type SourceMethod struct {
	SourceLabel       *string `json:"source_label"`
	HTTPMethod        *string `json:"http_method"`
	MethodKnown       bool    `json:"method_known"`
	BodyKind          string  `json:"body_kind"`
	ParameterLocation string  `json:"parameter_location"`
}

func CanonicalizeURL(raw string) (URL, error) {
	if raw == "" {
		return URL{}, errors.New("URL must be a non-empty string")
	}
	for _, character := range raw {
		if character == '\\' || character < 0x20 || character == 0x7f {
			return URL{}, errors.New("URL contains an ambiguous slash or control character")
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return URL{}, fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return URL{}, errors.New("only absolute HTTP(S) URLs are supported")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return URL{}, errors.New("absolute URL authority is required")
	}
	if parsed.User != nil || strings.Contains(parsed.Host, "@") {
		return URL{}, errors.New("userinfo is forbidden")
	}
	if strings.HasPrefix(parsed.Host, "[") {
		address, err := netip.ParseAddr(parsed.Hostname())
		if err != nil || !address.Is6() {
			return URL{}, errors.New("bracketed authority must contain an IPv6 address")
		}
	}

	host, ipv6, err := canonicalHost(parsed.Hostname())
	if err != nil {
		return URL{}, err
	}
	port, err := canonicalPort(parsed.Port(), scheme)
	if err != nil {
		return URL{}, err
	}
	authorityHost := host
	if ipv6 {
		authorityHost = "[" + host + "]"
	}
	authority := authorityHost
	if port != nil {
		authority += ":" + strconv.Itoa(*port)
	}
	origin := scheme + "://" + authority

	pathRaw, queryRaw, queryPresent, fragmentRaw, fragmentPresent, err := rawComponents(raw, scheme)
	if err != nil {
		return URL{}, err
	}
	if fragmentPresent {
		if _, err := normalizePercentComponent(fragmentRaw, querySafe, "fragment"); err != nil {
			return URL{}, err
		}
	}
	if pathRaw == "" {
		pathRaw = "/"
	}
	normalizedPath, err := normalizePercentComponent(pathRaw, pathSafe, "path")
	if err != nil {
		return URL{}, err
	}
	path := removeDotSegments(normalizedPath)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return URL{}, errors.New("HTTP URL path must be absolute")
	}

	pairs := []QueryPair{}
	var queryCanonical *string
	var rawQueryPointer *string
	if queryPresent {
		pairs, normalizedPath, err = queryPairs(queryRaw)
		if err != nil {
			return URL{}, err
		}
		queryCanonical = pointer(normalizedPath)
		rawQueryPointer = pointer(queryRaw)
	}
	route := origin + path
	observation := route
	if queryPresent {
		observation += "?" + *queryCanonical
	}
	warnings := []string{}
	var rawFragmentPointer *string
	if fragmentPresent {
		warnings = append(warnings, "fragment_removed")
		rawFragmentPointer = pointer(fragmentRaw)
	}
	return URL{
		RawURL: raw, Scheme: scheme, Host: host, Port: port, Origin: origin, Path: path,
		QueryPresent: queryPresent, QueryRaw: rawQueryPointer, QueryCanonical: queryCanonical,
		QueryPairs: pairs, FragmentPresent: fragmentPresent, FragmentRaw: rawFragmentPointer,
		CanonicalRouteURL: route, CanonicalObservationURL: observation, Warnings: warnings,
	}, nil
}

func canonicalHost(raw string) (string, bool, error) {
	host := hostLower.String(strings.TrimRight(norm.NFC.String(raw), "."))
	if host == "" || strings.Contains(host, "%") || strings.ContainsRune(".。．｡", []rune(host)[0]) {
		return "", false, errors.New("invalid or empty host")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return strings.ToLower(address.String()), address.Is6(), nil
	}
	if nonStandardNumericHost(host) {
		return "", false, errors.New("non-standard numeric IP address is forbidden")
	}
	mapped, err := idnaProfile.ToUnicode(host)
	if err != nil {
		return "", false, fmt.Errorf("invalid IDNA host: %w", err)
	}
	mapped = strings.TrimRight(mapped, ".")
	if mapped == "" || strings.HasPrefix(mapped, ".") || strings.Contains(mapped, "..") {
		return "", false, errors.New("invalid IDNA host")
	}
	for _, character := range mapped {
		if character != '.' && !idna15Allows(character) {
			return "", false, fmt.Errorf("invalid IDNA host code point U+%04X", character)
		}
	}
	if !validIDNA15Contexts(mapped) {
		return "", false, errors.New("invalid IDNA host code point context")
	}
	ascii, err := idnaProfile.ToASCII(mapped)
	if err != nil {
		return "", false, fmt.Errorf("invalid IDNA host: %w", err)
	}
	ascii = strings.TrimRight(ascii, ".")
	if ascii == "" || strings.HasPrefix(ascii, ".") || strings.Contains(ascii, "..") {
		return "", false, errors.New("invalid IDNA host")
	}
	if _, err := netip.ParseAddr(ascii); err == nil || nonStandardNumericHost(ascii) {
		return "", false, errors.New("non-standard numeric IP address is forbidden")
	}
	return strings.ToLower(ascii), false, nil
}

// ponytail: DNS labels are capped at 63 bytes, so direct context scans stay bounded.
func validIDNA15Contexts(host string) bool {
	for _, rawLabel := range strings.Split(host, ".") {
		label := []rune(rawLabel)
		for position, character := range label {
			switch {
			case character == 0x00b7:
				if position == 0 || position == len(label)-1 || label[position-1] != 'l' || label[position+1] != 'l' {
					return false
				}
			case character == 0x0375:
				if position == len(label)-1 || !unicode.Is(unicode.Greek, label[position+1]) {
					return false
				}
			case character == 0x05f3 || character == 0x05f4:
				if position == 0 || !unicode.Is(unicode.Hebrew, label[position-1]) {
					return false
				}
			case character == 0x30fb:
				valid := false
				for _, candidate := range label {
					valid = valid || unicode.Is(unicode.Hiragana, candidate) || unicode.Is(unicode.Katakana, candidate) || unicode.Is(unicode.Han, candidate)
				}
				if !valid {
					return false
				}
			case character >= 0x0660 && character <= 0x0669:
				for _, candidate := range label {
					if candidate >= 0x06f0 && candidate <= 0x06f9 {
						return false
					}
				}
			case character >= 0x06f0 && character <= 0x06f9:
				for _, candidate := range label {
					if candidate >= 0x0660 && candidate <= 0x0669 {
						return false
					}
				}
			}
		}
	}
	return true
}

func nonStandardNumericHost(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		if strings.IndexFunc(label, func(r rune) bool { return !unicode.IsDigit(r) }) == -1 {
			continue
		}
		lower := strings.ToLower(label)
		if len(lower) <= 2 || !strings.HasPrefix(lower, "0x") || strings.IndexFunc(lower[2:], func(r rune) bool {
			return !unicode.IsDigit(r) && (r < 'a' || r > 'f')
		}) != -1 {
			return false
		}
	}
	return true
}

func canonicalPort(raw, scheme string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("port out of range")
	}
	if scheme == "http" && port == 80 || scheme == "https" && port == 443 {
		return nil, nil
	}
	return &port, nil
}

func rawComponents(raw, scheme string) (path, query string, queryPresent bool, fragment string, fragmentPresent bool, err error) {
	authorityStart := len(scheme) + 3
	if len(raw) < authorityStart || raw[len(scheme):authorityStart] != "://" {
		return "", "", false, "", false, errors.New("absolute URL authority is required")
	}
	withoutFragment := raw
	if index := strings.IndexByte(raw, '#'); index >= 0 {
		fragmentPresent = true
		fragment = raw[index+1:]
		withoutFragment = raw[:index]
	}
	beforeQuery := withoutFragment
	if index := strings.IndexByte(withoutFragment[authorityStart:], '?'); index >= 0 {
		index += authorityStart
		queryPresent = true
		query = withoutFragment[index+1:]
		beforeQuery = withoutFragment[:index]
	}
	if index := strings.IndexByte(beforeQuery[authorityStart:], '/'); index >= 0 {
		path = beforeQuery[authorityStart+index:]
	}
	return path, query, queryPresent, fragment, fragmentPresent, nil
}

const (
	unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	pathSafe   = unreserved + "/:@!$&'()*+,;="
	querySafe  = unreserved + "/:?@!$'()*+,;="
)

func normalizePercentComponent(value, safe, label string) (string, error) {
	value = norm.NFC.String(value)
	var result strings.Builder
	for index := 0; index < len(value); {
		if value[index] == '%' {
			if index+2 >= len(value) || !isHex(value[index+1]) || !isHex(value[index+2]) {
				return "", fmt.Errorf("invalid percent escape in %s", label)
			}
			decoded, _ := strconv.ParseUint(value[index+1:index+3], 16, 8)
			if decoded < utf8.RuneSelf && strings.ContainsRune(unreserved, rune(decoded)) {
				result.WriteByte(byte(decoded))
			} else {
				fmt.Fprintf(&result, "%%%02X", decoded)
			}
			index += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("invalid UTF-8 in %s", label)
		}
		if r < utf8.RuneSelf && strings.ContainsRune(safe, r) {
			result.WriteRune(r)
		} else {
			for _, encoded := range []byte(string(r)) {
				fmt.Fprintf(&result, "%%%02X", encoded)
			}
		}
		index += size
	}
	return result.String(), nil
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func removeDotSegments(path string) string {
	input, output := path, ""
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = input[3:]
		case strings.HasPrefix(input, "./"):
			input = input[2:]
		case strings.HasPrefix(input, "/./"):
			input = "/" + input[3:]
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = "/" + input[4:]
			output = removeLastSegment(output)
		case input == "/..":
			input = "/"
			output = removeLastSegment(output)
		case input == "." || input == "..":
			input = ""
		default:
			start := 0
			if strings.HasPrefix(input, "/") {
				start = 1
			}
			next := strings.Index(input[start:], "/")
			if next < 0 {
				output += input
				input = ""
			} else {
				next += start
				output += input[:next]
				input = input[next:]
			}
		}
	}
	return output
}

func removeLastSegment(value string) string {
	if slash := strings.LastIndexByte(value, '/'); slash >= 0 {
		return value[:slash]
	}
	return ""
}

func queryPairs(raw string) ([]QueryPair, string, error) {
	if raw == "" {
		return []QueryPair{}, "", nil
	}
	components := strings.Split(raw, "&")
	pairs := make([]QueryPair, 0, len(components))
	canonical := make([]string, 0, len(components))
	for index, component := range components {
		rawName, rawValue, hasEquals := strings.Cut(component, "=")
		name, err := normalizePercentComponent(rawName, querySafe, "query name")
		if err != nil {
			return nil, "", err
		}
		pair := QueryPair{Index: index, RawName: rawName, Name: name, HasEquals: hasEquals}
		encoded := name
		if hasEquals {
			value, err := normalizePercentComponent(rawValue, querySafe, "query value")
			if err != nil {
				return nil, "", err
			}
			pair.RawValue, pair.Value = pointer(rawValue), pointer(value)
			encoded += "=" + value
		}
		pairs = append(pairs, pair)
		canonical = append(canonical, encoded)
	}
	return pairs, strings.Join(canonical, "&"), nil
}

func NormalizeSourceMethod(sourceLabel *string, tool string) (SourceMethod, error) {
	if sourceLabel == nil {
		return SourceMethod{BodyKind: "unknown", ParameterLocation: "unknown"}, nil
	}
	label := strings.ToUpper(*sourceLabel)
	if strings.EqualFold(tool, "arjun") {
		mapping := map[string][3]string{
			"GET": {"GET", "none", "query"}, "POST": {"POST", "form", "form"}, "JSON": {"POST", "json", "json"},
		}
		values, ok := mapping[label]
		if !ok {
			return SourceMethod{}, fmt.Errorf("unsupported Arjun method mode %q", *sourceLabel)
		}
		return SourceMethod{SourceLabel: pointer(label), HTTPMethod: pointer(values[0]), MethodKnown: true, BodyKind: values[1], ParameterLocation: values[2]}, nil
	}
	method, err := httpMethod(*sourceLabel)
	if err != nil {
		return SourceMethod{}, err
	}
	return SourceMethod{SourceLabel: pointer(*sourceLabel), HTTPMethod: pointer(method), MethodKnown: true, BodyKind: "unknown", ParameterLocation: "unknown"}, nil
}

func EndpointID(method *string, rawOrRouteURL string) (string, error) {
	canonical, err := CanonicalizeURL(rawOrRouteURL)
	if err != nil {
		return "", err
	}
	canonicalMethod := "*"
	if method != nil {
		canonicalMethod, err = httpMethod(*method)
		if err != nil {
			return "", err
		}
		if canonicalMethod == "*" {
			canonicalMethod = `\*`
		}
	}
	return stableHash("ep", canonicalMethod, canonical.CanonicalRouteURL), nil
}

func ParameterID(endpoint, location, name string) (string, error) {
	if !endpointID.MatchString(endpoint) {
		return "", errors.New("invalid endpoint ID")
	}
	if !parameterLocations[location] {
		return "", errors.New("invalid parameter location")
	}
	if !utf8.ValidString(name) {
		return "", errors.New("parameter name contains invalid UTF-8")
	}
	name = norm.NFC.String(name)
	if name == "" {
		return "", errors.New("parameter name cannot be empty")
	}
	return stableHash("param", endpoint, location, name), nil
}

func httpMethod(method string) (string, error) {
	if !methodToken.MatchString(method) {
		return "", errors.New("invalid HTTP method token")
	}
	return strings.ToUpper(method), nil
}

func stableHash(prefix string, components ...string) string {
	material := strings.Join(append([]string{"reconctx-" + prefix + "-v0"}, components...), "\x00")
	digest := sha256.Sum256([]byte(material))
	return prefix + "_sha256_" + hex.EncodeToString(digest[:])
}

func pointer(value string) *string { return &value }
