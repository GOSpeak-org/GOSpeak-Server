package plugin

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"GOSpeak/internal/plugin/proxyrules"
)

const pluginRouteRoot = "/plugins/"

type mountedPage struct {
	PluginID string `json:"plugin_id"`
	Prefix   string `json:"route_path"`
	Mode     string `json:"mode"`
	BaseURL  string `json:"entry_url"`
}

type PageProxy struct {
	mu            sync.RWMutex
	pages         map[string]mountedPage
	fallback      gin.HandlerFunc
	allowLoopback bool
}

func NewPageProxy() *PageProxy {
	return &PageProxy{pages: make(map[string]mountedPage)}
}

func (p *PageProxy) SetAllowLoopbackForTest(allow bool) {
	p.mu.Lock()
	p.allowLoopback = allow
	p.mu.Unlock()
}

func (p *PageProxy) Confirm(pluginID, mode, routePath, entryURL string) error {
	if pluginID == "" || strings.ContainsAny(pluginID, "/\\") {
		return errors.New("invalid plugin id")
	}
	if mode != "spa" && mode != "iframe" {
		return errors.New("unsupported page mode")
	}
	want := pluginRouteRoot + pluginID + "/"
	if routePath != want {
		return fmt.Errorf("route prefix must be %s", want)
	}
	if err := proxyrules.ValidateBaseURL(entryURL); err != nil {
		return err
	}
	p.mu.RLock()
	allowLoopback := p.allowLoopback
	p.mu.RUnlock()
	if !allowLoopback {
		if err := rejectPrivateEntryURL(entryURL); err != nil {
			return err
		}
	}
	base := proxyrules.NormalizeBaseURL(entryURL)
	if base == "" {
		return errors.New("entry url is required")
	}
	p.mu.Lock()
	p.pages[pluginID] = mountedPage{PluginID: pluginID, Prefix: want, Mode: mode, BaseURL: base}
	p.mu.Unlock()
	return nil
}

func (p *PageProxy) Lookup(pluginID string) (mountedPage, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	page, ok := p.pages[pluginID]
	return page, ok
}

func (p *PageProxy) Pages(c *gin.Context) {
	p.mu.RLock()
	pages := make([]mountedPage, 0, len(p.pages))
	for _, page := range p.pages {
		pages = append(pages, page)
	}
	p.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "data": pages})
}

func (p *PageProxy) SetFallback(handler gin.HandlerFunc) {
	p.fallback = handler
}

func (p *PageProxy) Register(r *gin.Engine) {
	r.Any(pluginRouteRoot+"*path", p.handle)
}

func (p *PageProxy) handle(c *gin.Context) {
	path := c.Request.URL.Path
	if strings.Contains(path, "..") {
		p.serveFallback(c)
		return
	}
	pluginID, rest := splitPluginPath(path)
	if pluginID == "" {
		p.serveFallback(c)
		return
	}
	page, ok := p.Lookup(pluginID)
	if !ok {
		p.serveFallback(c)
		return
	}
	target, err := url.Parse(page.BaseURL)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	upstream := httputil.NewSingleHostReverseProxy(target)
	upstream.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningPath(target.Path, rest)
		req.Host = target.Host
		req.Header.Set("X-Gospeak-Plugin-ID", pluginID)
	}
	upstream.ModifyResponse = func(resp *http.Response) error {
		if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
			resp.Header.Del("Set-Cookie")
			for _, cookie := range cookies {
				resp.Header.Add("Set-Cookie", proxyrules.RewriteSetCookie(cookie))
			}
		}
		resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self'")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		return nil
	}
	upstream.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "plugin page unavailable", http.StatusBadGateway)
	}
	upstream.ServeHTTP(closeNotifyWriter{ResponseWriter: c.Writer}, c.Request)
}

type closeNotifyWriter struct {
	http.ResponseWriter
}

func (closeNotifyWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}

func (p *PageProxy) serveFallback(c *gin.Context) {
	if p.fallback != nil {
		p.fallback(c)
		c.Abort()
		return
	}
	c.AbortWithStatus(http.StatusNotFound)
}

func rejectPrivateEntryURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("invalid entry url")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("entry url host %s is not publicly routable", host)
		}
		return nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("entry url host %s is not publicly routable", host)
	}
	return nil
}

func splitPluginPath(path string) (string, string) {
	if !strings.HasPrefix(path, pluginRouteRoot) {
		return "", ""
	}
	rest := strings.TrimPrefix(path, pluginRouteRoot)
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return rest, "/"
	}
	return rest[:i], "/" + rest[i+1:]
}

func singleJoiningPath(prefix, rest string) string {
	if prefix == "" || prefix == "/" {
		return rest
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(rest, "/")
}
