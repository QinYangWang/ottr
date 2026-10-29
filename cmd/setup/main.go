// Command setup fetches what Ottr needs to build and run outside the app: the
// sherpa-onnx native runtime for each platform it is built for. It exists so a
// build pipeline gets the same files a person would install by hand, checked
// against the pinned release, without a browser and without guessing URLs.
//
//	go run ./cmd/setup                    # this machine, into resources/<os>-<arch>/bin
//	go run ./cmd/setup -platform windows/amd64,darwin/arm64,linux/amd64
//	go run ./cmd/setup -platform all      # every platform the release covers
//	go run ./cmd/setup -check -platform all
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"ottr/internal/asr"
)

func main() {
	var (
		platforms = flag.String("platform", "", "targets to install, as os/arch separated by commas, or all (default: this machine)")
		out       = flag.String("out", "resources", "the resources directory the platform directories go in")
		cache     = flag.String("cache", "", "where archives are cached (default: the user cache directory)")
		check     = flag.Bool("check", false, "only report whether the files are there, downloading nothing")
		force     = flag.Bool("force", false, "replace files that are already installed")
		quiet     = flag.Bool("quiet", false, "print only warnings and errors")
		targets   = flag.Bool("targets", false, "list the platforms that can be installed, and exit")
		version   = flag.String("version", "", "the sherpa-onnx release to install (default: the newest this build records; or set OTTR_SHERPA_VERSION)")
	)
	flag.Usage = usage
	flag.Parse()

	logf := func(format string, args ...any) {
		if !*quiet {
			fmt.Printf(format+"\n", args...)
		}
	}
	if *targets {
		for _, t := range asr.Targets() {
			fmt.Println(t.String())
		}
		return
	}

	if *version == "" {
		*version = os.Getenv("OTTR_SHERPA_VERSION")
	}
	rel, err := asr.VersionFor(*version)
	if err != nil {
		fail(err)
	}
	want, err := parsePlatforms(*platforms, rel)
	if err != nil {
		fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failed := false
	for _, t := range want {
		dir := filepath.Join(*out, t.Dir(), "bin")
		if *check {
			if err := asr.Check(rel, dir, t); err != nil {
				fmt.Println("missing:", err)
				failed = true
			} else {
				logf("%s: ok in %s", t, dir)
			}
			continue
		}
		if err := asr.Install(ctx, rel, t, dir, *cache, *force, logf); err != nil {
			fmt.Fprintf(os.Stderr, "setup: %s: %v\n", t, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
	if !*check && !*quiet {
		fmt.Printf("\nNext: CGO_ENABLED=1 go tool mygo build -platform %s\n", platformFlag(want))
	}
}

// parsePlatforms reads -platform: empty means this machine, "all" every platform
// the release covers, otherwise a comma-separated list of os/arch.
func parsePlatforms(s string, rel asr.Version) ([]asr.Target, error) {
	switch strings.TrimSpace(s) {
	case "":
		return []asr.Target{{OS: runtime.GOOS, Arch: runtime.GOARCH}}, nil
	case "all":
		var all []asr.Target
		for t := range rel.Targets {
			all = append(all, t)
		}
		sort.Slice(all, func(i, j int) bool { return all[i].String() < all[j].String() })
		return all, nil
	}
	var out []asr.Target
	for _, part := range strings.Split(s, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		t, err := asr.ParseTarget(part)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// platformFlag writes targets the way mygo build takes them.
func platformFlag(ts []asr.Target) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.String()
	}
	return strings.Join(names, ",")
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: setup [flags]

Fetch the sherpa-onnx native runtime Ottr calls for speech recognition, for the
platforms an app is built for. Files land in <out>/<os>-<arch>/bin, which
`+"`mygo build`"+` merges into the app, so an install needs nothing else.

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Targets: %s

The release installed by default is sherpa-onnx %s, the newest whose archives this
build records; -version (or OTTR_SHERPA_VERSION) chooses another from %s.
Every archive is checked against its recorded SHA-256. A pipeline usually runs:

    go run ./cmd/setup -platform all
    CGO_ENABLED=1 go tool mygo build -platform windows/amd64,darwin/arm64,linux/amd64
`, strings.Join(mapTargets(asr.Targets()), ", "), asr.Current.Tag, strings.Join(asr.Supported(), ", "))
}

func mapTargets(ts []asr.Target) []string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.String()
	}
	return names
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "setup: %v\n", err)
	os.Exit(2)
}
