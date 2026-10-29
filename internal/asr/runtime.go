package asr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"ottr/internal/asr/native"
)

// libraryNames are the files the native runtime is looked for, best first. The
// pinned build is what docs/models.md describes; another version with the same
// entry points loads, and the version it reports is shown in Settings.
var libraryNames = func() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"sherpa-onnx-c-api.dll"}
	case "darwin":
		return []string{"libsherpa-onnx-c-api.dylib"}
	default:
		return []string{"libsherpa-onnx-c-api.so"}
	}
}()

// Runtime is the state of the native ASR library.
type Runtime struct {
	Path    string `json:"path,omitempty"`    // the library that loaded
	Version string `json:"version,omitempty"` // what it reports, e.g. "1.13.8"
	Err     string `json:"err,omitempty"`     // why there is no runtime
}

// OK reports whether native ASR can run.
func (r Runtime) OK() bool { return r.Err == "" && r.Path != "" }

// RuntimeStatus loads the native library and reports what happened. It is cheap
// after the first call: the library stays loaded for the life of the process.
func (s *Service) RuntimeStatus() Runtime {
	if !native.Available {
		return Runtime{Err: native.ErrUnavailable.Error()}
	}
	paths, dir := s.libraryCandidates()
	if len(paths) == 0 {
		return Runtime{Err: fmt.Sprintf("the sherpa-onnx library is not in %s: put %s there with onnxruntime beside it",
			dir, strings.Join(libraryNames, " / "))}
	}
	var lastErr string
	for _, p := range paths {
		v, err := native.Load(p)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if v == "" {
			v = "unknown version"
		}
		return Runtime{Path: p, Version: v}
	}
	return Runtime{Err: lastErr}
}

// ready fails with the runtime's own error when native ASR cannot run, so that
// every entry point that needs it gives the same answer.
func (s *Service) ready() error {
	if rt := s.RuntimeStatus(); !rt.OK() {
		return errors.New(rt.Err)
	}
	return nil
}

// libraryCandidates lists the library files to try, and the directory they are
// expected in.
//
// The app's own resources come first: an install carries the library with it.
// The configured bin directory comes next, which is where a user who installed
// it by hand puts it and what `go run` uses. Nothing on PATH is tried: a library
// from there could be any build with any dependencies.
func (s *Service) libraryCandidates() ([]string, string) {
	var dirs []string
	if res, err := mygo.App.Path(mygo.PathResources); err == nil {
		dirs = append(dirs, filepath.Join(res, "bin"))
	}
	if dir := s.models.BinDir(); dir != "" {
		dirs = append(dirs, dir)
	}
	var paths []string
	for _, dir := range dirs {
		for _, name := range libraryNames {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				paths = append(paths, p)
			}
		}
	}
	if len(dirs) == 0 {
		return nil, "<userData>/bin"
	}
	return paths, strings.Join(dirs, " or ")
}
