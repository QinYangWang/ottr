#define INITGUID
#define COBJMACROS
#define WIDL_using_Windows_Foundation
#define WIDL_using_Windows_Graphics
#define WIDL_using_Windows_Graphics_DirectX
#define WIDL_using_Windows_Graphics_DirectX_Direct3D11
#define WIDL_using_Windows_Graphics_Capture
#include "wgc.h"

#include <windows.h>
#include <d3d11.h>
#include <dwmapi.h>
#include <dxgi.h>
#include <roapi.h>
#include <winstring.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "windows.graphics.capture.h"
#include "windows.graphics.capture.interop.h"
#include "windows.graphics.directx.h"
#include "windows.graphics.directx.direct3d11.h"

HRESULT WINAPI CreateDirect3D11DeviceFromDXGIDevice(IDXGIDevice *d, IInspectable **g);

static const GUID IID_DxgiAccess = {
	0xA9B3D012, 0x3DF2, 0x4EE3, {0xB8, 0xD1, 0x86, 0x95, 0xF4, 0x57, 0xD3, 0xC1}};

/* IClosable::Close is slot 6: three IUnknown and three IInspectable methods. */
typedef struct {
	void *query_interface, *add_ref, *release;
	void *get_iids, *get_class_name, *get_trust_level;
	HRESULT(STDMETHODCALLTYPE *close_method)(void *);
} ClosableVtbl;

/* IDirect3DDxgiInterfaceAccess::GetInterface is slot 3. */
typedef struct {
	void *query_interface, *add_ref, *release;
	HRESULT(STDMETHODCALLTYPE *get_interface)(void *, const GUID *, void **);
} AccessVtbl;

typedef struct Stream Stream;

typedef struct Handler {
	void *vtbl;
	LONG refs;
	Stream *stream;
} Handler;

struct Stream {
	CRITICAL_SECTION lock;
	int closing;

	__x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureItem *item;
	__x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFramePool *pool;
	__x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureSession *session;
	EventRegistrationToken token;
	Handler *handler;
	IInspectable *wrt_device;
	ID3D11Device *device;
	ID3D11DeviceContext *context;
	ID3D11Texture2D *staging;

	ottr_frame_cb fcb;
	uintptr_t ctx;
	int width, height;
};

static void handler_free(Handler *h);

static HRESULT STDMETHODCALLTYPE h_qi(void *t, REFIID riid, void **ppv) {
	Handler *h = (Handler *)t;
	if (IsEqualIID(riid, &IID_IUnknown) || IsEqualIID(riid, &IID_IInspectable) ||
		IsEqualIID(riid, &IID_IAgileObject)) {
		*ppv = h;
		InterlockedIncrement(&h->refs);
		return S_OK;
	}
	*ppv = NULL;
	return E_NOINTERFACE;
}

static ULONG STDMETHODCALLTYPE h_addref(void *t) {
	return (ULONG)InterlockedIncrement(&((Handler *)t)->refs);
}

static ULONG STDMETHODCALLTYPE h_release(void *t) {
	Handler *h = (Handler *)t;
	ULONG left = (ULONG)InterlockedDecrement(&h->refs);
	if (left == 0) handler_free(h);
	return left;
}

static HRESULT STDMETHODCALLTYPE h_invoke(void *t,
	__x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFramePool *sender, IInspectable *args);

static __FITypedEventHandler_2_Windows__CGraphics__CCapture__CDirect3D11CaptureFramePool_IInspectableVtbl
	handler_vtbl = {(void *)h_qi, (void *)h_addref, (void *)h_release, (void *)h_invoke};

static void handler_free(Handler *h) { free(h); }

/* ---- helpers ---- */

static void set_err(char *err, int errlen, const char *what, HRESULT hr) {
	if (!err || errlen <= 0) return;
	snprintf(err, (size_t)errlen, "%s failed (0x%08lx)", what, (unsigned long)hr);
}

static void ensure_ro(void) {
	HRESULT hr = RoInitialize(RO_INIT_MULTITHREADED);
	(void)hr; /* S_FALSE or RPC_E_CHANGED_MODE: already in a partition */
}

static IInspectable *activation(const wchar_t *name) {
	HSTRING hs;
	IInspectable *f = NULL;
	if (FAILED(WindowsCreateString(name, (UINT32)wcslen(name), &hs))) return NULL;
	if (FAILED(RoGetActivationFactory(hs, &IID_IInspectable, (void **)&f))) f = NULL;
	WindowsDeleteString(hs);
	return f;
}

