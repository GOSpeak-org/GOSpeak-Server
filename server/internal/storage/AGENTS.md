# storage 模块

对象存储抽象：local 与 S3 兼容后端由 `github.com/NoelOrin/objectstore` 提供，本模块负责适配成项目自己的 `Provider`，并保留加解密工具。

## 文件

| 文件 | 职责 |
|------|------|
| `provider.go` | `Provider` 接口 + `PresignedResult` |
| `objectstore_provider.go` | `ObjectStoreProvider`：适配 `objectstore.Backend` 与可选 `objectstore.Presigner`，`NewProvider` 工厂 |
| `crypto.go` | `Cipher`：`Encrypt`/`Decrypt`、`EncryptSecret`/`DecryptSecret`、`InitCipher` |

## Provider 接口

`PresignUpload(key, contentType, size)`、`UploadFromReader`、`GetPublicURL`、`Delete`、`TestConnection`、`HeadObjectSize`。

## 库边界

- `objectstore.Backend` 负责 `Put`/`Stat`/`Delete`/`TestConnection`；S3 的 `PresignUpload` 来自可选接口 `objectstore.Presigner`，公开 URL 来自 `objectstore.PublicURLer`（local/memory 不实现 presigner，自然退化为中转上传）。
- URL 基址、S3 超时与预签名有效期全部走 `objectstore.Config`：`PublicURL`/`S3PublicBaseURL`、`S3Timeout`(5s)、`S3PresignTTL`(5m)，业务侧不自己拼 URL。
- **不设置库的 `S3Prefix`**：handler 已把 `path_prefix` 写进 object key，重复设置会双重前缀。
- 库的 `Get` 有 64MB 上限、`Put` 只接受 `[]byte`；本模块当前不用 `Get`，上传会把文件读入内存（`MaxFileSize` 默认 5MB）。
- 库要求 Go 1.27.4，server 的 `go.mod` 已对齐；依赖走已发布版本 `v0.1.0`，不在 `go.mod` 里保留指向本机路径的 `replace`，否则 CI 无法解析依赖。

## 易错点

- **S3 预签名的 `size` 会写进 `ContentLength` 签名**：客户端实际字节数必须与之一致，否则上传失败。前端必须先取真实 size。
- 本地上传目录与 `/uploads` 静态服务共用 `STORAGE_LOCAL_DIR`（默认 `./uploads`）；`STORAGE_PATH_PREFIX` 只影响 S3 对象键，本地忽略。
- 生产必须配置 `STORAGE_ENCRYPT_KEY`（64 位 hex）；dev 有全零兜底键，**不要在生产依赖兜底**。
- 上传完成后用 `HeadObjectSize` 复核大小，不要信任客户端声明。
- 对象键必须经过安全校验（库 local 后端会把 `../` 归一到根目录内，不会逃逸）。
