// sherpa_shim.c loads the sherpa-onnx C API at run time and exposes the few
// calls Ottr makes as a small C ABI of its own. Loading at run time is what
// keeps ASR optional: a missing library becomes an error Go can report instead
// of an OS error that stops the program before it can show one.
//
// Configuration structs come from the vendored copy of sherpa-onnx's own
// c-api.h at the pinned version (sherpa_c_api_v2.0.1.h), so their layout is
// never guessed. Functions reach the library only through the pointers
// resolved below, never through the import library.

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#if defined(_WIN32)
#include <windows.h>
typedef HMODULE shim_lib;
#else
#include <dlfcn.h>
typedef void *shim_lib;
#endif

#define SHERPA_ONNX_BUILD_MAIN_LIB
#include "sherpa_c_api_v2.0.1.h"

typedef struct {
  float start;
  float end;
  int32_t speaker;
} OttrDiarizationSegment;

typedef struct {
  const char *(*version)(void);

  const SherpaOnnxOnlineRecognizer *(*online_create)(
      const SherpaOnnxOnlineRecognizerConfig *);
  void (*online_destroy)(const SherpaOnnxOnlineRecognizer *);
  const SherpaOnnxOnlineStream *(*online_stream_create)(
      const SherpaOnnxOnlineRecognizer *);
  void (*online_stream_destroy)(const SherpaOnnxOnlineStream *);
  void (*online_stream_accept)(const SherpaOnnxOnlineStream *, int32_t,
                               const float *, int32_t);
  int32_t (*online_stream_ready)(const SherpaOnnxOnlineRecognizer *,
                                 const SherpaOnnxOnlineStream *);
  void (*online_stream_decode)(const SherpaOnnxOnlineRecognizer *,
                               const SherpaOnnxOnlineStream *);
  const SherpaOnnxOnlineRecognizerResult *(*online_stream_result)(
      const SherpaOnnxOnlineRecognizer *, const SherpaOnnxOnlineStream *);
  void (*online_result_destroy)(const SherpaOnnxOnlineRecognizerResult *);
  int32_t (*online_stream_endpoint)(const SherpaOnnxOnlineRecognizer *,
                                    const SherpaOnnxOnlineStream *);
  void (*online_stream_reset)(const SherpaOnnxOnlineRecognizer *,
                              const SherpaOnnxOnlineStream *);
  void (*online_stream_finish)(const SherpaOnnxOnlineStream *);

  const SherpaOnnxOfflineRecognizer *(*offline_create)(
      const SherpaOnnxOfflineRecognizerConfig *);
  void (*offline_destroy)(const SherpaOnnxOfflineRecognizer *);
  const SherpaOnnxOfflineStream *(*offline_stream_create)(
      const SherpaOnnxOfflineRecognizer *);
  void (*offline_stream_destroy)(const SherpaOnnxOfflineStream *);
  void (*offline_stream_accept)(const SherpaOnnxOfflineStream *, int32_t,
                                const float *, int32_t);
  void (*offline_stream_decode)(const SherpaOnnxOfflineRecognizer *,
                                const SherpaOnnxOfflineStream *);
  const SherpaOnnxOfflineRecognizerResult *(*offline_stream_result)(
      const SherpaOnnxOfflineStream *);
  void (*offline_result_destroy)(const SherpaOnnxOfflineRecognizerResult *);

  const SherpaOnnxOfflineSpeakerDiarization *(*diarization_create)(
      const SherpaOnnxOfflineSpeakerDiarizationConfig *);
  void (*diarization_destroy)(const SherpaOnnxOfflineSpeakerDiarization *);
  const SherpaOnnxOfflineSpeakerDiarizationResult *(*diarization_process)(
      const SherpaOnnxOfflineSpeakerDiarization *, const float *, int32_t);
  int32_t (*diarization_num_speakers)(
      const SherpaOnnxOfflineSpeakerDiarizationResult *);
  int32_t (*diarization_num_segments)(
      const SherpaOnnxOfflineSpeakerDiarizationResult *);
  const SherpaOnnxOfflineSpeakerDiarizationSegment *(*diarization_sorted)(
      const SherpaOnnxOfflineSpeakerDiarizationResult *);
  void (*diarization_destroy_result)(
      const SherpaOnnxOfflineSpeakerDiarizationResult *);
  void (*diarization_destroy_segments)(
      const SherpaOnnxOfflineSpeakerDiarizationSegment *);

  const SherpaOnnxSpeakerEmbeddingExtractor *(*embedding_create)(
      const SherpaOnnxSpeakerEmbeddingExtractorConfig *);
  void (*embedding_destroy)(const SherpaOnnxSpeakerEmbeddingExtractor *);
  int32_t (*embedding_dim)(const SherpaOnnxSpeakerEmbeddingExtractor *);
  const SherpaOnnxOnlineStream *(*embedding_stream)(
      const SherpaOnnxSpeakerEmbeddingExtractor *);
  int32_t (*embedding_ready)(const SherpaOnnxSpeakerEmbeddingExtractor *,
                             const SherpaOnnxOnlineStream *);
  const float *(*embedding_compute)(const SherpaOnnxSpeakerEmbeddingExtractor *,
                                    const SherpaOnnxOnlineStream *);
  void (*embedding_free)(const float *);

} api_t;

