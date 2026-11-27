package proxyrules

import "testing"

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty clears the setting", "", false},
		{"blank clears the setting", "   ", false},
		{"https absolute", "https://gospeak.example.com", false},
		{"http absolute", "http://gospeak.example.com", false},
		{"explicit port and path", "https://gospeak.example.com:8443/gospeak", false},
		{"loopback with port", "http://127.0.0.1:8998", false},
		{"ipv6 loopback", "http://[::1]:8998", false},
		{"uppercase scheme", "HTTPS://GOSPEAK.EXAMPLE.COM", false},
		{"surrounding whitespace", "  https://gospeak.example.com  ", false},
		{"no scheme", "gospeak.example.com", true},
		{"scheme-like host and port", "gospeak.example.com:8998", true},
		{"relative path", "/gospeak", true},
		{"dot relative path", "../gospeak", true},
		{"empty host", "http://", true},
		{"ftp scheme", "ftp://gospeak.example.com", true},
		{"websocket scheme", "ws://gospeak.example.com", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"file scheme", "file:///etc/passwd", true},
		{"unparsable ipv6", "http://[::1", true},
		{"invalid escape", "http://%zz", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURL(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, want an error", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateBaseURL(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

func TestValidateBaseURLErrorMessages(t *testing.T) {
	const wantAbsolute = "invalid server URL: must be an absolute http(s) URL"

	for _, in := range []string{
		"ftp://gospeak.example.com",
		"ws://gospeak.example.com",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"http://",
		"gospeak.example.com",
		"gospeak.example.com:8998",
		"/gospeak",
	} {
		if err := ValidateBaseURL(in); err == nil {
			t.Fatalf("ValidateBaseURL(%q) = nil, want an error", in)
		} else if err.Error() != wantAbsolute {
			t.Errorf("ValidateBaseURL(%q) = %q, want %q", in, err.Error(), wantAbsolute)
		}
	}

	for _, in := range []string{"http://[::1", "http://%zz"} {
		err := ValidateBaseURL(in)
		if err == nil {
			t.Fatalf("ValidateBaseURL(%q) = nil, want an error", in)
		}
		if want := "invalid server URL: "; len(err.Error()) <= len(want) || err.Error()[:len(want)] != want {
			t.Errorf("ValidateBaseURL(%q) = %q, want prefix %q", in, err.Error(), want)
		}
		if got := err.Error(); got == wantAbsolute {
			t.Errorf("ValidateBaseURL(%q) = %q, want a parse error, not the absolute-URL message", in, got)
		}
	}
}

func TestRewriteSetCookie(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"strips domain and secure, normalizes samesite",
			"a=1; Domain=x.com; Secure; HttpOnly; Path=/; SameSite=Strict",
			"a=1; HttpOnly; Path=/; SameSite=Lax",
		},
		{
			"appends samesite when absent",
			"a=1; HttpOnly; Path=/",
			"a=1; HttpOnly; Path=/; SameSite=Lax",
		},
		{
			"uppercase attributes are matched case-insensitively",
			"DOMAIN=X.COM; SECURE; a=1; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"mixed case attributes are stripped and samesite is appended",
			"DoMaIn=x.com; SeCuRe; a=1; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"single pair gets samesite",
			"a=1",
			"a=1; SameSite=Lax",
		},
		{
			"empty stays empty",
			"",
			"",
		},
		{
			"whitespace only stays empty",
			"   ",
			"",
		},
		{
			"bare domain and bare secure are dropped",
			"a=1; Domain; Secure; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"secure with a value is dropped",
			"a=1; Secure=true; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"samesite none is forced to lax",
			"a=1; SameSite=None; Path=/",
			"a=1; SameSite=Lax; Path=/",
		},
		{
			"samesite lowercase is normalized",
			"a=1; samesite=strict",
			"a=1; SameSite=Lax",
		},
		{
			"only samesite present",
			"SameSite=None",
			"SameSite=Lax",
		},
		{
			"duplicate samesite keeps a single lax",
			"a=1; SameSite=None; SameSite=Strict",
			"a=1; SameSite=Lax",
		},
		{
			"empty segments are dropped",
			"a=1;; ; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"unknown attributes and order are preserved",
			"a=1; Path=/; Partitioned; HttpOnly",
			"a=1; Path=/; Partitioned; HttpOnly; SameSite=Lax",
		},
		{
			"value containing an equals sign is kept",
			"token=abc=def; Secure; Path=/",
			"token=abc=def; Path=/; SameSite=Lax",
		},
		{
			"domain-like value is kept",
			"a=1; subdomain=x.com",
			"a=1; subdomain=x.com; SameSite=Lax",
		},
		{
			"secure prefix without equals on a longer token is kept",
			"a=1; Securely",
			"a=1; Securely; SameSite=Lax",
		},
		{
			"domain with a space before the equals is dropped",
			"a=1; domain =x.com",
			"a=1; SameSite=Lax",
		},
		{
			"secure with a space before the equals is dropped",
			"a=1; secure =1; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"uppercase domain with spaces on both sides is dropped",
			"a=1; DOMAIN = x.com; Path=/",
			"a=1; Path=/; SameSite=Lax",
		},
		{
			"mixed case domain with spaces on both sides is dropped",
			"a=1; Domain = x.com",
			"a=1; SameSite=Lax",
		},
		{
			"samesite with spaces around the equals is normalized",
			"a=1; samesite = none",
			"a=1; SameSite=Lax",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RewriteSetCookie(tc.in); got != tc.want {
				t.Fatalf("RewriteSetCookie(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteSetCookieIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"a=1; Domain=x.com; Secure; HttpOnly; Path=/; SameSite=Strict",
		"a=1",
		"",
		"a=1; SameSite=None; SameSite=Strict",
	} {
		once := RewriteSetCookie(in)
		if twice := RewriteSetCookie(once); twice != once {
			t.Errorf("RewriteSetCookie is not idempotent for %q: %q then %q", in, once, twice)
		}
	}
}

func TestNormalizedHostPort(t *testing.T) {
	cases := []struct {
		scheme, hostport, want string
		ok                     bool
	}{
		{"http", "example.com", "example.com:80", true},
		{"https", "example.com", "example.com:443", true},
		{"ws", "example.com", "example.com:80", true},
		{"wss", "example.com", "example.com:443", true},
		{"http", "Example.COM:8080", "example.com:8080", true},
		{"http", "[::1]:9000", "[::1]:9000", true},
		{"http", "[::1]", "[::1]:80", true},
		{"http", "", "", false},
		{"ftp", "example.com", "", false},
		{"http", "[::1", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizedHostPort(tc.scheme, tc.hostport)
		if ok := err == nil; ok != tc.ok || got != tc.want {
			t.Errorf("NormalizedHostPort(%q, %q) = %q,%v want %q,%v",
				tc.scheme, tc.hostport, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizedHostPortErrors(t *testing.T) {
	for _, tc := range []struct{ scheme, hostport string }{
		{"http", ""},
		{"ftp", "example.com"},
		{"http", "[::1"},
		{"http", "   "},
		{"", "example.com"},
	} {
		got, err := NormalizedHostPort(tc.scheme, tc.hostport)
		if err == nil {
			t.Errorf("NormalizedHostPort(%q, %q) = %q,nil want an error", tc.scheme, tc.hostport, got)
			continue
		}
		if got != "" {
			t.Errorf("NormalizedHostPort(%q, %q) returned %q alongside an error, want \"\"", tc.scheme, tc.hostport, got)
		}
	}
}

func TestIsTrustedWorker(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		baseURL string
		allowed string
		want    bool
	}{
		{
			"target matches baseURL host with the default plain port",
			"ws://backend.example/ws", "http://backend.example", "", true,
		},
		{
			"target matches baseURL host with the default tls port",
			"wss://backend.example/ws", "https://backend.example", "", true,
		},
		{
			"target matches baseURL with explicit matching ports",
			"ws://backend.example:8080/ws", "http://backend.example:8080", "", true,
		},
		{
			"target matches baseURL that carries a path",
			"ws://backend.example/ws", "http://backend.example/gospeak", "", true,
		},
		{
			"target matches an allowed worker host on another host",
			"wss://worker.example:7443/ws", "https://backend.example:8443",
			"worker.example:7443", true,
		},
		{
			"target matches an allowed entry that still needs normalization",
			"wss://worker.example/ws", "https://backend.example",
			"WORKER.EXAMPLE:443", true,
		},
		{
			"target matches the second entry of a comma separated list",
			"wss://worker2.example:7443/ws", "https://backend.example:8443",
			"worker.example:7443,worker2.example:7443", true,
		},
		{
			"spaces and empty segments around entries are trimmed",
			"wss://worker.example:7443/ws", "https://backend.example",
			" , worker.example:7443 ,, ", true,
		},
		{
			"empty allowed string means no extra hosts",
			"wss://worker.example:7443/ws", "https://backend.example", "", false,
		},
		{
			"target is a different host",
			"ws://evil.example/ws", "http://backend.example",
			"worker.example:7443", false,
		},
		{
			"same host on a different port is rejected",
			"ws://backend.example:9999/ws", "http://backend.example:8080",
			"backend.example:8080", false,
		},
		{
			"allowed host on another port does not open this port",
			"ws://worker.example:9999/ws", "http://backend.example",
			"worker.example:7443", false,
		},
		{
			"ftp scheme is rejected",
			"ftp://backend.example/ws", "http://backend.example", "", false,
		},
		{
			"userinfo is rejected",
			"ws://u:p@backend.example/ws", "http://backend.example", "", false,
		},
		{
			"userinfo without a password is rejected",
			"ws://u@backend.example/ws", "http://backend.example", "", false,
		},
		{
			"empty target is rejected",
			"", "http://backend.example", "backend.example:80", false,
		},
		{
			"relative target is rejected",
			"/ws", "http://backend.example", "", false,
		},
		{
			"unparsable target is rejected",
			"ws://[::1", "http://backend.example", "", false,
		},
		{
			"empty baseURL but allowed matches",
			"wss://worker.example:7443/ws", "", "worker.example:7443", true,
		},
		{
			"empty baseURL and empty allowlist rejects",
			"wss://worker.example:7443/ws", "", "", false,
		},
		{
			"host case is equivalent",
			"wss://WORKER.Example.COM/ws", "https://worker.example.com", "", true,
		},
		{
			"scheme case is equivalent",
			"WSS://backend.example/ws", "https://backend.example", "", true,
		},
		{
			"http target matches http baseURL",
			"http://backend.example/ws", "http://backend.example", "", true,
		},
		{
			"plain target does not match tls baseURL port",
			"ws://backend.example/ws", "https://backend.example", "", false,
		},
		{
			"ipv6 target matches ipv6 baseURL",
			"ws://[::1]:8998/ws", "http://[::1]:8998", "", true,
		},
		{
			"ipv6 entry in the allowlist matches",
			"ws://[::1]:9000/ws", "http://backend.example", "[::1]:9000", true,
		},
		{
			"blank allowlist entries are skipped",
			"ws://backend.example/ws", "http://backend.example",
			" ,  ", true,
		},
		{
			"unparsable allowlist entries are skipped",
			"ws://backend.example/ws", "http://backend.example",
			"[::1", true,
		},
		{
			"target with a leading space is rejected",
			" ws://backend.example/ws", "http://backend.example", "", false,
		},
		{
			"target with a leading tab is rejected",
			"\tws://backend.example/ws", "http://backend.example", "", false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTrustedWorker(tc.target, tc.baseURL, tc.allowed); got != tc.want {
				t.Fatalf("IsTrustedWorker(%q, %q, %q) = %v, want %v",
					tc.target, tc.baseURL, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestIsTrustedWorkerRejectsWhenNothingIsConfigured(t *testing.T) {
	for _, target := range []string{
		"ws://backend.example/ws",
		"wss://worker.example:7443/ws",
		"ws://127.0.0.1:8998/ws",
		"ws://169.254.169.254/latest/meta-data",
	} {
		if IsTrustedWorker(target, "", "") {
			t.Errorf("IsTrustedWorker(%q, \"\", \"\") = true, want false", target)
		}
		if IsTrustedWorker(target, "", " , ") {
			t.Errorf("IsTrustedWorker(%q, \"\", \" , \") = true, want false", target)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"blank stays empty", "   ", ""},
		{"no trailing slash", "https://gospeak.example.com", "https://gospeak.example.com"},
		{"one trailing slash", "https://gospeak.example.com/", "https://gospeak.example.com"},
		{"many trailing slashes", "https://gospeak.example.com////", "https://gospeak.example.com"},
		{"surrounding whitespace and slash", "  https://gospeak.example.com/  ", "https://gospeak.example.com"},
		{"explicit port", "https://gospeak.example.com:8443/", "https://gospeak.example.com:8443"},
		{"path prefix loses only its trailing slash", "https://gospeak.example.com/gospeak/", "https://gospeak.example.com/gospeak"},
		{"loopback with port", "http://127.0.0.1:8998/", "http://127.0.0.1:8998"},
		{"ipv6 loopback", "http://[::1]:8998/", "http://[::1]:8998"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeBaseURL(tc.in); got != tc.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeBaseURLKeepsValidationVerdict(t *testing.T) {
	for _, in := range []string{"https://gospeak.example.com/", "  http://127.0.0.1:8998//  "} {
		if err := ValidateBaseURL(NormalizeBaseURL(in)); err != nil {
			t.Fatalf("normalized %q = %q, ValidateBaseURL = %v, want nil", in, NormalizeBaseURL(in), err)
		}
	}
	if err := ValidateBaseURL(NormalizeBaseURL("gospeak.example.com/")); err == nil {
		t.Fatal("normalizing must not turn a schemeless host into a valid URL")
	}
}
