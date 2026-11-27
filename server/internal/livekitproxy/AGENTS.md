# livekitproxy 模块

把浏览器的 LiveKit 信令请求同源反代到 `LIVEKIT_HOST`，避免暴露容器内地址与跨域。

## API

- `NewHandler(resolve Resolver) http.Handler`：`Resolver` 返回当前 LiveKit 地址。
- `Cached(resolve, ttl) Resolver`：带 TTL 缓存，避免每请求解析配置。
- `ServerURLForRequest(r)`：推导下发给浏览器的同源地址（`/livekit`）。
- `PathPrefix = "/livekit"`。

## 安全模型（重要）

`allowedPrefixes` **只放行 `/livekit/rtc/`**，其余路径（admin twirp、`/debug/pprof`、`/whip` 等）一律 404 —— 防止把 LiveKit 管理面顺带暴露到公网。**放宽这个白名单等于开放管理 API**。

## 行为

- `parseTarget` 把 `ws/http` 归一为 `http`、`wss/https` 归一为 `https`；非法或空 → 502。
- 转发时剥掉 `/livekit` 前缀；WebSocket 升级交给 `httputil.ReverseProxy` 处理。
- `ServerURLForRequest` 优先 `X-Forwarded-Host` / `X-Forwarded-Proto`（反代或桌面回环代理改写 Host 时仍能拿到用户实际地址）。

## 约束

- 不要把 `LIVEKIT_HOST`（容器内地址）直接下发给浏览器；浏览器用 `LIVEKIT_PUBLIC_URL` 或同源 `/livekit`。
- 新增可转发路径必须评估暴露面，并补 `proxy_test.go` 用例。
