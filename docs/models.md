# Models

Model management is deliberately minimal: Ottr **discovers and validates** model
files the user placed on disk. It does not download them, so the installer stays
small and the user controls model provenance and licensing.

## Directory layout

```
<userData>/
  bin/
    sherpa-onnx.exe          (+ onnxruntime.dll, onnxruntime_providers_shared.dll)
  models/
    sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23/
      tokens.txt
      encoder-epoch-99-avg-1.int8.onnx
      decoder-epoch-99-avg-1.int8.onnx
      joiner-epoch-99-avg-1.int8.onnx
```

`models.Manager` is constructed with `(modelsDir, binDir)` and pointed at those
paths in `main.go` after `PathUserData` resolves.

## Binary discovery

`FindSherpaBin` checks, in order:

1. `<binDir>/sherpa-onnx.exe`, `<binDir>/sherpa-onnx`
2. `%APPDATA%\Ottr\bin\...`
3. Next to the executable: `<exeDir>/bin/...`, `<exeDir>/...`
4. `PATH`

When an explicit `binDir` was configured, only step 1 is used — this keeps tests
hermetic. Missing binary produces an actionable error naming the expected
directory.

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

## Listing

`ListModels` returns `[]Info{ID, Name, Kind, Path, SizeBytes, Installed, Error}`
for every subdirectory, marking `Installed` only when validation passes and
carrying the validation error otherwise.

> The UI does not surface this yet — there is no Settings/Models screen. See
> ROADMAP.

## Licensing and distribution notes

- The 14M Chinese Zipformer used during development is Apache-2.0.
- Model weights are **not** bundled with Ottr; the user supplies them.
- `sherpa-onnx` and `onnxruntime` binaries are also user-supplied at this stage;
  shipping them is a packaging task (ROADMAP) and must respect their licenses.

## Testing

- `TestManagerNoModels` — empty directory yields an empty list and a clear error
  from `FindASRModel`.
- `TestManagerDetectsModel` — a directory with the four required files is
  detected as installed, and the reported size equals the sum of file sizes.
- `TestTranscribeRealModel` — uses the genuinely installed model to run
  recognition.
