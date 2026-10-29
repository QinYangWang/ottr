# Models

Model management is deliberately minimal: Ottr **discovers and validates** model
files the user placed on disk. It does not download them, so the installer stays
small and the user controls model provenance and licensing.

## Directory layout

```
<userData>/
  bin/
    sherpa-onnx-c-api.dll    the pinned native library
    onnxruntime.dll          + onnxruntime_providers_shared.dll, beside it

resources/                   what a packaged app carries (see resources/README.md)
  windows-amd64/bin/         merging into the app's own bin/ for that target
  darwin-arm64/bin/
  linux-amd64/bin/
  models/
    sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23/
      tokens.txt
      encoder-epoch-99-avg-1.int8.onnx
      decoder-epoch-99-avg-1.int8.onnx
      joiner-epoch-99-avg-1.int8.onnx
```

`models.Manager` is constructed with `(modelsDir, binDir)` and pointed at those
paths in `main.go` after `PathUserData` resolves.

## Native runtime

Transcription calls sherpa-onnx in process through its C API, so what is needed
is the **library**, not an executable:

| OS | file |
|---|---|
| Windows | `sherpa-onnx-c-api.dll` |
| macOS | `libsherpa-onnx-c-api.dylib` |
| Linux | `libsherpa-onnx-c-api.so` |

`Service.RuntimeStatus` looks in two places, in this order:

1. the app's own resources, `resources/bin` — where `mygo build` puts what an
   install carries, so a packaged Ottr needs no separate ASR install;
2. the configured bin directory, `%APPDATA%\Ottr\bin` by default — where a
   user installs it by hand, and what `go run` uses.

Nothing on `PATH` is loaded: a library from there could be any build with any
dependencies. On Windows the library's own folder is searched for its
dependencies (`onnxruntime.dll`) and `PATH` is not; macOS and Linux builds
should set `@rpath` / `$ORIGIN` deliberately. The library stays loaded for the
life of the process, and Settings shows the version it reports.

**Version: 2.0.1.** `internal/asr/native/sherpa_c_api_v2.0.1.h` is a verbatim copy
of `sherpa-onnx/c-api/c-api.h` from that release, and the shim compiles
sherpa-onnx's configuration structs from it rather than declaring them by hand —
hand-copied layouts were wrong twice before it was vendored.

That header is why the version is recorded rather than looked up: the structs
are the ABI. What is pinned is not a version number but **bytes**: each release
in `internal/asr/nativeassets.go` lists its six archives with their SHA-256, and
only those can be installed. The header, the archive table and what the real
model test accepts have to agree, so a release is adopted deliberately:

1. download its archives, put their digests in `releases`, and make it `Current`;
2. drop the new `c-api.h` in as `native/sherpa_c_api_v<version>.h` and switch the
   shim's include;
3. run the real-model tests.

An older release stays installable by name (`-version v1.13.8`), and if its
header is gone from the tree the shim would have to go back with it — the structs
compiled in are those of `Current`.

1.13.8 → 2.0.1 needed no code change: every struct the shim fills —
`SherpaOnnxOnlineRecognizerConfig`, `SherpaOnnxOnlineModelConfig`,
`SherpaOnnxOnlineRecognizerResult` — is byte-identical between them, all the
entry points are exported by both, and the same audio recognizes to the same
text. The only structural change was an offline field Ottr does not use.

A missing library is an honest ASR error naming the directories searched, never
a startup failure: recording, importing and the rest of the window work without
it.

### Packaging

The library ships **with** the app: the build for each platform goes in
`resources/<goos>-<goarch>/bin/`, and `mygo build -platform ...` merges it in
and signs it, so nothing has to be installed separately. Model weights are not
shipped — they are the user's, and stay in `<userData>/models`.

`CGO_ENABLED=1` is needed for both `go tool mygo build` and `go tool mygo dev`,
because the wrapper is C. The tool defaults to `CGO_ENABLED=0`, so set it for
every command; a `CGO_ENABLED=0` build has no ASR at all. It is also what makes
cross-compiling need a C toolchain for the target, though not for each target's
libraries: those are simply files.

### Fetching the libraries: `cmd/setup`

Nobody has to find the right release archive by hand.
`cmd/setup` downloads the pinned release for the platforms of a build, checks
every archive against its recorded SHA-256, and unpacks only the libraries into
the right platform directory:

```sh
go run ./cmd/setup                                  # this machine
go run ./cmd/setup -platform windows/amd64,darwin/arm64,linux/amd64
go run ./cmd/setup -platform all                     # every pinned platform
go run ./cmd/setup -check -platform all              # report, download nothing
```

| Flag | Meaning |
|---|---|
| `-platform` | `os/arch` separated by commas, or `all`; default this machine |
| `-out` | the resources directory (default `resources`); files land in `<out>/<os>-<arch>/bin` |
| `-cache` | where archives are cached (default the user cache directory) |
| `-check` | only say what is missing; exit 1 if anything is |
| `-force` | replace files that are already there |
| `-quiet` | print only warnings and errors |
| `-targets` | list the platforms that can be installed |

