# test — API 集成测试

Vitest 集成测试套件（`@gospeak/test`）：**发真实 HTTP/WS 请求**，自己拉起临时 server。

## 结构

```
auth/ user/ role/ domain/ room/ mute/ message/ conversation/
invite/ announcement/ email/ oauth/ sfu/ signal/ storage/ system/
room_layout/ domain_layout/
smoke.test.ts
helpers/{api.ts, ws.ts}
globalSetup.ts          # 起停临时 server
vitest.config.ts        # fileParallelism:false, hookTimeout 120s, testTimeout 30s
```

## 运行

```bash
pnpm test:server        # = cd test && vitest run（自动拉起临时 server）
```

`globalSetup.ts` 会 `go build` 后端、挑空闲端口启动、写 `test/.runtime/server.json`，运行结束清理。它按 Go 源码 mtime 决定是否复用已有构建，**不要手动再起一个 server**。

## 约定

- 测试用 `helpers/api.ts` 的封装发请求，断言 `status` / `code` / `msg` / `data` 四段，不要把响应当字符串比。
- WS 测试用 `helpers/ws.ts`（ACK 关联、事件订阅），不要裸 new WebSocket 复制重连逻辑。
- 每个用例自建独立用户/域/房间，不依赖执行顺序（`fileParallelism: false` 只是为了避免端口与资源竞争，不是给你写顺序依赖的许可）。
- 新增模块建 `test/<module>/<module>.test.ts`；集成测试才放这里，纯函数单测放源码同目录。
- 断言失败要给出足够上下文（打印 code/msg），否则排查成本极高。
