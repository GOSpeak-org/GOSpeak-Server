// Package oauth 提供 OAuth2 第三方登录协议实现（GitHub / Google / QQ）。
// 通过 Provider 接口抽象统一的认证流程：构造授权 URL → 兑换 access_token → 获取用户信息。
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// ProviderConfig 第三方 OAuth 应用的配置项，根据平台填写对应值。
type ProviderConfig struct {
	ClientID     string       // OAuth App 的 Client ID
	ClientSecret string       // OAuth App 的 Client Secret
	AuthURL      string       // 用户授权页面 URL
	TokenURL     string       // 用 code 兑换 access_token 的端点
	UserInfoURL  string       // 获取用户信息的 API 端点
	RedirectURL  string       // 授权后回调地址
	Scopes       string       // 请求的权限范围，多个用空格分隔
	FieldMapping FieldMapping // 自建 OAuth 的用户信息字段映射（仅 GenericProvider 使用）
}

// toOAuth2Config 将 ProviderConfig 转为 golang.org/x/oauth2.Config。
func (c *ProviderConfig) toOAuth2Config() *oauth2.Config {
	var scopes []string
	if c.Scopes != "" {
		scopes = strings.Fields(c.Scopes)
	}
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURL,
			TokenURL: c.TokenURL,
		},
		RedirectURL: c.RedirectURL,
		Scopes:      scopes,
	}
}

// UserInfo 统一用户信息模型，屏蔽各平台字段差异。
type UserInfo struct {
	Provider    string // 平台名（github / google / qq）
	ProviderUID string // 平台侧用户唯一标识（github=id, google=sub, qq=openid）
	Username    string // 昵称
	Avatar      string // 头像 URL
	Email       string // 邮箱（部分平台可能为空）
}

// Provider 定义 OAuth2 认证流程的三步接口。
type Provider interface {
	// GetAuthURL 构造授权页面 URL，附带 S256 PKCE challenge。
	GetAuthURL(state, codeChallenge string) string
	// ExchangeToken 用 code + PKCE verifier 兑换 access_token / refresh_token。
	ExchangeToken(code, codeVerifier string) (accessToken, refreshToken string, err error)
	// GetUserInfo 用 access_token 获取用户基本信息。
	GetUserInfo(accessToken string) (*UserInfo, error)
}

// GeneratePKCE 生成一对 PKCE verifier / S256 challenge。
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// NewProvider 根据平台名创建对应的 Provider 实例。不支持的平台返回 nil。
func NewProvider(name string, cfg *ProviderConfig) Provider {
	switch name {
	case "github":
		return &GitHubProvider{cfg: cfg}
	case "google":
		return &GoogleProvider{cfg: cfg}
	case "qq":
		return &QQProvider{cfg: cfg}
	default:
		// 自建/自定义 OAuth 提供商，使用通用实现
		if cfg.AuthURL != "" && cfg.TokenURL != "" && cfg.UserInfoURL != "" {
			return &GenericProvider{cfg: cfg, name: name, fields: cfg.FieldMapping}
		}
		return nil
	}
}

const (
	oauthHTTPTimeout = 10 * time.Second
	maxOAuthBody     = 2 << 20 // 2 MiB
)

var oauthHTTPClient = &http.Client{
	Timeout: oauthHTTPTimeout,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           safeOAuthDialContext,
		TLSHandshakeTimeout:   oauthHTTPTimeout,
		ResponseHeaderTimeout: oauthHTTPTimeout,
	},
}

// exchangeOAuthCode 兑换 token。附带 PKCE code_verifier；部分老 provider 忽略该参数。
// oauth2 默认忽略 http.Client 超时，必须显式传入带 deadline 的 context。
func exchangeOAuthCode(cfg *ProviderConfig, code, codeVerifier string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oauthHTTPTimeout)
	defer cancel()
	// oauth2 默认用 http.DefaultClient；注入带 SSRF 防护的 client 并保留 deadline。
	ctx = context.WithValue(ctx, oauth2.HTTPClient, oauthHTTPClient)
	return cfg.toOAuth2Config().Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
}

