package genprotocol

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"GOSpeak/internal/protocol"
)

func TestGolden(t *testing.T) {
	pkgDir := filepath.Join("..", "protocol")
	files, err := Generate(pkgDir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range []string{"events.ts", "payloads.ts", "envelope.ts"} {
		want, err := os.ReadFile(filepath.Join("testdata", "golden", name))
		if err != nil {
			t.Fatalf("read golden %s: %v", name, err)
		}
		if files[name] != string(want) {
			t.Errorf("%s drifted from golden; run go run ./cmd/gen-protocol -update-golden and commit", name)
		}
	}
}

func TestEventsHaveNoInternal(t *testing.T) {
	files, err := Generate(filepath.Join("..", "protocol"))
	if err != nil {
		t.Fatal(err)
	}
	events := files["events.ts"]
	internal := 0
	for _, ev := range protocol.Catalog {
		if ev.Dir != protocol.DirInternal {
			continue
		}
		internal++
		if strings.Contains(events, fmt.Sprintf(": %q", ev.Name)) {
			t.Errorf("internal event %q must not be declared in generated events.ts", ev.Name)
		}
	}
	if internal == 0 {
		t.Fatal("expected protocol.Catalog to contain DirInternal events")
	}
}
