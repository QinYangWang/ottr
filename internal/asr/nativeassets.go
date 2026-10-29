package asr

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
	"sort"
	"strconv"
	"strings"
	"time"
)

// Target is one platform the app is built for, named as mygo build names it:
// "windows/amd64", "darwin/arm64", "linux/amd64".
type Target struct {
	OS   string
	Arch string
}

// String returns the target as goos/goarch.
func (t Target) String() string { return t.OS + "/" + t.Arch }

// Dir is the name of the platform directory the target's files go in, which is
// also what mygo build merges into the app: resources/<os>-<arch>/.
func (t Target) Dir() string { return t.OS + "-" + t.Arch }

// ParseTarget reads a target written as goos/goarch.
func ParseTarget(s string) (Target, error) {
	os, arch, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok || os == "" || arch == "" {
		return Target{}, fmt.Errorf("%q is not a target; write it as os/arch, such as windows/amd64", s)
	}
	return Target{OS: os, Arch: arch}, nil
}

// Version is a sherpa-onnx release whose archives this package knows the bytes
// of. Only releases listed here can be installed: fetching whatever a tag says
// today would mean running code nobody recorded a digest for.
type Version struct {
	Tag     string // the release tag, as in v2.0.1
	Targets map[Target]asset
}

// Current is the release apps are built with. It is the newest release whose
// archives this table records, not whatever the latest tag happens to be: a
// new release is adopted by filling in its table and running the real-model
// tests, and an operator can pin an older one with OTTR_SHERPA_VERSION.
var Current = Version{Tag: "v2.0.1", Targets: releases["v2.0.1"]}

// Supported lists the releases that can be installed, newest first.
func Supported() []string {
	tags := make([]string, 0, len(releases))
	for tag := range releases {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return compareVersions(tags[i], tags[j]) > 0 })
	return tags
}

// VersionFor returns the recorded archives of a release, with a leading "v"
// added when it is missing. An unknown release is an error, not a download:
// nothing about it has been checked.
func VersionFor(tag string) (Version, error) {
	if tag == "" {
		return Current, nil
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	a, ok := releases[tag]
	if !ok {
		return Version{}, fmt.Errorf("sherpa-onnx %s is not a release this build knows (%s); adding one means recording its archives' SHA-256 in internal/asr/nativeassets.go and re-running the real-model tests", tag, strings.Join(Supported(), ", "))
	}
	return Version{Tag: tag, Targets: a}, nil
}

// asset is one release archive: the smallest official build that has the C API
// library and the onnxruntime it needs, and nothing else (no TTS models, no
// example programs).
type asset struct {
	name string // the file name in the release
	sha  string // SHA-256 of the archive, so a changed release fails loudly
}

// releases records every release this build can install, keyed by tag. Adding
// one is three steps: download its archives, put their SHA-256 here, and run
// TestRealModelTranscribesAWav against them — the C API structs the shim
// compiles are the ABI, and a release may change them.
var releases = map[string]map[Target]asset{
	"v2.0.1": {
		{"windows", "amd64"}: {"sherpa-onnx-v2.0.1-win-x64-shared-MT-Release-no-tts-lib.tar.bz2", "21da6a8b92d63ca02ad41dbfc120012343aaaec2e5e02b14819c6b4f38004509"},
		{"windows", "arm64"}: {"sherpa-onnx-v2.0.1-win-arm64-shared-MT-Release-no-tts-lib.tar.bz2", "b3099bec60e6c3958275e90d0803d837cfe3ad6ed08528f29505d713b3a3e1bb"},
		{"darwin", "amd64"}:  {"sherpa-onnx-v2.0.1-osx-x64-shared-no-tts-lib.tar.bz2", "b0107b8166c4131245380bc9b984345c4ea8637c65807f09ca5f501c72926be5"},
		{"darwin", "arm64"}:  {"sherpa-onnx-v2.0.1-osx-arm64-shared-no-tts-lib.tar.bz2", "ac5594ad237db56ec4a6c4b3fc48700441b33a07bb71c235f35530a52a93f2e3"},
		{"linux", "amd64"}:   {"sherpa-onnx-v2.0.1-linux-x64-shared-no-tts-lib.tar.bz2", "ce437357915491e6f2754b52bec7dcf62ff6fc6aaed9cdcd66f17807bbaef374"},
		{"linux", "arm64"}:   {"sherpa-onnx-v2.0.1-linux-aarch64-shared-cpu-lib.tar.bz2", "15bd737eae467da32ec97e3740a329a535fd21e33caca4f327b0081451d11533"},
	},
	// Kept because it is what was verified before 2.0.1, so a build that has
	// to stay on it can say so.
	"v1.13.8": {
		{"windows", "amd64"}: {"sherpa-onnx-v1.13.8-win-x64-shared-MT-Release-no-tts-lib.tar.bz2", "a1253e665c4f236119c443c8932a8acfca32a546c78c05962d483c9a0eae21b7"},
		{"windows", "arm64"}: {"sherpa-onnx-v1.13.8-win-arm64-shared-MD-Release-no-tts-lib.tar.bz2", "bdd1e9a1334d2f79ce64fac4cc417d88a954f961f6055056c213c9f5321efdd4"},
		{"darwin", "amd64"}:  {"sherpa-onnx-v1.13.8-osx-x64-shared-no-tts-lib.tar.bz2", "c4d94cce92b6e04df1f17d247c3ac4e61b21359be80e940a0408e039e36a4d7e"},
		{"darwin", "arm64"}:  {"sherpa-onnx-v1.13.8-osx-arm64-shared-no-tts-lib.tar.bz2", "f3e0cbd86cc3f38dad30c97921b40e9a8bcc6f2c943777eb76ad77176993e417"},
		{"linux", "amd64"}:   {"sherpa-onnx-v1.13.8-linux-x64-shared-no-tts-lib.tar.bz2", "bf2d998c8b07012cd5098f3b92673bc1333fd9b927767d7cb664be8190d8bc0b"},
		{"linux", "arm64"}:   {"sherpa-onnx-v1.13.8-linux-aarch64-shared-cpu-lib.tar.bz2", "fb98b80628a383909bf83626daca2e0d0ffc93bf1f7d222eaaa4282b388f072f"},
	},
}

// compareVersions orders tags like v2.0.1 above v1.13.8, by their numbers.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return len(pa) - len(pb)
}

