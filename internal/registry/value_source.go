package registry

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxSourceURLBytes = 5 << 20
	sourceURLTimeout  = 5 * time.Second
)

// loadValues reads the value list for pick/sample from exactly one of
// 'values', 'file', or 'url', and reports whether it was inline. Files and
// URLs load once per run through the config's Env; gen prefixes errors.
func loadValues(config map[string]any, gen string) ([]any, bool, error) {
	hasValues := config["values"] != nil
	hasFile := config["file"] != nil
	hasURL := config["url"] != nil

	sourceCount := 0
	for _, has := range []bool{hasValues, hasFile, hasURL} {
		if has {
			sourceCount++
		}
	}
	if sourceCount == 0 {
		return nil, false, fmt.Errorf("%s: must specify 'values', 'file', or 'url'", gen)
	}
	if sourceCount > 1 {
		return nil, false, fmt.Errorf("%s: specify only one of 'values', 'file', or 'url'", gen)
	}

	env := envFrom(config)
	switch {
	case hasValues:
		values, err := inlineValues(config["values"], gen)
		return values, true, err
	case hasFile:
		values, err := fileValues(config["file"], env, gen)
		return values, false, err
	default:
		values, err := urlValues(config["url"], config["allowlist"], env, gen)
		return values, false, err
	}
}

// inlineValues validates the inline values array.
func inlineValues(val any, gen string) ([]any, error) {
	values, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: 'values' must be an array, got %T", gen, val)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s: 'values' cannot be empty", gen)
	}
	return values, nil
}

// fileValues loads values from a file, one per line. Relative paths resolve
// against the Env's base directory (the plan file's directory).
func fileValues(fileVal any, env *Env, gen string) ([]any, error) {
	path, ok := fileVal.(string)
	if !ok {
		return nil, fmt.Errorf("%s: 'file' must be a string, got %T", gen, fileVal)
	}
	path = env.resolvePath(path)

	return env.loadOnce("file:"+path, func() ([]any, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot open file: %w", gen, err)
		}
		defer file.Close()
		return readLines(file, "file", gen)
	})
}

// urlValues fetches values from an allowlisted URL, one per line. Every
// redirect hop must also be allowlisted.
func urlValues(urlVal any, allowlistVal any, env *Env, gen string) ([]any, error) {
	rawURL, ok := urlVal.(string)
	if !ok {
		return nil, fmt.Errorf("%s: 'url' must be a string, got %T", gen, urlVal)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid url: %w", gen, err)
	}
	allowlist, err := parseAllowlist(allowlistVal, gen)
	if err != nil {
		return nil, err
	}
	if err := validateURL(parsed, allowlist, gen); err != nil {
		return nil, err
	}

	return env.loadOnce("url:"+rawURL, func() ([]any, error) {
		client := &http.Client{
			Timeout: sourceURLTimeout,
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return validateURL(req.URL, allowlist, gen)
			},
		}
		resp, err := client.Get(rawURL)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot fetch url: %w", gen, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: url returned status %d", gen, resp.StatusCode)
		}
		return readLines(io.LimitReader(resp.Body, maxSourceURLBytes), "url", gen)
	})
}

// parseAllowlist reads the allowlist array, dropping empty entries.
func parseAllowlist(val any, gen string) ([]string, error) {
	if val == nil {
		return nil, nil
	}

	var raw []string
	switch list := val.(type) {
	case []string:
		raw = list
	case []any:
		for _, item := range list {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s: 'allowlist' must contain only strings", gen)
			}
			raw = append(raw, str)
		}
	default:
		return nil, fmt.Errorf("%s: 'allowlist' must be an array of strings", gen)
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

// validateURL enforces the http(s) scheme and the host allowlist. An entry
// matches the bare hostname or host:port exactly.
func validateURL(u *url.URL, allowlist []string, gen string) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s: url scheme must be http or https", gen)
	}
	if len(allowlist) == 0 {
		return fmt.Errorf("%s: 'allowlist' is required for url sources", gen)
	}
	for _, allowed := range allowlist {
		if allowed == u.Hostname() || allowed == u.Host {
			return nil
		}
	}
	return fmt.Errorf("%s: url host %q is not allowlisted", gen, u.Host)
}

// readLines reads non-empty, trimmed lines as values.
func readLines(r io.Reader, source, gen string) ([]any, error) {
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
		return nil, fmt.Errorf("%s: error reading %s: %w", gen, source, err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s: %s is empty or contains only blank lines", gen, source)
	}
	return values, nil
}
