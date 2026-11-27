package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// 统一安全响应头。CSP 采用最小可用策略：
// - script-src 'self'：页面只有外链 module script，无内联脚本；
// - style-src 允许 unsafe-inline：前端有 30+ 处动态 style（虚拟列表/音量条等）；
// - connect-src 覆盖同源 /ws /livekit 及绝对 wss 地址；
// - img-src 额外放行 blob: 与 https:：头像裁切/自定义背景走 blob URL，Markdown 走外链图；
// - worker-src 放行 blob:：AudioWorklet 发言检测以 blob URL 加载 worklet 模块；
// - frame-ancestors 'none' 替代老式 X-Frame-Options。
const cspPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' blob: data: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self' wss: ws: blob: https: http:; " +
	"worker-src 'self' blob:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'"

// swaggerCspPolicy Swagger UI 内部用 Function/eval 做 schema 校验，单独放宽 script-src。
const swaggerCspPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' blob: data: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self' wss: ws: blob: https: http:; " +
	"worker-src 'self' blob:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'"

// SecurityHeaders 为所有响应设置统一安全头。
// OAuth 回调桥接页有自己的内联 script CSP（handler 内 c.Header 为 Set 语义，
// 后于本中间件执行会覆盖这里的值），不受影响。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		policy := cspPolicy
		if strings.HasPrefix(c.Request.URL.Path, "/swagger") {
			policy = swaggerCspPolicy
		}
		c.Header("Content-Security-Policy", policy)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