static void release(void *p) {
	if (p) ((IUnknown *)p)->lpVtbl->Release((IUnknown *)p);
}

static void close_object(void *p) {
	if (!p) return;
	((ClosableVtbl *)((IUnknown *)p)->lpVtbl)->close_method(p);
	release(p);
}

/* readback copies into a staging texture and hands Go the rows; the Go callback
 * copies them, so the map is released before it returns. */
static void readback(Stream *s, __x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFrame *frame,
	int w, int h, int64_t pts) {
	__x_ABI_CWindows_CGraphics_CDirectX_CDirect3D11_CIDirect3DSurface *surf = NULL;
	if (FAILED(IDirect3D11CaptureFrame_get_Surface(frame, &surf)) || !surf) return;

	void *access = NULL;
	if (FAILED(IUnknown_QueryInterface((IUnknown *)surf, &IID_DxgiAccess, &access)) || !access) {
		release(surf);
		return;
	}
	ID3D11Texture2D *tex = NULL;
	HRESULT hr = ((AccessVtbl *)((IUnknown *)access)->lpVtbl)->get_interface(access, &IID_ID3D11Texture2D,
		(void **)&tex);
	release(access);
	release(surf);
	if (FAILED(hr) || !tex) return;

	if (!s->staging) {
		D3D11_TEXTURE2D_DESC sd = {0};
		sd.Width = (UINT)w;
		sd.Height = (UINT)h;
		sd.MipLevels = 1;
		sd.ArraySize = 1;
		sd.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
		sd.SampleDesc.Count = 1;
		sd.Usage = D3D11_USAGE_STAGING;
		sd.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
		if (FAILED(ID3D11Device_CreateTexture2D(s->device, &sd, NULL, &s->staging))) {
			release(tex);
			return;
		}
	}

	ID3D11Resource *dst = (ID3D11Resource *)s->staging;
	ID3D11DeviceContext_CopyResource(s->context, dst, (ID3D11Resource *)tex);
	release(tex);

	D3D11_MAPPED_SUBRESOURCE map;
	if (FAILED(ID3D11DeviceContext_Map(s->context, dst, 0, D3D11_MAP_READ, 0, &map))) return;
	s->fcb(s->ctx, (const uint8_t *)map.pData, w, h, (int)map.RowPitch, pts);
	ID3D11DeviceContext_Unmap(s->context, dst, 0);
}

static HRESULT STDMETHODCALLTYPE h_invoke(void *t,
	__x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFramePool *sender, IInspectable *args) {
	(void)args;
	Stream *s = ((Handler *)t)->stream;
	if (!s) return S_OK;

	/* The lock is what Close waits on: no callback touches the device once
	 * closing is set. */
	EnterCriticalSection(&s->lock);
	if (s->closing) goto done;

	__x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFrame *frame = NULL;
	if (SUCCEEDED(IDirect3D11CaptureFramePool_TryGetNextFrame(sender, &frame)) && frame) {
		__x_ABI_CWindows_CGraphics_CSizeInt32 size = {0};
		IDirect3D11CaptureFrame_get_ContentSize(frame, &size);
		__x_ABI_CWindows_CFoundation_CTimeSpan span = {0};
		IDirect3D11CaptureFrame_get_SystemRelativeTime(frame, &span);
		int64_t pts = span.Duration * 100; /* TimeSpan ticks are 100 ns */

		if (size.Width != s->width || size.Height != s->height) {
			/* A resized source invalidates the pool's buffers: rebuild, and
			 * deliver nothing until a frame of the new size arrives. */
			IDirect3D11CaptureFramePool_Recreate(s->pool,
				(__x_ABI_CWindows_CGraphics_CDirectX_CDirect3D11_CIDirect3DDevice *)s->wrt_device,
				DirectXPixelFormat_B8G8R8A8UIntNormalized, 2, size);
			release(s->staging);
			s->staging = NULL;
			s->width = size.Width;
			s->height = size.Height;
		} else {
			readback(s, frame, size.Width, size.Height, pts);
		}
		IDirect3D11CaptureFrame_Release(frame);
	}
done:
	LeaveCriticalSection(&s->lock);
	return S_OK;
}

/* ---- sources ---- */

typedef struct MonitorList {
	HMONITOR *items;
	int count;
} MonitorList;