func authCodeURL(cfg *ProviderConfig, state, codeChallenge string, extra ...oauth2.AuthCodeOption) string {
	opts := make([]oauth2.AuthCodeOption, 0, len(extra)+2)
	opts = append(opts,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	opts = append(opts, extra...)
	return cfg.toOAuth2Config().AuthCodeURL(state, opts...)
}

// isLoopbackHost 判断 host 是否本地回环（开发环境的自建 OAuth 允许明文 http）。
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// blockedIP 拒绝云元数据/链路本地等 SSRF 高价值目标。
// 保留 RFC1918 私网访问：自建 OAuth（内网 GitLab 等）是受支持的部署方式。
func blockedIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// 阿里云元数据服务使用 100.100.100.200，落在 CGNAT 段。
	if ip.Equal(net.IPv4(100, 100, 100, 200)) {
		return true
	}
	// AWS IMDSv6 使用 ULA 地址，不属于链路本地但同样敏感。
	return ip.Equal(net.ParseIP("fd00:ec2::254"))
}

// safeOAuthDialContext 在建立连接前校验解析出的目标 IP，覆盖 DNS rebinding 与重定向。
func safeOAuthDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("oauth: invalid address %q", addr)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("oauth: no address for host %q", host)
	}
	for _, resolved := range ips {
		if blockedIP(resolved.IP) {
			return nil, fmt.Errorf("oauth: blocked destination %s", resolved.IP)
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// safeOAuthURL 限制出站地址：https 必填（本地回环可用 http），且 host 不得为空。
func safeOAuthURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("oauth: invalid url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("oauth: empty host")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", fmt.Errorf("oauth: http is only allowed for loopback hosts")
		}
	default:
		return "", fmt.Errorf("oauth: unsupported scheme %q", u.Scheme)
	}
	return u.String(), nil
}

func httpDo(req *http.Request) ([]byte, error) {
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		// 上游 *url.Error 带完整请求 URL；QQ 的 userinfo URL 含 access_token，
		// 必须在错误离开本包前脱敏，避免任何上层把它写进响应或日志。
		return nil, redactURLError(err)
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxOAuthBody+1))
}

// httpPostForm 通用 POST application/x-www-form-urlencoded 请求，限制响应体大小。
func httpPostForm(rawURL string, data url.Values) ([]byte, error) {
	safe, err := safeOAuthURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, safe, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, err := httpDo(req)
	if err != nil {
		return nil, err
	}
	if len(body) > maxOAuthBody {
		return nil, fmt.Errorf("oauth: response body too large")
	}
	return body, nil
}

// httpGet 通用 GET 请求，支持 Bearer Token 鉴权，限制响应体大小。
func httpGet(rawURL, token string) ([]byte, error) {
	safe, err := safeOAuthURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, safe, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	body, err := httpDo(req)
	if err != nil {
		return nil, err
	}
	if len(body) > maxOAuthBody {
		return nil, fmt.Errorf("oauth: response body too large")
	}
	return body, nil
}

// GitHubProvider GitHub OAuth2 登录实现。
// 响应格式：URL 编码键值对（token 端点），JSON（userinfo 端点）。
type GitHubProvider struct {
	cfg *ProviderConfig
}

func (g *GitHubProvider) GetAuthURL(state, codeChallenge string) string {
	return authCodeURL(g.cfg, state, codeChallenge)
}

func (g *GitHubProvider) ExchangeToken(code, codeVerifier string) (string, string, error) {
	token, err := exchangeOAuthCode(g.cfg, code, codeVerifier)
	if err != nil {
		return "", "", err
	}
	return token.AccessToken, token.RefreshToken, nil
}

