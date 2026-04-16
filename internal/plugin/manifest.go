// Package plugin — manifest.go
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func loadManifest(dirPath string) (*Manifest, error) {
	type candidate struct {
		filename string
		parse    func([]byte, *Manifest) error
	}
	candidates := []candidate{
		{"manifest.json", func(b []byte, m *Manifest) error { return json.Unmarshal(b, m) }},
		{"manifest.yaml", func(b []byte, m *Manifest) error { return yaml.Unmarshal(b, m) }},
		{"manifest.yml", func(b []byte, m *Manifest) error { return yaml.Unmarshal(b, m) }},
	}
	for _, c := range candidates {
		path := filepath.Join(dirPath, c.filename)
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", c.filename, err)
		}
		var manifest Manifest
		if err := c.parse(data, &manifest); err != nil {
			return nil, fmt.Errorf("parse %s: %w", c.filename, err)
		}
		return &manifest, nil
	}
	return nil, errNoManifest
}

func buildEnv(manifestEnv map[string]string, socketPath string) []string {
	env := os.Environ()
	for k, v := range manifestEnv {
		env = append(env, k+"="+v)
	}
	env = append(env, "VDB_SOCKET="+socketPath)
	return env
}