static BOOL CALLBACK collect_monitor(HMONITOR mon, HDC dc, LPRECT r, LPARAM lp) {
	(void)dc;
	(void)r;
	MonitorList *m = (MonitorList *)lp;
	HMONITOR *grown = (HMONITOR *)realloc(m->items, sizeof(HMONITOR) * (size_t)(m->count + 1));
	if (!grown) return FALSE;
	m->items = grown;
	m->items[m->count++] = mon;
	return TRUE;
}

typedef struct WindowCtx {
	ottr_source_cb cb;
	uintptr_t ctx;
} WindowCtx;

static char *utf8_of(const wchar_t *wide) {
	int need = WideCharToMultiByte(CP_UTF8, 0, wide, -1, NULL, 0, NULL, NULL);
	if (need <= 0) return NULL;
	char *out = (char *)malloc((size_t)need);
	if (out) WideCharToMultiByte(CP_UTF8, 0, wide, -1, out, need, NULL, NULL);
	return out;
}

static BOOL CALLBACK collect_window(HWND hwnd, LPARAM lp) {
	WindowCtx *wc = (WindowCtx *)lp;
	DWORD pid = 0;
	if (!IsWindowVisible(hwnd)) return TRUE;
	GetWindowThreadProcessId(hwnd, &pid);
	if (pid == GetCurrentProcessId()) return TRUE;

	/* Cloaked windows are closed UWP shells and windows of another virtual
	 * desktop: invisible in fact, though IsWindowVisible says otherwise. */
	DWORD cloaked = 0;
	if (SUCCEEDED(DwmGetWindowAttribute(hwnd, DWMWA_CLOAKED, &cloaked, sizeof(cloaked))) && cloaked) {
		return TRUE;
	}

	int len = GetWindowTextLengthW(hwnd);
	if (len <= 0) return TRUE;
	wchar_t *title = (wchar_t *)calloc((size_t)len + 1, sizeof(wchar_t));
	if (!title) return TRUE;
	GetWindowTextW(hwnd, title, len + 1);
	char *name = utf8_of(title);
	free(title);
	if (!name || name[0] == 0) {
		free(name);
		return TRUE;
	}

	RECT r = {0};
	int w = 0, h = 0;
	if (SUCCEEDED(DwmGetWindowAttribute(hwnd, DWMWA_EXTENDED_FRAME_BOUNDS, &r, sizeof(r))) ||
		GetWindowRect(hwnd, &r)) {
		w = (int)(r.right - r.left);
		h = (int)(r.bottom - r.top);
		if (w <= 0 || h <= 0) {
			w = 0; /* minimized: listed, but with no size to show */
			h = 0;
		}
	}

	char id[32];
	snprintf(id, sizeof(id), "window:%llx", (unsigned long long)(uintptr_t)hwnd);
	wc->cb(wc->ctx, id, "window", name, w, h);
	free(name);
	return TRUE;
}

int ottr_wgc_list(ottr_source_cb cb, uintptr_t ctx, char *err, int errlen) {
	ensure_ro();

	MonitorList monitors = {0};
	EnumDisplayMonitors(NULL, NULL, collect_monitor, (LPARAM)&monitors);
	if (monitors.count == 0) {
		if (err && errlen > 0) snprintf(err, (size_t)errlen, "no display is attached");
		return -1;
	}

	RECT box = {0, 0, 0, 0};
	for (int i = 0; i < monitors.count; i++) {
		MONITORINFO mi = {0};
		mi.cbSize = sizeof(mi);
		if (!GetMonitorInfo(monitors.items[i], &mi)) continue;
		if (i == 0 || mi.rcMonitor.left < box.left) box.left = mi.rcMonitor.left;
		if (i == 0 || mi.rcMonitor.top < box.top) box.top = mi.rcMonitor.top;
		if (i == 0 || mi.rcMonitor.right > box.right) box.right = mi.rcMonitor.right;
		if (i == 0 || mi.rcMonitor.bottom > box.bottom) box.bottom = mi.rcMonitor.bottom;
	}
	cb(ctx, "screen", "screen", "Full screen", (int)(box.right - box.left),
		(int)(box.bottom - box.top));

	for (int i = 0; i < monitors.count; i++) {
		MONITORINFO mi = {0};
		mi.cbSize = sizeof(mi);
		if (!GetMonitorInfo(monitors.items[i], &mi)) continue;
		char id[32], name[32];
		snprintf(id, sizeof(id), "display:%llx", (unsigned long long)(uintptr_t)monitors.items[i]);
		snprintf(name, sizeof(name), "Display %d", i + 1);
		cb(ctx, id, "display", name, (int)(mi.rcMonitor.right - mi.rcMonitor.left),
			(int)(mi.rcMonitor.bottom - mi.rcMonitor.top));
	}
	free(monitors.items);

	WindowCtx wc = {cb, ctx};
	EnumWindows(collect_window, (LPARAM)&wc);
	return 0;
}