static api_t *g_api;
static char g_error[1024];

static void set_error(const char *msg, const char *detail) {
  if (detail && detail[0]) {
    snprintf(g_error, sizeof(g_error), "%s: %s", msg, detail);
  } else {
    snprintf(g_error, sizeof(g_error), "%s", msg);
  }
}

static int load_lib(const char *path, shim_lib *out) {
#if defined(_WIN32)
  // A UTF-8 path becomes UTF-16 here, so a library or model may live under any
  // name. The library's dependencies (onnxruntime) sit beside it, so its own
  // folder is searched for them; PATH never is.
  wchar_t wide[32768];
  if (MultiByteToWideChar(CP_UTF8, 0, path, -1, wide, 32768) <= 0) return 0;
  *out = LoadLibraryExW(wide, NULL, LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR |
                                       LOAD_LIBRARY_SEARCH_DEFAULT_DIRS);
  if (!*out) *out = LoadLibraryExW(wide, NULL, 8);  // loaders without flags
#else
  *out = dlopen(path, RTLD_NOW | RTLD_LOCAL);
#endif
  return *out != NULL;
}

static void *sym(shim_lib lib, const char *name) {
#if defined(_WIN32)
  return (void *)GetProcAddress(lib, name);
#else
  return dlsym(lib, name);
#endif
}

