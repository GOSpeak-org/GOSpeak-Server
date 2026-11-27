package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/pkg/oauth"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

const oauthStateCookie = "gospeak_oauth_state"

// oauthFlowState 是一次 OAuth 往返的服务端私有状态，base64(JSON) 存于 HttpOnly cookie。
// verifier 用于 PKCE，action=bind 时 callback 走「关联已有账号」而非登录。
type oauthFlowState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Action   string `json:"a,omitempty"`
}

func encodeOAuthFlow(s oauthFlowState) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeOAuthFlow(value string) (*oauthFlowState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var s oauthFlowState
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.State == "" || s.Verifier == "" {
		return nil, errors.New("incomplete oauth flow state")
	}
	return &s, nil
}

type OAuthHandler struct {
	oauthService *service.OAuthService
	cookie       *AuthCookieConfig
}

func NewOAuthHandler(oauthService *service.OAuthService, cookie *AuthCookieConfig) *OAuthHandler {
	if cookie == nil {
		cookie = defaultAuthCookieConfig()
	}
	return &OAuthHandler{oauthService: oauthService, cookie: cookie}
}

func (h *OAuthHandler) Login(c *gin.Context) {
	provider := c.Param("provider")
	if provider == "" {
		redirectOAuthError(c, "provider is required")
		return
	}

	// 服务端生成高熵 state + PKCE verifier，写入 HttpOnly cookie，callback 强制校验。
	// 不信任客户端传入的 state：统一服务端生成，避免预置固定 state。
	state, err := randomState(16)
	if err != nil {
		redirectOAuthError(c, "failed to generate oauth state")
		return
	}
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		redirectOAuthError(c, "failed to generate oauth verifier")
		return
	}
	if err := h.setFlowState(c, oauthFlowState{State: state, Verifier: verifier}); err != nil {
		redirectOAuthError(c, "failed to persist oauth state")
		return
	}

	authURL, err := h.oauthService.GetAuthURL(provider, state, challenge)
	if err != nil {
		// 浏览器从登录页跳入，错误统一回登录页展示，避免卡在 JSON 页。
		redirectOAuthError(c, oauthErrorMessage(err, "oauth provider unavailable"))
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

func (h *OAuthHandler) setFlowState(c *gin.Context, s oauthFlowState) error {
	value, err := encodeOAuthFlow(s)
	if err != nil {
		return err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cookie.secure(c.Request),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Bind 发起「关联已有账号」流程：要求已登录，回调走 LinkAccount 而非建号/登录。
func (h *OAuthHandler) Bind(c *gin.Context) {
	provider := c.Param("provider")
	if provider == "" || middleware.CurrentUser(c) == nil {
		redirectOAuthError(c, "login required")
		return
	}

	state, err := randomState(16)
	if err != nil || state == "" {
		redirectOAuthError(c, "failed to generate oauth state")
		return
	}
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		redirectOAuthError(c, "failed to generate oauth verifier")
		return
	}
	if err := h.setFlowState(c, oauthFlowState{State: state, Verifier: verifier, Action: "bind"}); err != nil {
		redirectOAuthError(c, "failed to persist oauth state")
		return
	}
	authURL, err := h.oauthService.GetAuthURL(provider, state, challenge)
	if err != nil {
		redirectOAuthError(c, oauthErrorMessage(err, "oauth provider unavailable"))
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

func (h *OAuthHandler) Callback(c *gin.Context) {
	provider := c.Param("provider")
	code := c.Query("code")
	if code == "" {
		redirectOAuthError(c, "code is required")
		return
	}

	state := c.Query("state")
	cookie, err := c.Request.Cookie(oauthStateCookie)
	if err != nil || cookie.Value == "" || state == "" {
		redirectOAuthError(c, "invalid oauth state")
		return
	}
	flow, err := decodeOAuthFlow(cookie.Value)
	if err != nil || flow.State != state {
		redirectOAuthError(c, "invalid oauth state")
		return
	}
	// 一次性 state
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookie.secure(c.Request),
		SameSite: http.SameSiteLaxMode,
	})

	// 授权方错误回传（用户拒绝等）
	if errDesc := c.Query("error"); errDesc != "" {
		msg := errDesc
		if d := c.Query("error_description"); d != "" {
			msg = d
		}
		redirectOAuthError(c, msg)
		return
	}

	action := "login"
	if flow.Action == "bind" {
		claims := middleware.CurrentUser(c)
		if claims == nil {
			// callback 是公开路由：bind 流程用 access cookie 恢复身份后再校验。
			if authCookie, err := c.Request.Cookie(h.cookie.AccessName); err == nil {
				claims, _ = middleware.VerifyToken(authCookie.Value)
			}
		}
		if claims == nil || claims.UserUUID == "" {
			redirectOAuthError(c, "login required")
			return
		}
		if err := h.oauthService.LinkAccount(claims.UserUUID, provider, code, flow.Verifier); err != nil {
			redirectOAuthError(c, oauthErrorMessage(err, "oauth link failed"))
			return
		}
		action = "bind"
	} else {
		resp, err := h.oauthService.HandleCallback(provider, code, flow.Verifier)
		if err != nil {
			redirectOAuthError(c, oauthErrorMessage(err, "oauth login failed"))
			return
		}
		h.cookie.Set(c, resp.Token, resp.RefreshToken)
	}

	// 浏览器回调：token 已写入 HttpOnly Cookie，同源 postMessage 只通知 opener 成功，
	// 避免 token 出现在 URL fragment、浏览器历史、代理日志或跨窗口消息中。
	payload, _ := oauthSuccessPayload(action)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'")
	_, _ = c.Writer.Write([]byte(oauthBridgeHTML(payload)))
}

// oauthSuccessPayload 通知 opener 登录成功并附带 access token 有效期（秒）。
func oauthSuccessPayload(action string) ([]byte, error) {
	return json.Marshal(map[string]any{"ok": true, "action": action, "expires_in": pkg.AccessTokenExpiresIn()})
}

// oauthBridgeHTML 返回 OAuth 回调落地页：同源窗口通过 postMessage 单次交付 token。
func oauthBridgeHTML(payload []byte) string {
	return `<!doctype html>
<html lang="zh-CN">
<meta charset="utf-8">
<title>GOSpeak 登录</title>
<body>
<script>
(function () {
  var data = ` + string(payload) + `;
  if (window.opener && window.opener !== window) {
    try {
      window.opener.postMessage({type: "gospeak-oauth", ok: data.ok === true, action: data.action}, window.location.origin);
    } catch (e) {}
    window.close();
    return;
  }
  document.body.textContent = "登录成功，请关闭本窗口返回 GOSpeak。";
})();
</script>
</body>
</html>`
}

// maxOAuthRedirectMessage 限制回显到 /login?oauth_error= 的文案长度，
// 避免上游超长错误描述进入浏览器历史与反代日志。
const maxOAuthRedirectMessage = 200

// cleanRedirectMessage 剔除控制字符并按 rune 截断（不切断 UTF-8 多字节字符）。
func cleanRedirectMessage(msg string) string {
	msg = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, msg)
	runes := []rune(msg)
	if len(runes) > maxOAuthRedirectMessage {
		runes = runes[:maxOAuthRedirectMessage]
	}
	return string(runes)
}

func redirectOAuthError(c *gin.Context, msg string) {
	q := make(url.Values)
	q.Set("oauth_error", cleanRedirectMessage(msg))
	c.Redirect(http.StatusFound, "/login?"+q.Encode())
}

// oauthErrorMessage 把服务层错误映射为**固定**的重定向文案。
//
// 绝不回传 appErr.Message：OAUTH_GET_USER_FAILED 的原始错误来自 Go 的
// *url.Error，其 Error() 会带上完整请求 URL；QQ 等 provider 把 access_token
// 放在 userinfo URL 的 query 里。OAuth 走 302，任何原文都会写进 Location，
// 进而进入浏览器历史、反代日志与前端错误展示，绕过 pkg.HandleError 的脱敏。
// 这里复用 pkg.ClientError 的错误分类，但只取错误码对应的默认安全文案。
func oauthErrorMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	code, _ := pkg.ClientError(err)
	switch code {
	case pkg.OAUTH_PROVIDER_NOT_FOUND,
		pkg.OAUTH_PROVIDER_DISABLED,
		pkg.OAUTH_TOKEN_EXCHANGE_FAILED,
		pkg.OAUTH_GET_USER_FAILED:
		return pkg.GetErrMsg(code)
	default:
		return fallback
	}
}