It installs the newest release this build records — `-version` (or
`OTTR_SHERPA_VERSION`) chooses another that is recorded, such as `v1.13.8`, and
a tag that is not recorded is refused rather than downloaded, because nothing
about it has been checked.

It is idempotent — a complete directory is left alone — and archives are cached
by name and checked by content, so a pipeline that builds several platforms
downloads each release once. `-check` is what a pipeline runs to fail early,
before a long build. The pinned table lives in `internal/asr/nativeassets.go`: the release, each
platform's archive, and its SHA-256. A release re-uploaded under the same name
therefore fails loudly instead of installing unchecked code.

A CI job is then two commands:

```sh
go run ./cmd/setup -platform all
CGO_ENABLED=1 go tool mygo build -platform windows/amd64,darwin/arm64,linux/amd64
```

The archives used are the smallest official builds that contain the C API and
its onnxruntime — the `-lib` variants, without TTS and without example programs.

For `v2.0.1` these are:

| Target | Archive |
|---|---|
| windows/amd64 | `sherpa-onnx-v2.0.1-win-x64-shared-MT-Release-no-tts-lib.tar.bz2` |
| windows/arm64 | `sherpa-onnx-v2.0.1-win-arm64-shared-MT-Release-no-tts-lib.tar.bz2` |
| darwin/amd64 | `sherpa-onnx-v2.0.1-osx-x64-shared-no-tts-lib.tar.bz2` |
| darwin/arm64 | `sherpa-onnx-v2.0.1-osx-arm64-shared-no-tts-lib.tar.bz2` |
| linux/amd64 | `sherpa-onnx-v2.0.1-linux-x64-shared-no-tts-lib.tar.bz2` |
| linux/arm64 | `sherpa-onnx-v2.0.1-linux-aarch64-shared-cpu-lib.tar.bz2` |

The files these install are the ones `RuntimeStatus` loads and the ones the
version's own binaries link against, so a packaged app is self-contained:
`sherpa-onnx-c-api.dll` with `onnxruntime.dll` and
`onnxruntime_providers_shared.dll` on Windows, and the `libsherpa-onnx-c-api` /
`libonnxruntime` pair elsewhere.

## Model validation

`FindASRModel` scans each candidate directory (configured models dir, then
`%APPDATA%\Ottr\models`, then `<exeDir>/models`, then `./models`) and inspects
each subdirectory with `inspectASRDir`:

- `tokens.txt` must exist and be non-empty.
- exactly one `encoder*onnx`, `decoder*onnx`, `joiner*onnx` must be resolvable.
  For each, `.int8.onnx` is preferred over full-precision `.onnx` (lower memory,
  faster CPU inference); full precision is the fallback.
- total size is summed for display.

A directory missing any file is skipped with the reason recorded, so the final
error explains what was wrong with the last candidate rather than just "not
found".

`ListModels` returns `[]Info{ID, Name, Kind, Path, SizeBytes, Installed, Error}`
for every subdirectory, marking `Installed` only when validation passes and
carrying the validation error otherwise.

## Choosing a model

The user chooses in the **Settings** window, Transcription section (the sliders
icon at the bottom of the sidebar), which lists every directory with its size or the reason it
cannot be used, says which model transcription uses now, and says which
native sherpa-onnx version loaded (or why none did). `Open folder` opens the models directory and `Re-check` looks
again after a model is installed by hand.

`Manager.Select(dir)` writes `settings.asr_model_dir` and `Open(db)` reads it
back, so the choice survives a restart. `FindASRModel` then returns that model
and nothing else: if it was deleted or broken, transcription fails naming the
directory instead of quietly using a different model. With no choice made, the
first valid directory is used, as before.

GPU selection is still absent: the recognizer is configured with
`provider = "cpu"` and two threads, and no build with CUDA providers is
shipped.


## Licensing and distribution notes

- The 14M Chinese Zipformer used during development is Apache-2.0.
- Model weights are **not** bundled with Ottr; the user supplies them.
- The `sherpa-onnx-c-api` library and `onnxruntime` are also user-supplied at
  this stage; shipping them is a packaging task (ROADMAP) and must respect
  their licenses (both Apache-2.0).

## Testing

- `TestManagerNoModels` — empty directory yields an empty list and a clear error
  from `FindASRModel`.
- `TestManagerDetectsModel` — a directory with the four required files is
  detected as installed, and the reported size equals the sum of file sizes.
- `TestSelectedModelIsUsedAndRemembered` — a chosen model is what
  `FindASRModel` returns, another `Manager` reads the same choice from the
  settings table, and a chosen model that lost a file reports that instead of
  falling back.
- `TestModelsPageChoosesAModel` — the page lists models, shows why a folder is
  unusable and that the native runtime is missing, and `Use` changes which model
  is in use.
- `TestTranscribeRealModel` — uses the genuinely installed model and library to
  run recognition.
