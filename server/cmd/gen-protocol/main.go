package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"GOSpeak/internal/genprotocol"
)

func main() {
	var updateGolden bool
	var out string
	flag.BoolVar(&updateGolden, "update-golden", false, "overwrite internal/genprotocol/testdata/golden instead of the requested output directory")
	flag.StringVar(&out, "out", "", "output directory for generated TypeScript files (required unless -update-golden)")
	flag.Parse()

	if !updateGolden && out == "" {
		fmt.Fprintln(os.Stderr, "gen-protocol: -out is required (or use -update-golden)")
		flag.Usage()
		os.Exit(2)
	}

	pkgDir := filepath.Join("internal", "protocol")
	outDir := out
	if updateGolden {
		outDir = filepath.Join("internal", "genprotocol", "testdata", "golden")
	}
	files, err := genprotocol.Generate(pkgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-protocol: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gen-protocol: %v\n", err)
		os.Exit(1)
	}
	for _, name := range []string{"events.ts", "payloads.ts", "envelope.ts"} {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(files[name]), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gen-protocol: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("gen-protocol: wrote %d files to %s\n", len(files), outDir)
}