// sherpa_api_load loads the library at path (UTF-8) and resolves the function
// table. The library stays loaded for the life of the process; recognizers and
// streams are still freed one owner at a time.
const api_t *sherpa_api_load(const char *path) {
  shim_lib lib;
  api_t *a;
  if (g_api) return g_api;
  if (!path || !path[0]) {
    set_error("no sherpa-onnx library path", NULL);
    return NULL;
  }
  if (!load_lib(path, &lib)) {
    set_error("cannot load the sherpa-onnx library", path);
    return NULL;
  }
  a = (api_t *)calloc(1, sizeof(api_t));
  if (!a) {
    set_error("out of memory loading the sherpa-onnx library", NULL);
    return NULL;
  }
#define RESOLVE(field, name)                                    \
  do {                                                          \
    *(void **)(&a->field) = sym(lib, name);                     \
    if (!a->field) {                                            \
      set_error("missing sherpa-onnx entry point " name, path); \
      free(a);                                                  \
      return NULL;                                              \
    }                                                           \
  } while (0)
  RESOLVE(version, "SherpaOnnxGetVersionStr");
  RESOLVE(online_create, "SherpaOnnxCreateOnlineRecognizer");
  RESOLVE(online_destroy, "SherpaOnnxDestroyOnlineRecognizer");
  RESOLVE(online_stream_create, "SherpaOnnxCreateOnlineStream");
  RESOLVE(online_stream_destroy, "SherpaOnnxDestroyOnlineStream");
  RESOLVE(online_stream_accept, "SherpaOnnxOnlineStreamAcceptWaveform");
  RESOLVE(online_stream_ready, "SherpaOnnxIsOnlineStreamReady");
  RESOLVE(online_stream_decode, "SherpaOnnxDecodeOnlineStream");
  RESOLVE(online_stream_result, "SherpaOnnxGetOnlineStreamResult");
  RESOLVE(online_result_destroy, "SherpaOnnxDestroyOnlineRecognizerResult");
  RESOLVE(online_stream_endpoint, "SherpaOnnxOnlineStreamIsEndpoint");
  RESOLVE(online_stream_reset, "SherpaOnnxOnlineStreamReset");
  RESOLVE(online_stream_finish, "SherpaOnnxOnlineStreamInputFinished");
  RESOLVE(offline_create, "SherpaOnnxCreateOfflineRecognizer");
  RESOLVE(offline_destroy, "SherpaOnnxDestroyOfflineRecognizer");
  RESOLVE(offline_stream_create, "SherpaOnnxCreateOfflineStream");
  RESOLVE(offline_stream_destroy, "SherpaOnnxDestroyOfflineStream");
  RESOLVE(offline_stream_accept, "SherpaOnnxAcceptWaveformOffline");
  RESOLVE(offline_stream_decode, "SherpaOnnxDecodeOfflineStream");
  RESOLVE(offline_stream_result, "SherpaOnnxGetOfflineStreamResult");
  RESOLVE(offline_result_destroy, "SherpaOnnxDestroyOfflineRecognizerResult");
  RESOLVE(diarization_create, "SherpaOnnxCreateOfflineSpeakerDiarization");
  RESOLVE(diarization_destroy, "SherpaOnnxDestroyOfflineSpeakerDiarization");
  RESOLVE(diarization_process, "SherpaOnnxOfflineSpeakerDiarizationProcess");
  RESOLVE(diarization_num_speakers, "SherpaOnnxOfflineSpeakerDiarizationResultGetNumSpeakers");
  RESOLVE(diarization_num_segments, "SherpaOnnxOfflineSpeakerDiarizationResultGetNumSegments");
  RESOLVE(diarization_sorted, "SherpaOnnxOfflineSpeakerDiarizationResultSortByStartTime");
  RESOLVE(diarization_destroy_result, "SherpaOnnxOfflineSpeakerDiarizationDestroyResult");
  RESOLVE(diarization_destroy_segments, "SherpaOnnxOfflineSpeakerDiarizationDestroySegment");
  RESOLVE(embedding_create, "SherpaOnnxCreateSpeakerEmbeddingExtractor");
  RESOLVE(embedding_destroy, "SherpaOnnxDestroySpeakerEmbeddingExtractor");
  RESOLVE(embedding_dim, "SherpaOnnxSpeakerEmbeddingExtractorDim");
  RESOLVE(embedding_stream, "SherpaOnnxSpeakerEmbeddingExtractorCreateStream");
  RESOLVE(embedding_ready, "SherpaOnnxSpeakerEmbeddingExtractorIsReady");
  RESOLVE(embedding_compute, "SherpaOnnxSpeakerEmbeddingExtractorComputeEmbedding");
  RESOLVE(embedding_free, "SherpaOnnxSpeakerEmbeddingExtractorDestroyEmbedding");
#undef RESOLVE
  g_api = a;
  return g_api;
}

const char *sherpa_api_error(void) { return g_error; }

// The version of the loaded library, or "" when it is not loaded.
const char *sherpa_api_version(void) {
  if (!g_api || !g_api->version) return "";
  return g_api->version();
}

// ------------------------------------------------------------------ online

