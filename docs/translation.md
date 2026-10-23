# Translation

Online subtitle translation. Local/offline models are deliberately out of scope
for now (see ROADMAP).

## Providers

One `Translator` interface, two implementations:

```go
type Translator interface {
	Translate(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error)
}
```

| Provider | Endpoint | Auth | Notes |
|---|---|---|---|
| `mymemory` (default) | `https://api.mymemory.translated.net/get` | none | Free public API; used per segment; concurrency limited to 3 |
| `custom` | any OpenAI-compatible `/v1/chat/completions` | Bearer key | One prompt for all lines, numbered output |

`custom` works with DeepSeek, OpenAI, SiliconFlow, Moonshot, and local Ollama
(`localhost`/`127.0.0.1` bases skip the API-key requirement).

Provider selection and credentials live in the `settings` table under
`trans_provider`, `trans_base_url`, `trans_api_key`, `trans_model`, and are
loaded on `Open`.

### MyMemory behavior

One HTTP request per non-empty segment, `langpair=src|tgt`, with a 15 s timeout.
Up to three requests run concurrently. Language codes are normalized to
MyMemory's expectations (`zh→zh-CN`, `en→en-GB`, `ja→ja-JP`). The first error
aborts the batch.

### Custom-LLM behavior

All segments go into a single prompt asking for numbered lines, with
`temperature: 0.2`. The reply is parsed by `parseNumberedTranslations`, which
tolerates extra prose. Unparsed slots fall back to the original text so the
result never contains holes.

## Alignment and storage

Original and translated subtitles are stored **separately** and linked:

```
transcript(media_id) → segment(id, idx, start_ms, end_ms, text)
translation(media_id, target_lang) → translation_segment(segment_id, idx, start_ms, end_ms, original_text, translated_text)
```

`translation_segment` denormalizes `start_ms`/`end_ms`/`original_text` so a
translation row is self-contained for export, while `segment_id` preserves the
link to the source segment.

`UNIQUE(media_id, target_lang)` means one translation per target language;
re-translating replaces the previous one inside a transaction.

Supported target languages in the UI: `en`, `zh`, `ja`. The source language is
taken from the transcript row (currently always `zh`).

## Export formats

| Format ID | Output |
|---|---|
| `srt` | translated text only |
| `bilingual_srt` | translated line, then original line, per cue |
| `vtt` | translated text, `WEBVTT` header |
| `txt` | translated text, one line per cue |

## Preconditions

`Translate` requires that transcription has already produced segments and
returns `no transcript found for this media; please transcribe first`
otherwise. The UI mirrors this: the Translate tab shows a "Go to Transcript"
button when the transcript is missing.

## Testing

- `TestFormatSRT / TestFormatBilingualSRT / TestFormatVTT` — timestamp and
  line-ordering assertions.
- `TestSettingsPersist` — settings round-trip through SQLite.
- `TestRealOnlineTranslation` — **live network call** translating `你好世界`
  through MyMemory, asserting a non-empty translation is both returned and
  readable back from the database. Skips only if the endpoint is unreachable.

## Known limitations

- MyMemory is rate-limited per IP and is not a premium MT engine; quality on
  long or technical sentences varies.
- One request per segment means latency scales with subtitle count.
- Source language detection is not implemented.
