package models

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Supported is one model Ottr can install by itself: where the archive comes
// from, what it must hash to, and the languages it hears.
type Supported struct {
	ID     string // the folder under the models directory
	Kind   string // the family inspectASRDir reports
	Name   string
	Detail string // the languages, as the settings row writes them
	Size   int64  // the archive's size in bytes
	URL    string
	SHA256 string
}

// supported lists the model families Ottr accepts, with the pinned archive of
// each: a different release is a change to this table, not a runtime choice.
var supported = []Supported{
	{
		ID:     "sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17",
		Kind:   KindSenseVoice,
		Name:   "SenseVoice Small",
		Detail: "zh · en · ja · ko · yue",
		Size:   163002883,
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2",
		SHA256: "7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e",
	},
	{
		ID:     "sherpa-onnx-funasr-nano-int8-2025-12-30",
		Kind:   KindFunASRNano,
		Name:   "FunASR Nano",
		Detail: "zh · en",
		Size:   841730611,
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-funasr-nano-int8-2025-12-30.tar.bz2",
		SHA256: "eb43d7ccc2e86b243f6a03b7df361033dda66db9523d1a92bf6aca2b50c9476b",
	},
	{
		ID:     "sherpa-onnx-qwen3-asr-0.6B-int8-2026-03-25",
		Kind:   KindQwen3ASR,
		Name:   "Qwen3 ASR 0.6B",
		Detail: "52 languages",
		Size:   878702423,
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-qwen3-asr-0.6B-int8-2026-03-25.tar.bz2",
		SHA256: "393f8a14e2f5fb96746aaab342997a40641001fbd5bf9592a080a8329178ee96",
	},
}

// SupportedModels are the models Ottr can run, downloaded or not.
func SupportedModels() []Supported {
	out := make([]Supported, len(supported))
	copy(out, supported)
	return out
}

// Offer is one supported model as the settings page shows it: the copy on
// disk when there is one, else what a download would fetch.
type Offer struct {
	ID           string
	Name         string
	Detail       string
	Kind         string
	Path         string // where a usable copy lives, "" when none is
	Installed    bool
	SizeBytes    int64  // the copy's size on disk
	DownloadSize int64  // the archive's size when it is not installed
	Error        string // why the copy on disk cannot be used
}

// Available lists every supported model in catalog order: each copy on disk as
// its own offer, and the size still to download for a family with none.
func (m *Manager) Available() []Offer {
	list, _ := m.ListModels()
	byKind := make(map[string][]Info, len(list))
	for _, info := range list {
		if info.Installed {
			byKind[info.Kind] = append(byKind[info.Kind], info)
		}
	}
	selected := m.SelectedDir()
	var out []Offer
	for _, s := range supported {
		copies := byKind[s.Kind]
		if len(copies) == 0 {
			out = append(out, Offer{ID: s.ID, Name: s.Name, Detail: s.Detail, Kind: s.Kind, DownloadSize: s.Size})
			continue
		}
		// The chosen copy leads; the others follow, each usable on its own.
		pick := copies[0]
		for _, copy := range copies {
			if copy.Path == selected {
				pick = copy
			}
		}
		ordered := append([]Info{pick}, copies...)
		seen := make(map[string]bool, len(copies))
		for _, copy := range ordered {
			if seen[copy.Path] {
				continue
			}
			seen[copy.Path] = true
			out = append(out, Offer{
				ID: copy.ID, Name: s.Name, Detail: s.Detail, Kind: s.Kind,
				Path: copy.Path, Installed: true, SizeBytes: copy.SizeBytes,
			})
		}
	}
	return out
}

// Download installs one supported model from its pinned archive, reporting how
// much of it has arrived as a fraction.
func (m *Manager) Download(ctx context.Context, id string, progress func(float64)) error {
	for _, s := range supported {
		if s.ID == id {
			return m.Install(ctx, s, progress)
		}
	}
	return fmt.Errorf("unknown model %q", id)
}

// Install fetches one archive, checks it against its SHA-256, and unpacks it
// into the models directory. A failed install leaves nothing behind.
func (m *Manager) Install(ctx context.Context, s Supported, progress func(float64)) error {
	dir := m.ModelsDir()
	if dir == "" {
		return errors.New("no models folder is configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, s.ID)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s is already installed", s.Name)
	}

	archive, err := os.CreateTemp(dir, ".download-*.tar.bz2")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	hash := sha256.New()
	if err := fetch(ctx, s.URL, io.MultiWriter(archive, hash), s.Size, progress); err != nil {
		archive.Close()
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != s.SHA256 {
		return fmt.Errorf("%s: the archive hashes to %s, not %s", s.Name, got, s.SHA256)
	}

	stage, err := os.MkdirTemp(dir, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractTarBz2(archive.Name(), stage); err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}
	if _, err := inspectASRDir(stage); err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}
	return os.Rename(stage, target)
}

// fetch copies a URL into w, calling progress with the fraction received.
func fetch(ctx context.Context, url string, w io.Writer, size int64, progress func(float64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = size
	}
	var seen int64
	buf := make([]byte, 512<<10)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return err
			}
			seen += int64(n)
			if progress != nil && total > 0 {
				progress(min(float64(seen)/float64(total), 1))
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("download %s: %w", url, readErr)
		}
	}
}

// extractTarBz2 unpacks a model archive into dest, dropping the archive's own
// root folder so the files land where inspectASRDir looks for them.
func extractTarBz2(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(bzip2.NewReader(f))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		if i := strings.IndexByte(name, '/'); i < 0 {
			continue
		} else {
			name = name[i+1:]
		}
		if name == "" || name == "." || strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry escapes the folder: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}
