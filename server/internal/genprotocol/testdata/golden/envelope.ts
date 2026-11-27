// GENERATED — 手改无效，跑 go run ./cmd/gen-protocol -out <dir>
// ACK 信封约定（与 ws.ACK 对应）：成功 data 携带结果、失败走 error 通道。
// emitAck 成功路径 resolve 的是 data 本身（非信封）。
export interface AckFrame<T = unknown> {
	id?: string;
	event: string;
	data?: T;
	error?: { code: number; message: string };
}

// AckPayload<T>：emitAck 成功路径 resolve 的 data 类型。
// 服务端没有声明 ACK 结果类型：ws.ACK.Data 是 interface{}，handler 直接返回
// raw JSON 字符串，Catalog 也只用 Payload 描述请求体。因此 ACK data 无法从这里
// 推导，AckResult 恒为 unknown —— 不要用 c2s 请求 payload 冒充 ACK 类型。
// 调用方需要具体类型时，在取用处自行标注（如本地泛型包装 emitAck<T> 或类型断言）。
export type AckPayload<T> = T;

export type AckResult = AckPayload<unknown>;
