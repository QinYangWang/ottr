package asr

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every pinned target installs the libraries the runtime loads, and the file
// names match what internal/asr and native/ look for.
func TestTargetsInstallWhatTheRuntimeLoads(t *testing.T) {
	for _, target := range Targets() {
		files := RuntimeFiles(target)
		if len(files) == 0 {
			t.Errorf("%s installs nothing", target)
		}
		// The name runtime.go looks for on that platform has to be installed.
		want := map[string]string{
			"windows": "sherpa-onnx-c-api.dll",
			"darwin":  "libsherpa-onnx-c-api.dylib",
			"linux":   "libsherpa-onnx-c-api.so",
		}[target.OS]
		found := false
		for _, f := range files {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s installs %v, which does not include %s", target, files, want)
		}
	}
}

func TestParseTarget(t *testing.T) {
	got, err := ParseTarget("windows/amd64")
	if err != nil || got.OS != "windows" || got.Arch != "amd64" || got.Dir() != "windows-amd64" {
		t.Fatalf("ParseTarget: %+v %v", got, err)
	}
	for _, bad := range []string{"", "windows", "windows/", "/amd64"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("ParseTarget(%q) should fail", bad)
		}
	}
}

// Bytes that are not the pinned archive are refused, so a release re-uploaded
// under the same name fails out loud instead of installing unchecked code.
func TestInstallRefusesBytesThatAreNotThePinnedArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not the archive")
	}))
	defer srv.Close()
	old := releaseURL
	releaseURL = func(string, string) string { return srv.URL + "/x" }
	t.Cleanup(func() { releaseURL = old })

	dir := filepath.Join(t.TempDir(), "bin")
	err := Install(t.Context(), Current, Target{OS: "windows", Arch: "amd64"}, dir, t.TempDir(), false, func(string, ...any) {})
	if err == nil {
		t.Fatal("bytes with the wrong digest were installed")
	}
	if !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("the error does not say what went wrong: %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) > 0 {
		t.Errorf("a refused archive left %d files behind", len(left))
	}
}

// A good archive in the cache is not downloaded again, and the libraries land
// with their names.
func TestInstallFromACachedArchive(t *testing.T) {
	target := Target{OS: "linux", Arch: "amd64"}
	a := Current.Targets[target]
	archive := tarBytes(t, append(RuntimeFiles(target), "libsherpa-onnx-cxx-api.so"))
	cache := t.TempDir()
	cached := filepath.Join(cache, a.name)
	if err := os.WriteFile(cached, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	// The cache is keyed by name and checked by digest, so the digest has to be
	// the test archive's own.
	rel := Version{Tag: Current.Tag, Targets: map[Target]asset{target: {name: a.name, sha: digestOf(t, archive)}}}
	savedUnpack := unpack
	unpack = func(path string, t Target, dir string, force bool) ([]string, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return untar(f, t, dir, force)
	}
	t.Cleanup(func() { unpack = savedUnpack })

	dir := t.TempDir()
	var said string
	if err := Install(t.Context(), rel, target, dir, cache, false, func(f string, args ...any) { said += f }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "cached") {
		t.Errorf("the cached archive was not reused: %q", said)
	}
	if err := Check(rel, dir, target); err != nil {
		t.Fatalf("installed files do not check out: %v", err)
	}
}

// Installing twice leaves the second run with nothing to do.
func TestInstallIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := Target{OS: "linux", Arch: "amd64"}
	for _, name := range RuntimeFiles(target) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := Check(Current, dir, target); err != nil {
		t.Fatalf("Check: %v", err)
	}
	var said string
	if err := Install(t.Context(), Current, target, dir, t.TempDir(), false, func(f string, a ...any) { said += f }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "already installed") {
		t.Errorf("a complete directory was not recognized: %q", said)
	}
}

// The extractor takes the libraries a target needs and leaves the rest of the
// archive alone.
func TestExtractTakesOnlyWhatIsNeeded(t *testing.T) {
	names := append(RuntimeFiles(Target{OS: "linux", Arch: "amd64"}),
		"libsherpa-onnx-cxx-api.so", "onnxruntime_providers_shared.dll", "sherpa-onnx")
	out := t.TempDir()
	written, err := untar(bytes.NewReader(tarBytes(t, names)), Target{OS: "linux", Arch: "amd64"}, out, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %v, want only the two libraries", written)
	}
	for _, name := range written {
		st, err := os.Stat(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("%s was not written: %v", name, err)
		}
		// Windows has no executable bit; elsewhere the loader needs one.
		if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s is not executable", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "libsherpa-onnx-cxx-api.so")); err == nil {
		t.Error("the C++ wrapper was installed")
	}
	if _, err := os.Stat(filepath.Join(out, "onnxruntime_providers_shared.dll")); err == nil {
		t.Error("another platform's library was installed")
	}
}

// tarBytes is a tar stream with one small file per name, laid out as the
// release archives are, so the extractor can be tested without the network.
func tarBytes(t *testing.T, names []string) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, name := range names {
		body := []byte("data")
		if err := tw.WriteHeader(&tar.Header{Name: "root/lib/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

// digestOf is the SHA-256 of a test archive, hex encoded.
func digestOf(t *testing.T, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestDigestOfAKnownFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := checksum(p, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if err != nil || !ok {
		t.Fatalf("checksum of \"abc\": %v %v", ok, err)
	}
	if ok, _ := checksum(p, "0000"); ok {
		t.Error("a wrong digest matched")
	}
}

// The newest recorded release is what a build gets, and an older one is still
// installable by name.
func TestVersionSelection(t *testing.T) {
	if Current.Tag != "v2.0.1" {
		t.Errorf("Current is %s, want the newest recorded release", Current.Tag)
	}
	if got := Supported(); len(got) < 2 || got[0] != "v2.0.1" || got[1] != "v1.13.8" {
		t.Errorf("Supported is %v, want the newest first", got)
	}
	v, err := VersionFor("1.13.8") // no leading v
	if err != nil || v.Tag != "v1.13.8" {
		t.Fatalf("VersionFor(1.13.8): %+v %v", v, err)
	}
	if _, ok := v.Targets[Target{OS: "windows", Arch: "amd64"}]; !ok {
		t.Error("the older release has no windows/amd64 archive")
	}
}

// A release this build has no digests for is refused rather than downloaded.
func TestUnknownVersionIsRefused(t *testing.T) {
	_, err := VersionFor("v9.9.9")
	if err == nil {
		t.Fatal("an unknown release was accepted")
	}
	if !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("the error does not say what adding a release takes: %v", err)
	}
}