/* ---- streams ---- */

static int parse_id(const char *id, HWND *hwnd, HMONITOR *monitor) {
	unsigned long long v = 0;
	*hwnd = NULL;
	*monitor = NULL;
	if (strcmp(id, "screen") == 0) {
		POINT pt = {0, 0};
		*monitor = MonitorFromPoint(pt, MONITOR_DEFAULTTOPRIMARY);
		return *monitor ? 0 : -1;
	}
	if (sscanf(id, "display:%llx", &v) == 1) {
		*monitor = (HMONITOR)(uintptr_t)v;
		return 0;
	}
	if (sscanf(id, "window:%llx", &v) == 1) {
		*hwnd = (HWND)(uintptr_t)v;
		return IsWindow(*hwnd) ? 0 : -1; /* closed since it was listed */
	}
	return -1;
}

void ottr_wgc_close(void *p);

void *ottr_wgc_open(const char *id, int cursor, ottr_frame_cb fcb, uintptr_t ctx,
	int *width, int *height, int *code, char *err, int errlen) {
	ensure_ro();

	HWND hwnd = NULL;
	HMONITOR monitor = NULL;
	*code = OTTR_ERR_SOURCE;
	if (parse_id(id, &hwnd, &monitor) != 0) {
		if (err && errlen > 0) snprintf(err, (size_t)errlen, "%s is not capturable any more", id);
		return NULL;
	}

	Stream *s = (Stream *)calloc(1, sizeof(Stream));
	if (!s) return NULL;
	InitializeCriticalSection(&s->lock);
	s->fcb = fcb;
	s->ctx = ctx;

	HRESULT hr = D3D11CreateDevice(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, D3D11_CREATE_DEVICE_BGRA_SUPPORT,
		NULL, 0, D3D11_SDK_VERSION, &s->device, NULL, &s->context);
	if (FAILED(hr)) {
		set_err(err, errlen, "D3D11CreateDevice", hr);
		goto fail;
	}
	IDXGIDevice *dxgi = NULL;
	if (FAILED(hr = ID3D11Device_QueryInterface(s->device, &IID_IDXGIDevice, (void **)&dxgi))) {
		set_err(err, errlen, "IDXGIDevice", hr);
		goto fail;
	}
	hr = CreateDirect3D11DeviceFromDXGIDevice(dxgi, &s->wrt_device);
	release(dxgi);
	if (FAILED(hr)) {
		set_err(err, errlen, "CreateDirect3D11DeviceFromDXGIDevice", hr);
		goto fail;
	}

	IInspectable *factory = activation(L"Windows.Graphics.Capture.GraphicsCaptureItem");
	IGraphicsCaptureItemInterop *interop = NULL;
	if (factory) {
		IUnknown_QueryInterface(factory, &IID_IGraphicsCaptureItemInterop, (void **)&interop);
		release(factory);
	}
	if (!interop) {
		if (err && errlen > 0) {
			snprintf(err, (size_t)errlen, "Windows Graphics Capture is not available");
		}
		goto fail;
	}
	if (hwnd) {
		hr = IGraphicsCaptureItemInterop_CreateForWindow(interop, hwnd,
			&IID___x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureItem, (void **)&s->item);
	} else {
		hr = IGraphicsCaptureItemInterop_CreateForMonitor(interop, monitor,
			&IID___x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureItem, (void **)&s->item);
	}
	release(interop);
	if (FAILED(hr) || !s->item) {
		set_err(err, errlen, "the capture item", hr);
		goto fail;
	}

	__x_ABI_CWindows_CGraphics_CSizeInt32 size = {0};
	IGraphicsCaptureItem_get_Size(s->item, &size);
	if (size.Width <= 0 || size.Height <= 0) {
		if (err && errlen > 0) snprintf(err, (size_t)errlen, "the source has no visible area");
		goto fail;
	}
	s->width = size.Width;
	s->height = size.Height;

	IInspectable *statics = activation(L"Windows.Graphics.Capture.Direct3D11CaptureFramePool");
	if (!statics) {
		if (err && errlen > 0) snprintf(err, (size_t)errlen, "the frame pool is not available");
		goto fail;
	}
	/* A free-threaded pool delivers frames without a dispatcher queue, which
	 * Go does not have; older Windows cannot do that. */
	__x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFramePoolStatics2 *statics2 = NULL;
	if (FAILED(IUnknown_QueryInterface(statics,
			&IID___x_ABI_CWindows_CGraphics_CCapture_CIDirect3D11CaptureFramePoolStatics2,
			(void **)&statics2)) ||
		!statics2) {
		release(statics);
		*code = OTTR_ERR_PERMISSION;
		if (err && errlen > 0) {
			snprintf(err, (size_t)errlen, "Windows Graphics Capture is too old to record here");
		}
		goto fail;
	}
	hr = IDirect3D11CaptureFramePoolStatics2_CreateFreeThreaded(statics2,
		(__x_ABI_CWindows_CGraphics_CDirectX_CDirect3D11_CIDirect3DDevice *)s->wrt_device,
		DirectXPixelFormat_B8G8R8A8UIntNormalized, 2, size, &s->pool);
	release(statics2);
	release(statics);
	if (FAILED(hr) || !s->pool) {
		set_err(err, errlen, "the frame pool", hr);
		goto fail;
	}

	Handler *h = (Handler *)calloc(1, sizeof(Handler));
	if (!h) goto fail;
	h->vtbl = &handler_vtbl;
	h->refs = 1;
	h->stream = s;
	s->handler = h;
	hr = IDirect3D11CaptureFramePool_add_FrameArrived(s->pool, (void *)h, &s->token);
	if (FAILED(hr)) {
		set_err(err, errlen, "add_FrameArrived", hr);
		goto fail;
	}

	hr = IDirect3D11CaptureFramePool_CreateCaptureSession(s->pool, s->item, &s->session);
	if (FAILED(hr) || !s->session) {
		set_err(err, errlen, "CreateCaptureSession", hr);
		goto fail;
	}

	__x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureSession2 *s2 = NULL;
	if (SUCCEEDED(IUnknown_QueryInterface(s->session,
			&IID___x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureSession2, (void **)&s2)) &&
		s2) {
		IGraphicsCaptureSession2_put_IsCursorCaptureEnabled(s2, (unsigned char)(cursor ? 1 : 0));
		release(s2);
	}
	/* The capture border is a yellow frame around what is recorded; ask for
	 * none where Windows lets it go. */
	__x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureSession3 *s3 = NULL;
	if (SUCCEEDED(IUnknown_QueryInterface(s->session,
			&IID___x_ABI_CWindows_CGraphics_CCapture_CIGraphicsCaptureSession3, (void **)&s3)) &&
		s3) {
		IGraphicsCaptureSession3_put_IsBorderRequired(s3, 0);
		release(s3);
	}

	hr = IGraphicsCaptureSession_StartCapture(s->session);
	if (FAILED(hr)) {
		set_err(err, errlen, "StartCapture", hr);
		goto fail;
	}

	*width = s->width;
	*height = s->height;
	return s;

fail:
	ottr_wgc_close(s);
	return NULL;
}

