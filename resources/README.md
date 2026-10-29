# Resources shipped with the app

Files here are copied into the app by `mygo build` and `mygo dev`: into
`Contents/Resources` inside a macOS `.app`, and next to the executable on
Windows and Linux. At run time `mygo.App.Path(mygo.PathResources)` is that
directory.

## The ASR library

Transcription calls sherpa-onnx in process, so its C API library has to be
**here**, not only in `%APPDATA%\Ottr\bin`: `internal/asr` looks in
`resources/bin` first and falls back to the user's bin folder for a manual
install.

Put the files in the directory of each platform the app is built for
(`mygo build -platform ...`), laid out like this:

```
resources/
├── windows-amd64/bin/
│   ├── sherpa-onnx-c-api.dll
│   ├── onnxruntime.dll
│   └── onnxruntime_providers_shared.dll
├── darwin-arm64/bin/
│   ├── libsherpa-onnx-c-api.dylib
│   ├── libonnxruntime.dylib
│   └── libonnxruntime_providers_shared.dylib
├── darwin-amd64/bin/            (same files, Intel builds)
├── linux-amd64/bin/
│   ├── libsherpa-onnx-c-api.so
│   ├── libonnxruntime.so
│   └── libonnxruntime_providers_shared.so
└── linux-arm64/bin/             (same files, ARM64 builds)
```

Use the release matching the pinned version in
[Models](../docs/models.md#native-runtime) (v2.0.1), and keep the library and
its `onnxruntime` together: a mismatched runtime crashes in the C library
rather than returning an error. `cmd/setup` fetches exactly that pair.

The build signs these files along with the app, so an installed Ottr needs no
separate ASR install and no `PATH` change. A platform directory with no files
in it is ignored; building for a platform whose directory is missing produces
an app that runs without ASR and says so.

## Models are not here

Model weights are the user's, and large: they stay in `<userData>/models`,
chosen in Settings. Nothing in this directory replaces that.
