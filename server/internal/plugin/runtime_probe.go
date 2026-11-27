package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
)

func resolveNode(root string) (string, string, bool) {
	managed := filepath.Join(root, "runtime", "bin", "node")
	if info, err := os.Stat(managed); err == nil && !info.IsDir() {
		if out, err := exec.Command(managed, "--version").Output(); err == nil && len(out) > 0 {
			return managed, "managed", true
		}
	}
	system, err := exec.LookPath("node")
	if err == nil {
		if out, err := exec.Command(system, "--version").Output(); err == nil && len(out) > 0 {
			return system, "system", true
		}
	}
	return "", "", false
}