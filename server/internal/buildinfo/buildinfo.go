// Package buildinfo 承载构建时注入的版本信息。
//
// Dockerfile 与 .github/workflows/release.yml 通过 ldflags 注入：
//
//	-X 'GOSpeak/internal/buildinfo.Version=<ver>' -X 'GOSpeak/internal/buildinfo.Commit=<sha>'
//
// 未注入时（dev 热重载、go run、本地 go build）以仓库根 VERSION 文件为准；
// commit 同理走 COMMIT 文件兜底（Docker 镜像采用这种方式，避免用 ldflags
// 污染 go build 层的缓存键）。cmd 启动时调用 ResolveVersion / ResolveCommit
// 读取这两个文件，VERSION 是版本的权威来源。
// cmd（cobra version 命令）与 internal/service（系统更新检查）均从此处读取，
// 避免 internal 反向依赖入口包。
package buildinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Version 默认 dev，构建时注入或由 ResolveVersion 从 VERSION 文件读取。
	Version = "dev"
	// Commit 默认 unknown，构建时注入。
	Commit = "unknown"
)

const (
	versionFileName = "VERSION"
	commitFileName  = "COMMIT"
)

// semverPattern 只接受形如 1.2.3 / 1.2.3-rc.1 的内容，
// 防止误把占位文本（如 dev）当成可比较的版本号。
var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// ResolveVersion 在 ldflags 未注入版本（仍为 dev）时，从仓库根 VERSION 文件补全版本。
// 查找顺序覆盖：当前目录、上两级目录（app/server 运行）、可执行文件同级与上三级目录
// （air 的 app/server/tmp/main）。找不到或内容非法时保持 dev。
func ResolveVersion() {
	if Version != "dev" {
		return
	}
	if v := readInfoFile(versionFileName); semverPattern.MatchString(v) {
		Version = v
	}
}

// ResolveCommit 在 ldflags 未注入 commit（仍为 unknown）时，从 COMMIT 文件补全。
// 查阅路径与 VERSION 相同。Docker 镜像把 COMMIT 写成文件而非 ldflags，
// 这样 go build 层不随版本变化，能稳定命中缓存。
func ResolveCommit() {
	if Commit != "unknown" {
		return
	}
	if c := readInfoFile(commitFileName); c != "" {
		Commit = c
	}
}

// readInfoFile 在当前目录、上两级目录和可执行文件附近查找给定文件，
// 返回首个非空内容；覆盖项目根、app/server 运行目录与 air 临时目录。
func readInfoFile(name string) string {
	candidates := []string{
		name,
		filepath.Join("..", name),
		filepath.Join("..", "..", name),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, name),
			filepath.Join(dir, "..", "..", name),
			filepath.Join(dir, "..", "..", "..", name),
		)
	}
	for _, path := range candidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v
		}
	}
	return ""
}