const void *sherpa_online_create(const char *tokens, const char *encoder,
                                 const char *decoder, const char *joiner,
                                 int32_t threads, int32_t endpoint,
                                 float silence1, float silence2,
                                 float max_utterance) {
  SherpaOnnxOnlineRecognizerConfig c;
  if (!g_api) return NULL;
  memset(&c, 0, sizeof(c));
  c.feat_config.sample_rate = 16000;
  c.feat_config.feature_dim = 80;
  c.model_config.transducer.encoder = encoder;
  c.model_config.transducer.decoder = decoder;
  c.model_config.transducer.joiner = joiner;
  c.model_config.tokens = tokens;
  c.model_config.num_threads = threads;
  c.model_config.provider = "cpu";
  c.decoding_method = "greedy_search";
  c.max_active_paths = 4;
  c.enable_endpoint = endpoint;
  c.rule1_min_trailing_silence = silence1;
  c.rule2_min_trailing_silence = silence2;
  c.rule3_min_utterance_length = max_utterance;
  return g_api->online_create(&c);
}

void sherpa_online_destroy(const void *r) {
  if (g_api && r) g_api->online_destroy(r);
}

const void *sherpa_online_stream_create(const void *r) {
  if (!g_api || !r) return NULL;
  return g_api->online_stream_create(r);
}

void sherpa_online_stream_destroy(const void *s) {
  if (g_api && s) g_api->online_stream_destroy(s);
}

void sherpa_online_stream_accept(const void *s, int32_t sample_rate,
                                 const float *samples, int32_t n) {
  if (g_api && s) g_api->online_stream_accept(s, sample_rate, samples, n);
}

int32_t sherpa_online_stream_ready(const void *r, const void *s) {
  if (!g_api || !r || !s) return 0;
  return g_api->online_stream_ready(r, s);
}

void sherpa_online_stream_decode(const void *r, const void *s) {
  if (g_api && r && s) g_api->online_stream_decode(r, s);
}

int32_t sherpa_online_stream_endpoint(const void *r, const void *s) {
  if (!g_api || !r || !s) return 0;
  return g_api->online_stream_endpoint(r, s);
}

void sherpa_online_stream_reset(const void *r, const void *s) {
  if (g_api && r && s) g_api->online_stream_reset(r, s);
}

void sherpa_online_stream_finish(const void *s) {
  if (g_api && s) g_api->online_stream_finish(s);
}

// sherpa_online_result copies the stream's current result out of native
// memory: the recognized text into text (NUL terminated, truncated to
// text_len-1 bytes; the length the text needs is returned, so truncation is
// visible) and, when tokens is not NULL, the tokens and timestamps into
// malloc'd blocks freed with sherpa_free. The native result is destroyed here,
// so no pointer into library memory reaches Go.
static int32_t copy_result(const char *src, int32_t n,
                           const char *const *token_strings,
                           const float *timestamps, char *text,
                           int32_t text_len, char ***tokens, float **stamps,
                           int32_t *count) {
  int32_t i, need;
  size_t blobs = 0, off = 0, slot;
  char **out;
  char *data;
  if (tokens) *tokens = NULL;
  if (stamps) *stamps = NULL;
  if (count) *count = 0;
  if (text && text_len > 0) text[0] = 0;
  if (!src) src = "";
  need = (int32_t)strlen(src);
  if (text && text_len > 0) {
    strncpy(text, src, (size_t)text_len - 1);
    text[text_len - 1] = 0;
  }
  if (tokens) {
    slot = (size_t)(n > 0 ? n : 1);
    for (i = 0; i < n; i++) {
      const char *tok = token_strings && token_strings[i] ? token_strings[i] : "";
      blobs += strlen(tok) + 1;
    }
    out = (char **)malloc(sizeof(char *) * slot + blobs);
    if (out) {
      data = (char *)(out + slot);
      for (i = 0; i < n; i++) {
        const char *tok = token_strings && token_strings[i] ? token_strings[i] : "";
        size_t len = strlen(tok) + 1;
        memcpy(data + off, tok, len);
        out[i] = data + off;
        off += len;
      }
      *tokens = out;
    }
    if (n > 0 && timestamps && stamps) {
      float *ts = (float *)malloc(sizeof(float) * (size_t)n);
      if (ts) {
        memcpy(ts, timestamps, sizeof(float) * (size_t)n);
        *stamps = ts;
      }
    }
    if (count) *count = n;
  }
  return need;
}

