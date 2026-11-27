package plugin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const proxyCallTimeout = 10 * time.Second

type browserRequest struct {
	PluginID  string `json:"plugin_id"`
	RoutePath string `json:"route_path"`
	Kind      string `json:"kind"`
	Payload   any    `json:"payload"`
}

type BrowserProxy struct {
	pages   *PageProxy
	current func(*gin.Context) string
	host    string
	client  *http.Client
}

func NewBrowserProxy(pages *PageProxy, current func(*gin.Context) string) *BrowserProxy {
	return &BrowserProxy{
		pages:   pages,
		current: current,
		client:  &http.Client{Timeout: proxyCallTimeout},
	}
}

func (b *BrowserProxy) SetAllowedHost(host string) {
	b.host = strings.TrimSpace(host)
}

func (b *BrowserProxy) userUUID(c *gin.Context) string {
	if b.current == nil {
		return ""
	}
	return b.current(c)
}

func (b *BrowserProxy) auth(c *gin.Context) {
	if b.userUUID(c) == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func (b *BrowserProxy) Call(c *gin.Context) {
	b.forward(c, "/__gospeak/browser/call")
}

func (b *BrowserProxy) Refresh(c *gin.Context) {
	b.forward(c, "/__gospeak/browser/refresh")
}

func (b *BrowserProxy) forward(c *gin.Context, suffix string) {
	b.auth(c)
	if c.IsAborted() {
		return
	}
	if !b.originAllowed(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var req browserRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "msg": "invalid request"})
		return
	}
	page, ok := b.pages.Lookup(req.PluginID)
	if !ok || !strings.HasPrefix(req.RoutePath, page.Prefix) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	target, err := url.Parse(page.BaseURL)
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	target.Path = strings.TrimRight(target.Path, "/") + suffix
	body, err := json.Marshal(req)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	upstream, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("X-Gospeak-Plugin-ID", req.PluginID)
	upstream.Header.Set("X-Gospeak-User-UUID", b.userUUID(c))
	resp, err := b.client.Do(upstream)
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

func (b *BrowserProxy) originAllowed(c *gin.Context) bool {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	if strings.EqualFold(u.Host, c.Request.Host) {
		return true
	}
	if b.host != "" && strings.EqualFold(u.Host, b.host) {
		return true
	}
	return false
}
