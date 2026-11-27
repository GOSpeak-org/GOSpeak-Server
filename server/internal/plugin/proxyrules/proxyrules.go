package proxyrules

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func ValidateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid server URL: %v", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return errors.New("invalid server URL: must be an absolute http(s) URL")
	}
	if u.Host == "" {
		return errors.New("invalid server URL: must be an absolute http(s) URL")
	}
	return nil
}

// NormalizeBaseURL returns the canonical string form of a backend base URL:
// surrounding whitespace and every trailing slash are dropped, while an optional
// path prefix keeps its own content. "https://gospeak.example.com/" and
// "https://gospeak.example.com" address the same server, so only one form may be
// persisted, compared or echoed back to the user.
func NormalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func RewriteSetCookie(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	kept := make([]string, 0, 8)
	sameSiteSet := false
	for _, raw := range strings.Split(value, ";") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		name := part
		if i := strings.IndexByte(part, '='); i >= 0 {
			name = part[:i]
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "domain" || name == "secure" {
			continue
		}
		if name == "samesite" {
			if sameSiteSet {
				continue
			}
			sameSiteSet = true
			kept = append(kept, "SameSite=Lax")
			continue
		}
		kept = append(kept, part)
	}
	if !sameSiteSet {
		kept = append(kept, "SameSite=Lax")
	}
	return strings.Join(kept, "; ")
}

func IsTrustedWorker(target, baseURL, allowed string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return false
	}
	if u.User != nil {
		return false
	}
	var scheme string
	switch strings.ToLower(u.Scheme) {
	case "ws", "http":
		scheme = "http"
	case "wss", "https":
		scheme = "https"
	default:
		return false
	}
	want, err := NormalizedHostPort(scheme, u.Host)
	if err != nil {
		return false
	}
	if base, err := url.Parse(strings.TrimSpace(baseURL)); err == nil && base.Host != "" {
		if hp, err := NormalizedHostPort(base.Scheme, base.Host); err == nil && hp == want {
			return true
		}
	}
	for _, entry := range strings.Split(allowed, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if hp, err := NormalizedHostPort(scheme, entry); err == nil && hp == want {
			return true
		}
	}
	return false
}

func NormalizedHostPort(scheme, hostport string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(hostport))
	if h == "" {
		return "", errors.New("empty host:port")
	}
	host, port := h, ""
	if strings.HasPrefix(h, "[") {
		end := strings.Index(h, "]")
		if end < 0 {
			return "", fmt.Errorf("unterminated IPv6 literal: %q", hostport)
		}
		host, port = h[:end+1], strings.TrimPrefix(h[end+1:], ":")
	} else if i := strings.LastIndexByte(h, ':'); i >= 0 {
		host, port = h[:i], h[i+1:]
	}
	if host == "" {
		return "", fmt.Errorf("empty host in %q", hostport)
	}
	if port == "" {
		switch scheme {
		case "http", "ws":
			port = "80"
		case "https", "wss":
			port = "443"
		default:
			return "", fmt.Errorf("no default port for scheme %q", scheme)
		}
	}
	return host + ":" + port, nil
}