int32_t sherpa_online_result(const void *r, const void *s, char *text,
                             int32_t text_len, char ***tokens, float **stamps,
                             int32_t *count) {
  const SherpaOnnxOnlineRecognizerResult *res;
  int32_t need;
  if (!g_api || !r || !s) return 0;
  res = g_api->online_stream_result(r, s);
  if (!res) return 0;
  need = copy_result(res->text, res->count, res->tokens_arr, res->timestamps,
                     text, text_len, tokens, stamps, count);
  g_api->online_result_destroy(res);
  return need;
}

// ------------------------------------------------------------------ offline

const void *sherpa_offline_create(int32_t kind, const char **p,
                                  int32_t threads) {
  SherpaOnnxOfflineRecognizerConfig c;
  if (!g_api || !p) return NULL;
  memset(&c, 0, sizeof(c));
  c.feat_config.sample_rate = 16000;
  c.feat_config.feature_dim = 80;
  c.model_config.num_threads = threads;
  c.model_config.provider = "cpu";
  c.decoding_method = "greedy_search";
  c.max_active_paths = 4;
  switch (kind) {
  case 0:
    c.model_config.tokens = p[0];
    c.model_config.sense_voice.model = p[1];
    c.model_config.sense_voice.language = "auto";
    c.model_config.sense_voice.use_itn = 1;
    break;
  case 1:
    c.model_config.funasr_nano.encoder_adaptor = p[2];
    c.model_config.funasr_nano.llm = p[3];
    c.model_config.funasr_nano.embedding = p[4];
    c.model_config.funasr_nano.tokenizer = p[5];
    c.model_config.funasr_nano.max_new_tokens = 256;
    c.model_config.funasr_nano.temperature = 0.2f;
    c.model_config.funasr_nano.top_p = 0.8f;
    c.model_config.funasr_nano.seed = 42;
    c.model_config.funasr_nano.language = "auto";
    c.model_config.funasr_nano.itn = 1;
    break;
  case 2:
    c.model_config.qwen3_asr.conv_frontend = p[6];
    c.model_config.qwen3_asr.encoder = p[7];
    c.model_config.qwen3_asr.decoder = p[8];
    c.model_config.qwen3_asr.tokenizer = p[5];
    c.model_config.qwen3_asr.max_total_len = 32768;
    c.model_config.qwen3_asr.max_new_tokens = 256;
    c.model_config.qwen3_asr.temperature = 0.2f;
    c.model_config.qwen3_asr.top_p = 0.8f;
    c.model_config.qwen3_asr.seed = 42;
    break;
  default:
    return NULL;
  }
  return g_api->offline_create(&c);
}

void sherpa_offline_destroy(const void *r) {
  if (g_api && r) g_api->offline_destroy(r);
}

const void *sherpa_offline_stream_create(const void *r) {
  if (!g_api || !r) return NULL;
  return g_api->offline_stream_create(r);
}

void sherpa_offline_stream_destroy(const void *s) {
  if (g_api && s) g_api->offline_stream_destroy(s);
}

void sherpa_offline_stream_accept(const void *s, int32_t sample_rate,
                                  const float *samples, int32_t n) {
  if (g_api && s) g_api->offline_stream_accept(s, sample_rate, samples, n);
}

void sherpa_offline_stream_decode(const void *r, const void *s) {
  if (g_api && r && s) g_api->offline_stream_decode(r, s);
}

int32_t sherpa_offline_result(const void *s, char *text, int32_t text_len,
                              char ***tokens, float **stamps, int32_t *count) {
  const SherpaOnnxOfflineRecognizerResult *res;
  int32_t need;
  if (!g_api || !s) return 0;
  res = g_api->offline_stream_result(s);
  if (!res) return 0;
  need = copy_result(res->text, res->count, res->tokens_arr, res->timestamps,
                     text, text_len, tokens, stamps, count);
  g_api->offline_result_destroy(res);
  return need;
}