func (g *GitHubProvider) GetUserInfo(accessToken string) (*UserInfo, error) {
	body, err := httpGet(g.cfg.UserInfoURL, accessToken)
	if err != nil {
		return nil, err
	}
	var data struct {
		ID     int    `json:"id"`
		Login  string `json:"login"`
		Email  string `json:"email"`
		Avatar string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &UserInfo{
		Provider:    "github",
		ProviderUID: fmt.Sprintf("%d", data.ID),
		Username:    data.Login,
		Avatar:      data.Avatar,
		Email:       data.Email,
	}, nil
}

// GoogleProvider Google OAuth2 登录实现。
// 响应格式：JSON（token 端点和 userinfo 端点）。
// 注意请求中额外携带 response_type=code 和 access_type=offline 参数。
type GoogleProvider struct {
	cfg *ProviderConfig
}

func (gp *GoogleProvider) GetAuthURL(state, codeChallenge string) string {
	return authCodeURL(gp.cfg, state, codeChallenge, oauth2.AccessTypeOffline)
}

func (gp *GoogleProvider) ExchangeToken(code, codeVerifier string) (string, string, error) {
	token, err := exchangeOAuthCode(gp.cfg, code, codeVerifier)
	if err != nil {
		return "", "", err
	}
	return token.AccessToken, token.RefreshToken, nil
}

func (gp *GoogleProvider) GetUserInfo(accessToken string) (*UserInfo, error) {
	body, err := httpGet(gp.cfg.UserInfoURL, accessToken)
	if err != nil {
		return nil, err
	}
	var data struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &UserInfo{
		Provider:    "google",
		ProviderUID: data.Sub,
		Username:    data.Name,
		Avatar:      data.Picture,
		Email:       data.Email,
	}, nil
}

// QQProvider QQ OAuth2 登录实现。
// QQ 的 token 端点同时支持 JSON 和 URL 编码响应（通过 fmt=json 参数控制）。
// userinfo 端点需将 access_token 以 query 参数形式传递。
type QQProvider struct {
	cfg *ProviderConfig
}

func (q *QQProvider) GetAuthURL(state, codeChallenge string) string {
	return authCodeURL(q.cfg, state, codeChallenge)
}

func (q *QQProvider) ExchangeToken(code, codeVerifier string) (string, string, error) {
	token, err := exchangeOAuthCode(q.cfg, code, codeVerifier)
	if err != nil {
		return "", "", err
	}
	return token.AccessToken, token.RefreshToken, nil
}

func (q *QQProvider) GetUserInfo(accessToken string) (*UserInfo, error) {
	params := url.Values{}
	params.Set("access_token", accessToken)
	params.Set("fmt", "json")

	body, err := httpGet(q.cfg.UserInfoURL+"?"+params.Encode(), "")
	if err != nil {
		return nil, err
	}
	var data struct {
		Ret      int    `json:"ret"`
		OpenID   string `json:"openid"`
		Nickname string `json:"nickname"`
		Avatar   string `json:"figureurl_qq_2"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Ret != 0 {
		return nil, fmt.Errorf("qq: get user info failed, ret=%d", data.Ret)
	}
	return &UserInfo{
		Provider:    "qq",
		ProviderUID: data.OpenID,
		Username:    data.Nickname,
		Avatar:      data.Avatar,
	}, nil
}

// GetDefaultConfig 返回各平台的预设端点配置和默认 scope。
// ClientID / ClientSecret / RedirectURL 需业务方自行注入。
func GetDefaultConfig(name string) *ProviderConfig {
	switch name {
	case "github":
		return &ProviderConfig{
			AuthURL:     "https://github.com/login/oauth/authorize",
			TokenURL:    "https://github.com/login/oauth/access_token",
			UserInfoURL: "https://api.github.com/user",
			Scopes:      "read:user user:email",
		}
	case "google":
		return &ProviderConfig{
			AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:    "https://oauth2.googleapis.com/token",
			UserInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
			Scopes:      "openid email profile",
		}
	case "qq":
		return &ProviderConfig{
			AuthURL:     "https://graph.qq.com/oauth2.0/authorize",
			TokenURL:    "https://graph.qq.com/oauth2.0/token",
			UserInfoURL: "https://graph.qq.com/user/get_user_info",
			Scopes:      "get_user_info",
		}
	default:
		return nil
	}
}

// FormatUsername 将第三方昵称转为统一小写+下划线格式的用户名，用于自动生成系统用户名。
func FormatUsername(base string) string {
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, " ", "_")
	return base
}
