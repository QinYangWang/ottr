/* Ottr's Windows capture bridge: a small C ABI over Windows Graphics Capture
 * and D3D11. Go passes callbacks and an opaque handle, and receives tightly
 * packed BGRA rows with a monotonic timestamp; no C++ or WinRT type crosses.
 */
#ifndef OTTR_WGC_H
#define OTTR_WGC_H

#include <stdint.h>

/* Why a stream could not be opened, for capture's typed errors. */
#define OTTR_ERR_PERMISSION 1
#define OTTR_ERR_SOURCE 2
#define OTTR_ERR_OTHER 3

/* A source is "screen", "display:<monitor handle>" or "window:<hwnd>", the
 * last two in hex: the platform's own identity, not a title or an index. */
typedef void (*ottr_source_cb)(uintptr_t ctx, const char *id, const char *kind,
	const char *name, int width, int height);
typedef void (*ottr_frame_cb)(uintptr_t ctx, const uint8_t *bgra, int width,
	int height, int stride, int64_t pts_ns);

int ottr_wgc_list(ottr_source_cb cb, uintptr_t ctx, char *err, int errlen);

void *ottr_wgc_open(const char *id, int cursor, ottr_frame_cb fcb, uintptr_t ctx,
	int *width, int *height, int *code, char *err, int errlen);

/* ottr_wgc_alive reports whether the source an id names still exists: a
 * closed window never ends its stream by itself. */
int ottr_wgc_alive(const char *id);

/* ottr_wgc_close stops the stream; when it returns no callback runs again. */
void ottr_wgc_close(void *stream);

#endif