// sherpa_diarization_process returns a malloc'd, start-time-sorted segment
// array. The caller reads it once and frees it with sherpa_free.
const void *sherpa_diarization_process(const char *segmentation,
                                       const char *embedding,
                                       const float *samples, int32_t n,
                                       int32_t *out_count) {
  SherpaOnnxOfflineSpeakerDiarizationConfig c;
  const SherpaOnnxOfflineSpeakerDiarization *sd;
  const SherpaOnnxOfflineSpeakerDiarizationResult *res;
  const SherpaOnnxOfflineSpeakerDiarizationSegment *segments;
  OttrDiarizationSegment *out;
  int32_t i, count;
  if (out_count) *out_count = 0;
  if (!g_api || !samples || n <= 0) return NULL;
  memset(&c, 0, sizeof(c));
  c.segmentation.pyannote.model = segmentation;
  c.segmentation.num_threads = 2;
  c.segmentation.provider = "cpu";
  c.embedding.model = embedding;
  c.embedding.num_threads = 2;
  c.embedding.provider = "cpu";
  c.clustering.num_clusters = 0;
  c.clustering.threshold = 0.5f;
  c.min_duration_on = 0.3f;
  c.min_duration_off = 0.5f;
  sd = g_api->diarization_create(&c);
  if (!sd) return NULL;
  res = g_api->diarization_process(sd, samples, n);
  if (!res) {
    g_api->diarization_destroy(sd);
    return NULL;
  }
  count = g_api->diarization_num_segments(res);
  segments = g_api->diarization_sorted(res);
  out = (OttrDiarizationSegment *)malloc(sizeof(OttrDiarizationSegment) * (size_t)(count > 0 ? count : 1));
  if (out) {
    for (i = 0; i < count; i++) {
      out[i].start = segments[i].start;
      out[i].end = segments[i].end;
      out[i].speaker = segments[i].speaker;
    }
  }
  if (segments) g_api->diarization_destroy_segments(segments);
  if (out_count) *out_count = out ? count : 0;
  g_api->diarization_destroy_result(res);
  g_api->diarization_destroy(sd);
  return out;
}

const float *sherpa_speaker_embedding(const char *model,
                                      const float *samples, int32_t n,
                                      int32_t *out_dim) {
  SherpaOnnxSpeakerEmbeddingExtractorConfig c;
  const SherpaOnnxSpeakerEmbeddingExtractor *ex;
  const SherpaOnnxOnlineStream *stream;
  const float *computed;
  float *out;
  int32_t dim;
  if (out_dim) *out_dim = 0;
  if (!g_api || !samples || n <= 0) return NULL;
  memset(&c, 0, sizeof(c));
  c.model = model;
  c.num_threads = 2;
  c.provider = "cpu";
  ex = g_api->embedding_create(&c);
  if (!ex) return NULL;
  stream = g_api->embedding_stream(ex);
  if (!stream) {
    g_api->embedding_destroy(ex);
    return NULL;
  }
  g_api->online_stream_accept(stream, 16000, samples, n);
  g_api->online_stream_finish(stream);
  computed = NULL;
  if (g_api->embedding_ready(ex, stream))
    computed = g_api->embedding_compute(ex, stream);
  dim = g_api->embedding_dim(ex);
  out = NULL;
  if (computed && dim > 0) {
    out = (float *)malloc(sizeof(float) * (size_t)dim);
    if (out) {
      memcpy(out, computed, sizeof(float) * (size_t)dim);
      if (out_dim) *out_dim = dim;
    }
  }
  if (computed) g_api->embedding_free(computed);
  g_api->online_stream_destroy(stream);
  g_api->embedding_destroy(ex);
  return out;
}

void sherpa_free(void *p) { free(p); }
