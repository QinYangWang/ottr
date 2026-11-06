// Package models manages the local offline AI models Ottr runs, and where the
// native library that runs them lives.
package models

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
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

const (
	KindSenseVoice = "sense_voice"
	KindFunASRNano = "funasr_nano"
	KindQwen3ASR   = "qwen3_asr"
)

// ASRModel holds verified paths to one supported model family.
type ASRModel struct {
	Dir            string
	Kind           string
	Tokens         string
	Model          string
	EncoderAdaptor string
	LLM            string
	Embedding      string
	Tokenizer      string
	ConvFrontend   string
	Encoder        string
	Decoder        string
	Size           int64
}

// Manager finds and validates models in configured directories.
type Manager struct {
	mu        sync.Mutex
	db        *sql.DB
	modelsDir string
	binDir    string
	selected  string // the model the user chose, preferred by FindASRModel
}

// NewManager creates a manager given the base models and bin directories.
func NewManager(modelsDir, binDir string) *Manager {
	return &Manager{modelsDir: modelsDir, binDir: binDir}
}

// Open connects the manager to the database, where the chosen model is
// remembered across launches.
func (m *Manager) Open(db *sql.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
	var selected string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'asr_model_dir'`).Scan(&selected); err == nil {
		m.selected = selected
	}
}

// Select makes one model directory the one FindASRModel returns, and remembers
// it. The directory is not checked here: ListModels says whether it is valid.
func (m *Manager) Select(dir string) error {
	m.mu.Lock()
	db := m.db
	m.mu.Unlock()
	if db == nil {
		return errors.New("models are not ready")
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('asr_model_dir', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, dir); err != nil {
		return err
	}
	m.mu.Lock()
	m.selected = dir
	m.mu.Unlock()
	return nil
}

// SelectedDir returns the model the user chose, or "" when none was chosen.
func (m *Manager) SelectedDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.selected
}

// ModelsDir returns the folder models are installed in.
func (m *Manager) ModelsDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.modelsDir
}

// BinDir returns the folder the native ASR library and its dependencies are
// installed in, or "" when none was configured.
func (m *Manager) BinDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.binDir != "" {
		return m.binDir
	}
	// No folder was configured: the one beside the data files Ottr itself
	// writes to is the one an install puts them in.
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "Ottr", "bin")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "bin")
	}
	return ""
}

// DiarizationModels returns the separate speaker segmentation and embedding
// models, which are not ASR choices and never appear in the model list.
func (m *Manager) DiarizationModels() (segmentation, embedding string, err error) {
	m.mu.Lock()
	dir := m.modelsDir
	m.mu.Unlock()
	root := filepath.Join(filepath.Dir(dir), "diarization")
	segmentation = filepath.Join(root, "pyannote", "model.onnx")
	embedding = filepath.Join(root, "eres2net.onnx")
	if !fileNonEmpty(segmentation) || !fileNonEmpty(embedding) {
		return "", "", errors.New("speaker diarization models are not installed")
	}
	return segmentation, embedding, nil
}

// SetDirs updates the search directories.
func (m *Manager) SetDirs(modelsDir, binDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelsDir = modelsDir
	m.binDir = binDir
}

// FindASRModel returns the model the user chose, or the first valid one in the
// models directory when none was chosen.
func (m *Manager) FindASRModel() (*ASRModel, error) {
	m.mu.Lock()
	dir := m.modelsDir
	selected := m.selected
	m.mu.Unlock()

	if selected != "" {
		mdl, err := inspectASRDir(selected)
		if err != nil {
			return nil, fmt.Errorf("the chosen ASR model in %s cannot be used, choose another in Models: %w", selected, err)
		}
		return mdl, nil
	}

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
	return nil, fmt.Errorf("no supported ASR model installed; use SenseVoice, FunASR Nano or Qwen3 ASR in %s", dir)
}

func inspectASRDir(dir string) (*ASRModel, error) {
	if tokens := filepath.Join(dir, "tokens.txt"); fileNonEmpty(tokens) {
		if model, err := findOnnx(dir, "model"); err == nil {
			return &ASRModel{Dir: dir, Kind: KindSenseVoice, Tokens: tokens, Model: model, Size: fileSizes(tokens, model)}, nil
		}
	}
	if tokenizer := findTokenizer(dir); tokenizer != "" {
		adaptor, adaptorErr := findOnnx(dir, "encoder_adaptor")
		llm, llmErr := findOnnx(dir, "llm")
		embedding, embeddingErr := findOnnx(dir, "embedding")
		if adaptorErr == nil && llmErr == nil && embeddingErr == nil {
			return &ASRModel{
				Dir: dir, Kind: KindFunASRNano, EncoderAdaptor: adaptor, LLM: llm, Embedding: embedding,
				Tokenizer: tokenizer, Size: fileSizes(adaptor, llm, embedding),
			}, nil
		}
		frontend, frontendErr := findOnnx(dir, "conv_frontend")
		encoder, encoderErr := findOnnx(dir, "encoder")
		decoder, decoderErr := findOnnx(dir, "decoder")
		if frontendErr == nil && encoderErr == nil && decoderErr == nil {
			return &ASRModel{
				Dir: dir, Kind: KindQwen3ASR, ConvFrontend: frontend, Encoder: encoder, Decoder: decoder,
				Tokenizer: tokenizer, Size: fileSizes(frontend, encoder, decoder),
			}, nil
		}
	}
	return nil, errors.New("not a supported SenseVoice, FunASR Nano or Qwen3 ASR model")
}

func fileNonEmpty(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func fileSizes(paths ...string) int64 {
	var total int64
	for _, path := range paths {
		if st, err := os.Stat(path); err == nil {
			total += st.Size()
		}
	}
	return total
}

func findTokenizer(dir string) string {
	candidate := filepath.Join(dir, "Qwen3-0.6B")
	if st, err := os.Stat(candidate); err == nil && st.IsDir() {
		return candidate
	}
	if fileNonEmpty(filepath.Join(dir, "tokenizer.json")) {
		return dir
	}
	return ""
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

func modelDisplayName(kind string) string {
	switch kind {
	case KindSenseVoice:
		return "SenseVoice"
	case KindFunASRNano:
		return "FunASR Nano"
	case KindQwen3ASR:
		return "Qwen3 ASR"
	default:
		return kind
	}
}

// CurrentName returns the selected model's display name, or an empty string
// when no supported model is available.
func (m *Manager) CurrentName() string {
	mdl, err := m.FindASRModel()
	if err != nil {
		return ""
	}
	return modelDisplayName(mdl.Kind)
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
		if !e.IsDir() || e.Name() == "test_wavs" || e.Name() == "Qwen3-0.6B" {
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
			info.Name = modelDisplayName(mdl.Kind)
			info.SizeBytes = mdl.Size
		} else {
			info.Error = err.Error()
		}
		list = append(list, info)
	}
	return list, nil
}