func randomState(n int) (string, error) {
	b := make([]byte, n)
	for i := 0; i < 3; i++ {
		if _, err := rand.Read(b); err == nil {
			return hex.EncodeToString(b), nil
		}
	}
	return "", errors.New("oauth state: crypto/rand unavailable")
}

func (h *OAuthHandler) ListEnabledProviders(c *gin.Context) {
	providers, err := h.oauthService.ListEnabledProviders()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, providers)
}

// ListLinkedAccounts 返回当前用户已绑定的第三方账号。
func (h *OAuthHandler) ListLinkedAccounts(c *gin.Context) {
	claims := middleware.CurrentUser(c)
	if claims == nil {
		pkg.Fail(c, pkg.TOKEN_NOT_EXIST)
		return
	}
	accounts, err := h.oauthService.ListLinkedAccounts(claims.UserUUID)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, accounts)
}

// UnlinkAccount 解除当前用户在指定 provider 上的绑定。
func (h *OAuthHandler) UnlinkAccount(c *gin.Context) {
	claims := middleware.CurrentUser(c)
	if claims == nil {
		pkg.Fail(c, pkg.TOKEN_NOT_EXIST)
		return
	}
	if err := h.oauthService.UnlinkAccount(claims.UserUUID, c.Param("provider")); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, nil)
}

func (h *OAuthHandler) ListProviders(c *gin.Context) {
	providers, err := h.oauthService.ListProviders()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, providers)
}

func (h *OAuthHandler) CreateProvider(c *gin.Context) {
	var req service.CreateOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if req.Name == "" {
		pkg.Fail(c, pkg.INVALID_PARAMS, "name is required")
		return
	}
	provider, err := h.oauthService.CreateProviderFromDTO(&req)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, provider)
}

func (h *OAuthHandler) UpdateProvider(c *gin.Context) {
	var req service.UpdateOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	provider, err := h.oauthService.UpdateProviderFromDTO(&req)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, provider)
}

func (h *OAuthHandler) DeleteProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, "invalid id")
		return
	}
	if err := h.oauthService.DeleteProvider(uint(id)); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, nil)
}