int ottr_wgc_alive(const char *id) {
	HWND hwnd = NULL;
	HMONITOR monitor = NULL;
	if (parse_id(id, &hwnd, &monitor) != 0) return 0;
	if (!hwnd) {
		MONITORINFO mi = {0};
		mi.cbSize = sizeof(mi);
		return GetMonitorInfo(monitor, &mi) ? 1 : 0;
	}
	/* A window whose title went empty is a shell left behind by its app. */
	if (IsIconic(hwnd)) return 1;
	return IsWindowVisible(hwnd) && GetWindowTextLengthW(hwnd) > 0;
}

void ottr_wgc_close(void *p) {
	Stream *s = (Stream *)p;
	if (!s) return;

	/* Set the flag, then take the lock once: an in-flight callback is done
	 * when this returns, and no later one does any work. */
	EnterCriticalSection(&s->lock);
	s->closing = 1;
	LeaveCriticalSection(&s->lock);
	EnterCriticalSection(&s->lock);
	LeaveCriticalSection(&s->lock);

	if (s->pool && s->handler) IDirect3D11CaptureFramePool_remove_FrameArrived(s->pool, s->token);
	if (s->session) close_object(s->session);
	release(s->pool);
	release(s->item);
	release(s->staging);
	release(s->context);
	release(s->device);
	release(s->wrt_device);
	if (s->handler) h_release(s->handler);
	DeleteCriticalSection(&s->lock);
	free(s);
}

int ottr_wgc_init(void) {
	ensure_ro();
	return 0;
}
