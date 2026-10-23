// Package models manages local offline AI models and their binaries.
package models

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Info describes a detected model on disk.
type Info struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // asr | llm | translation
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Installed bool   `json:"installed"`
	Error     string `json:"error,omitempty"`
}

// ASRModel holds verified paths to sherpa-onnx zipformer model files.
type ASRModel struct {
	Dir     string
	Tokens  string
	Encoder string
	Decoder string
	Joiner  string
	Size    int64
}

// Manager finds and validates models in configured directories.
type Manager struct {
	mu        sync.Mutex
	modelsDir string
	binDir    string
}

// NewManager creates a manager given the base models and bin directories.
func NewManager(modelsDir, binDir string) *Manager {
	return &Manager{modelsDir: modelsDir, binDir: binDir}
}

// SetDirs updates the search directories.
func (m *Manager) SetDirs(modelsDir, binDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelsDir = modelsDir
	m.binDir = binDir
}

// FindSherpaBin locates the sherpa-onnx executable.
func (m *Manager) FindSherpaBin() (string, error) {
	m.mu.Lock()
	binDir := m.binDir
	m.mu.Unlock()

	name := "sherpa-onnx"
	var cands []string
	if binDir != "" {
		cands = append(cands, filepath.Join(binDir, name+".exe"), filepath.Join(binDir, name))
	} else {
		if appData := os.Getenv("APPDATA"); appData != "" {
			cands = append(cands, filepath.Join(appData, "Ottr", "bin", name+".exe"), filepath.Join(appData, "Ottr", "bin", name))
		}
		if exe, err := os.Executable(); err == nil {
			cands = append(cands,
				filepath.Join(filepath.Dir(exe), "bin", name+".exe"),
				filepath.Join(filepath.Dir(exe), "bin", name),
				filepath.Join(filepath.Dir(exe), name+".exe"),
				filepath.Join(filepath.Dir(exe), name),
			)
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	// Fall back to PATH.
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("sherpa-onnx binary not found; please ensure it is installed in %s or on PATH", binDir)
}

// FindASRModel searches for a valid sherpa-onnx streaming zipformer model.
func (m *Manager) FindASRModel() (*ASRModel, error) {
	m.mu.Lock()
	dir := m.modelsDir
	m.mu.Unlock()

	var dirs []string
	if dir != "" {
		dirs = append(dirs, dir)
	} else {
		if appData := os.Getenv("APPDATA"); appData != "" {
			dirs = append(dirs, filepath.Join(appData, "Ottr", "models"))
		}
		if exe, err := os.Executable(); err == nil {
			dirs = append(dirs, filepath.Join(filepath.Dir(exe), "models"))
		}
		dirs = append(dirs, "models")
	}

	var lastErr error
	for _, base := range dirs {
		if base == "" {
			continue
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			target := filepath.Join(base, e.Name())
			if mdl, err := inspectASRDir(target); err == nil {
				return mdl, nil
			} else {
				lastErr = err
			}
		}
		// Also inspect the base dir itself if it holds the files directly
		if mdl, err := inspectASRDir(base); err == nil {
			return mdl, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no valid ASR model found: %w", lastErr)
	}
	return nil, fmt.Errorf("no ASR model installed; please place a streaming zipformer model in %s", dir)
}

func inspectASRDir(dir string) (*ASRModel, error) {
	tokens := filepath.Join(dir, "tokens.txt")
	if st, err := os.Stat(tokens); err != nil || st.Size() == 0 {
		return nil, errors.New("missing tokens.txt")
	}

	encoder, err := findOnnx(dir, "encoder")
	if err != nil {
		return nil, err
	}
	decoder, err := findOnnx(dir, "decoder")
	if err != nil {
		return nil, err
	}
	joiner, err := findOnnx(dir, "joiner")
	if err != nil {
		return nil, err
	}

	var totalSize int64
	for _, p := range []string{tokens, encoder, decoder, joiner} {
		if st, err := os.Stat(p); err == nil {
			totalSize += st.Size()
		}
	}

	return &ASRModel{
		Dir:     dir,
		Tokens:  tokens,
		Encoder: encoder,
		Decoder: decoder,
		Joiner:  joiner,
		Size:    totalSize,
	}, nil
}

func findOnnx(dir, prefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	// Prefer .int8.onnx for lower memory and faster CPU inference.
	var int8Match, generalMatch string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".onnx") {
			full := filepath.Join(dir, name)
			if strings.HasSuffix(name, ".int8.onnx") {
				int8Match = full
			} else if generalMatch == "" {
				generalMatch = full
			}
		}
	}
	if int8Match != "" {
		return int8Match, nil
	}
	if generalMatch != "" {
		return generalMatch, nil
	}
	return "", fmt.Errorf("missing %s ONNX model in %s", prefix, dir)
}

// ListModels enumerates all known models in the models directory.
func (m *Manager) ListModels() ([]Info, error) {
	m.mu.Lock()
	dir := m.modelsDir
	m.mu.Unlock()

	var list []Info
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return list, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		info := Info{
			ID:   e.Name(),
			Name: e.Name(),
			Kind: "asr",
			Path: sub,
		}
		if mdl, err := inspectASRDir(sub); err == nil {
			info.Installed = true
			info.SizeBytes = mdl.Size
		} else {
			info.Error = err.Error()
		}
		list = append(list, info)
	}
	return list, nil
}