func versionParts(tag string) []int {
	var out []int
	for _, part := range strings.Split(strings.TrimPrefix(tag, "v"), ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// Targets lists every platform the current release has a build for, in a
// stable order.
func Targets() []Target { return targetsOf(Current) }

func targetsOf(v Version) []Target {
	out := make([]Target, 0, len(v.Targets))
	for t := range v.Targets {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// runtimeFiles are the archive entries a Target installs, by base name: the C
// API library and the onnxruntime it links against, and not what merely ships
// beside them (the C++ wrapper, import libraries, example programs).
var runtimeFiles = map[string][]string{
	"windows": {"sherpa-onnx-c-api.dll", "onnxruntime.dll", "onnxruntime_providers_shared.dll"},
	"darwin":  {"libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"},
	"linux":   {"libsherpa-onnx-c-api.so", "libonnxruntime.so"},
}

// RuntimeFiles are the library names a Target installs, so a caller can tell
// whether they are already there.
func RuntimeFiles(t Target) []string {
	names := append([]string(nil), runtimeFiles[t.OS]...)
	sort.Strings(names)
	return names
}

// Missing reports which of the target's libraries are not in dir yet.
func Missing(dir string, t Target) []string {
	var missing []string
	for _, name := range RuntimeFiles(t) {
		if st, err := os.Stat(filepath.Join(dir, name)); err != nil || st.IsDir() {
			missing = append(missing, name)
		}
	}
	return missing
}

// Install downloads one release's archive for a target, checks it against the
// recorded SHA-256, and writes its libraries into dir. Files already there are
// left alone, so running it twice costs nothing the second time; pass force to
// replace them.
//
// The archives are .tar.bz2, which the standard library reads but cannot write:
// a test replaces unpack, so an install can be exercised without the network.
//
// The download is cached by file name in cacheDir, so several targets and
// repeated builds of a pipeline fetch each archive once.
func Install(ctx context.Context, v Version, t Target, dir, cacheDir string, force bool, logf func(string, ...any)) error {
	a, ok := v.Targets[t]
	if !ok {
		return fmt.Errorf("sherpa-onnx %s has no build for %s; it covers %s", v.Tag, t, joinTargets(targetsOf(v)))
	}
	if !force && len(Missing(dir, t)) == 0 {
		logf("%s: already installed in %s", t, dir)
		return nil
	}
	archive, err := cached(ctx, v.Tag, a, cacheDir, logf)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	written, err := unpack(archive, t, dir, force)
	if err != nil {
		return fmt.Errorf("install %s: %w", t, err)
	}
	logf("%s: %d files in %s", t, len(written), dir)
	return nil
}

// Check says whether dir holds the target's libraries, without downloading
// anything. It is what a pipeline runs after installing, and what tells a
// person what is missing.
func Check(v Version, dir string, t Target) error {
	if _, ok := v.Targets[t]; !ok {
		return fmt.Errorf("sherpa-onnx %s has no build for %s", v.Tag, t)
	}
	if missing := Missing(dir, t); len(missing) > 0 {
		return fmt.Errorf("%s: %s missing from %s", t, strings.Join(missing, ", "), dir)
	}
	return nil
}

// cached returns the archive, downloading and checking it once.
func cached(ctx context.Context, tag string, a asset, cacheDir string, logf func(string, ...any)) (string, error) {
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		cacheDir = filepath.Join(base, "ottr", "asr")
	}
	path := filepath.Join(cacheDir, a.name)
	if ok, err := checksum(path, a.sha); err == nil && ok {
		logf("%s: cached", a.name)
		return path, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	url := releaseURL(tag, a.name)
	logf("%s: downloading from %s", a.name, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp(cacheDir, a.name+".*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name()) // no-op once renamed
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	// A release re-uploaded under the same name would change the digest, and
	// installing bytes nobody checked is the one thing this must not do.
	if got := hex.EncodeToString(h.Sum(nil)); got != a.sha {
		return "", fmt.Errorf("%s does not match the pinned SHA-256 (got %s); the release changed, so its version and this table need updating together", a.name, got)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

// releaseURL is where a release asset is downloaded from. A test replaces it,
// so that a truncated or replaced archive can be tested without GitHub.
var releaseURL = func(tag, name string) string {
	return "https://github.com/k2-fsa/sherpa-onnx/releases/download/" + tag + "/" + name
}

// checksum reports whether the file at path has the wanted SHA-256.
func checksum(path, want string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == want, nil
}

// unpack writes the wanted entries of the release archive into dir and returns
// their names.
var unpack = func(archive string, t Target, dir string, force bool) ([]string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return untar(bzip2.NewReader(f), t, dir, force)
}

// untar writes the entries of a tar stream that the target installs, and skips
// the rest: an archive carrying headers and example programs costs nothing.
func untar(r io.Reader, t Target, dir string, force bool) ([]string, error) {
	tr := tar.NewReader(r)
	var written []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return written, err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		name := filepath.Base(hdr.Name)
		if !wanted(name, t) {
			continue
		}
		dest := filepath.Join(dir, name)
		if !force {
			if _, err := os.Stat(dest); err == nil {
				continue
			}
		}
		if err := writeFile(dest, tr); err != nil {
			return written, err
		}
		written = append(written, name)
	}
	if len(written) == 0 {
		return nil, fmt.Errorf("the archive has none of the libraries %s installs (%s)", t, strings.Join(RuntimeFiles(t), ", "))
	}
	sort.Strings(written)
	return written, nil
}

// writeFile writes r to path through a temporary file, so an interrupted run
// never leaves half a library behind.
func writeFile(path string, r io.Reader) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Libraries are mapped as code by the loader, so they have to be readable
	// and executable.
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// wanted reports whether the archive entry name is one this target installs.
func wanted(name string, t Target) bool {
	for _, w := range runtimeFiles[t.OS] {
		if w == name {
			return true
		}
	}
	return false
}

func joinTargets(ts []Target) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.String()
	}
	return strings.Join(names, ", ")
}
