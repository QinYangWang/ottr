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
int32_t sherpa_online_result(const void *r, const void *s, char *text,
                             int32_t text_len, char ***tokens, float **stamps,
                             int32_t *count) {
  const SherpaOnnxOnlineRecognizerResult *res;
  const char *src;
  int32_t i, n, need;
  size_t blobs = 0, off = 0, slot;
  char **out;
  char *data;
  if (tokens) *tokens = NULL;
  if (stamps) *stamps = NULL;
  if (count) *count = 0;
  if (text && text_len > 0) text[0] = 0;
  if (!g_api || !r || !s) return 0;
  res = g_api->online_stream_result(r, s);
  if (!res) return 0;
  src = res->text ? res->text : "";
  need = (int32_t)strlen(src);
  if (text && text_len > 0) {
    strncpy(text, src, (size_t)text_len - 1);
    text[text_len - 1] = 0;
  }
  n = res->count;
  if (tokens) {
    slot = (size_t)(n > 0 ? n : 1);
    for (i = 0; i < n; i++) {
      const char *tok =
          res->tokens_arr && res->tokens_arr[i] ? res->tokens_arr[i] : "";
      blobs += strlen(tok) + 1;
    }
    out = (char **)malloc(sizeof(char *) * slot + blobs);
    if (out) {
      data = (char *)(out + slot);
      for (i = 0; i < n; i++) {
        const char *tok =
            res->tokens_arr && res->tokens_arr[i] ? res->tokens_arr[i] : "";
        size_t len = strlen(tok) + 1;
        memcpy(data + off, tok, len);
        out[i] = data + off;
        off += len;
      }
      *tokens = out;
    }
    if (n > 0 && res->timestamps && stamps) {
      float *ts = (float *)malloc(sizeof(float) * (size_t)n);
      if (ts) {
        memcpy(ts, res->timestamps, sizeof(float) * (size_t)n);
        *stamps = ts;
      }
    }
    if (count) *count = n;
  }
  g_api->online_result_destroy(res);
  return need;
}

void sherpa_free(void *p) { free(p); }
