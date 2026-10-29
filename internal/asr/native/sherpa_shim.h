// sherpa_shim.h is the C ABI between Go and sherpa_shim.c. Handles are opaque
// voids: nothing here names a sherpa-onnx type, so Go never includes its
// headers.
#ifndef OTTR_SHERPA_SHIM_H_
#define OTTR_SHERPA_SHIM_H_

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct api_t api_t;

// sherpa_api_load loads the library at path (UTF-8, also on Windows) and
// resolves its entry points. It returns NULL on failure, and sherpa_api_error
// says why. Once loaded the library stays for the life of the process.
const api_t *sherpa_api_load(const char *path);
const char *sherpa_api_error(void);
const char *sherpa_api_version(void);

// --- streaming recognition, used for both batch and live audio
const void *sherpa_online_create(const char *tokens, const char *encoder,
                                 const char *decoder, const char *joiner,
                                 int32_t num_threads, int32_t enable_endpoint,
                                 float rule1_min_trailing_silence,
                                 float rule2_min_trailing_silence,
                                 float rule3_min_utterance_length);
void sherpa_online_destroy(const void *recognizer);
const void *sherpa_online_stream_create(const void *recognizer);
void sherpa_online_stream_destroy(const void *stream);
void sherpa_online_stream_accept(const void *stream, int32_t rate,
                                 const float *samples, int32_t n);
int32_t sherpa_online_stream_ready(const void *recognizer, const void *stream);
void sherpa_online_stream_decode(const void *recognizer, const void *stream);
int32_t sherpa_online_stream_endpoint(const void *recognizer,
                                      const void *stream);
void sherpa_online_stream_reset(const void *recognizer, const void *stream);
void sherpa_online_stream_finish(const void *stream);
// Copies the stream's current result out: the text into text (NUL terminated,
// truncated to text_len-1 bytes; the length it needs is returned) and, when
// tokens is not NULL, its tokens and timestamps into malloc'd blocks freed with
// sherpa_free (*stamps may be NULL when the model gives no timestamps).
int32_t sherpa_online_result(const void *recognizer, const void *stream,
                             char *text, int32_t text_len, char ***tokens,
                             float **stamps, int32_t *count);

void sherpa_free(void *p);

#ifdef __cplusplus
}
#endif

#endif  // OTTR_SHERPA_SHIM_H_
