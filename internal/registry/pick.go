package registry

import (
	"apery/internal/rng"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PickGenerator randomly selects values from a configured list
type PickGenerator struct {
	values []any
}

const (
	maxPickURLBytes = 5 << 20
	pickURLTimeout  = 5 * time.Second
)

// Next returns a random value from the list
func (p *PickGenerator) Next(r *rng.Rng) (any, error) {
	ran := r.Intn(len(p.values))
	return p.values[ran], nil
}

// validatePickConfig validates and parses config for pick generator
func validatePickConfig(config map[string]any) ([]any, error) {
	hasValues := config["values"] != nil
	hasFile := config["file"] != nil
	hasURL := config["url"] != nil

	sourceCount := 0
	if hasValues {
		sourceCount++
	}
	if hasFile {
		sourceCount++
	}
	if hasURL {
		sourceCount++
	}

	if sourceCount == 0 {
		return nil, fmt.Errorf("pick: must specify 'values', 'file', or 'url'")
	}
	if sourceCount > 1 {
		return nil, fmt.Errorf("pick: specify only one of 'values', 'file', or 'url'")
	}

	switch {
	case hasValues:
		return validatePickValues(config["values"])
	case hasFile:
		return loadPickFile(config["file"])
	default:
		return loadPickURL(config["url"], config["allowlist"])
	}
}

// validatePickValues validates the inline values array
func validatePickValues(val any) ([]any, error) {
	values, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("pick: 'values' must be an array, got %T", val)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("pick: 'values' cannot be empty")
	}
	return values, nil
}

// loadPickFile loads values from a file, one per line
func loadPickFile(fileVal any) ([]any, error) {
	path, ok := fileVal.(string)
	if !ok {
		return nil, fmt.Errorf("pick: 'file' must be a string, got %T", fileVal)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pick: cannot open file: %w", err)
	}
	defer file.Close()

	return loadPickLines(file, "file")
}

// loadPickURL loads values from a URL, one per line
func loadPickURL(urlVal any, allowlistVal any) ([]any, error) {
	rawURL, ok := urlVal.(string)
	if !ok {
		return nil, fmt.Errorf("pick: 'url' must be a string, got %T", urlVal)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("pick: invalid url: %w", err)
	}
	allowlist, err := parseAllowlist(allowlistVal)
	if err != nil {
		return nil, err
	}
	if err := validateURL(parsed, allowlist); err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: pickURLTimeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("pick: cannot fetch url: %w", err)
	}
	defer resp.Body.Close()

	if err := validateHTTPStatus(resp.StatusCode); err != nil {
		return nil, err
	}

	reader := io.LimitReader(resp.Body, maxPickURLBytes)
	return loadPickLines(reader, "url")
}

// parseAllowlist reads and validates the allowlist array.
func parseAllowlist(val any) ([]string, error) {
	if val == nil {
		return nil, nil
	}
	switch list := val.(type) {
	case []string:
		return filterAllowlist(list), nil
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("pick: 'allowlist' must contain only strings")
			}
			out = append(out, str)
		}
		return filterAllowlist(out), nil
	default:
		return nil, fmt.Errorf("pick: 'allowlist' must be an array of strings")
	}
}

// filterAllowlist drops empty host entries.
func filterAllowlist(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// hostAllowed reports whether a URL host is on the allowlist.
func hostAllowed(u *url.URL, allowlist []string) bool {
	host := u.Hostname()
	fullHost := u.Host
	for _, allowed := range allowlist {
		if allowed == host || allowed == fullHost {
			return true
		}
	}
	return false
}

// validateURL enforces URL scheme and allowlist requirements.
func validateURL(u *url.URL, allowlist []string) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("pick: url scheme must be http or https")
	}
	if len(allowlist) == 0 {
		return fmt.Errorf("pick: 'allowlist' is required for url sources")
	}
	if !hostAllowed(u, allowlist) {
		return fmt.Errorf("pick: url host %q is not allowlisted", u.Hostname())
	}
	return nil
}

// validateHTTPStatus returns an error for non-2xx responses.
func validateHTTPStatus(status int) error {
	if status < 200 || status >= 300 {
		return fmt.Errorf("pick: url returned status %d", status)
	}
	return nil
}

// loadPickLines reads non-empty lines and returns them as values.
func loadPickLines(r io.Reader, source string) ([]any, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var values []any
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		values = append(values, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("pick: error reading %s: %w", source, err)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("pick: %s is empty or contains only blank lines", source)
	}

	return values, nil
}

// init registers the pick generator.
func init() {
	MustRegister("pick", func(config map[string]any) (Generator, error) {
		values, err := validatePickConfig(config)
		if err != nil {
			return nil, err
		}
		return &PickGenerator{values: values}, nil
	})
}
