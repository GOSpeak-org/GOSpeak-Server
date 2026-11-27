package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type PluginMetadata struct {
	PluginID    string
	Name        string
	Version     string
	Runtime     string
	Entrypoint  string
	NPMRegistry string
	NPMArgs     []string
}

type metadataFile struct {
	PluginID    string   `yaml:"plugin_id"`
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Runtime     string   `yaml:"runtime"`
	Entrypoint  string   `yaml:"entrypoint"`
	NPMRegistry string   `yaml:"npm_registry"`
	NPMArgs     []string `yaml:"npm_args"`
}

func parseMetadata(raw []byte) (PluginMetadata, error) {
	var file metadataFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return PluginMetadata{}, fmt.Errorf("parse metadata: %w", err)
	}
	md := PluginMetadata{
		PluginID:    strings.TrimSpace(file.PluginID),
		Name:        strings.TrimSpace(file.Name),
		Version:     strings.TrimSpace(file.Version),
		Runtime:     strings.TrimSpace(file.Runtime),
		Entrypoint:  strings.TrimSpace(file.Entrypoint),
		NPMRegistry: strings.TrimSpace(file.NPMRegistry),
		NPMArgs:     file.NPMArgs,
	}
	if md.PluginID == "" || strings.ContainsAny(md.PluginID, "/\\:") || md.PluginID == "." || md.PluginID == ".." {
		return PluginMetadata{}, errors.New("metadata: invalid plugin_id")
	}
	if md.Runtime == "" {
		md.Runtime = "node"
	}
	if md.Entrypoint == "" {
		md.Entrypoint = "index.js"
	}
	if filepath.Base(md.Entrypoint) != md.Entrypoint {
		return PluginMetadata{}, errors.New("metadata: entrypoint must be a plain file name")
	}
	return md, nil
}

func validatePluginID(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\:") || id == "." || id == ".." {
		return fmt.Errorf("invalid plugin id: %q", id)
	}
	return nil
}

func credentialPort(data []byte) (int, error) {
	raw := strings.TrimSpace(string(data))
	if _, after, ok := strings.Cut(raw, ":"); ok {
		raw = after
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid credential port: %q", strings.TrimSpace(string(data)))
	}
	return port, nil
}
