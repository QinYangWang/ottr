//go:build windows

package capture

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// audioStream is one WASAPI capture (system loopback or microphone)
// writing f32le PCM to a named pipe that ffmpeg reads.
type audioStream struct {
	loopback bool
	rate     int
	channels int
	pipe     string
	stop     func()
}

var pipeSeq atomic.Int64

const audclntBufferflagsSilent = 0x2

// startAudioStream opens the default render (loopback) or capture endpoint
// and starts streaming its mix format PCM. The stream is live once nil is
// returned; ffmpeg connects to the pipe when its process starts.
func startAudioStream(loopback bool, sink *liveFeed) (*audioStream, error) {
	name := fmt.Sprintf(`\\.\pipe\ottr-audio-%d-%d`, os.Getpid(), pipeSeq.Add(1))
	ln, err := winio.ListenPipe(name, nil)
	if err != nil {
		return nil, err
	}
	as := &audioStream{pipe: name, loopback: loopback}
	ready := make(chan error, 1)
	done := make(chan struct{})
	var once atomic.Bool
	as.stop = func() {
		if once.CompareAndSwap(false, true) {
			close(done)
		}
		ln.Close()
	}

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()

		var de *wca.IMMDeviceEnumerator
		if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
			ready <- fmt.Errorf("device enumerator: %w", err)
			return
		}
		defer de.Release()
		flow := uint32(wca.ERender)
		if !as.loopback {
			flow = wca.ECapture
		}
		var mmd *wca.IMMDevice
		if err := de.GetDefaultAudioEndpoint(flow, wca.EConsole, &mmd); err != nil {
			ready <- fmt.Errorf("no default audio endpoint: %w", err)
			return
		}
		defer mmd.Release()
		var ac *wca.IAudioClient
		if err := mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
			ready <- fmt.Errorf("activate audio client: %w", err)
			return
		}
		defer ac.Release()
		var wfx *wca.WAVEFORMATEX
		if err := ac.GetMixFormat(&wfx); err != nil {
			ready <- fmt.Errorf("mix format: %w", err)
			return
		}
		var flags uint32
		if as.loopback {
			flags = wca.AUDCLNT_STREAMFLAGS_LOOPBACK
		}
		if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, flags, wca.REFERENCE_TIME(200*time.Millisecond/100), 0, wfx, nil); err != nil {
			ready <- fmt.Errorf("initialize: %w", err)
			return
		}
		var acc *wca.IAudioCaptureClient
		if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
			ready <- fmt.Errorf("capture client: %w", err)
			return
		}
		defer acc.Release()
		if err := ac.Start(); err != nil {
			ready <- fmt.Errorf("start: %w", err)
			return
		}
		defer ac.Stop()
		as.rate = int(wfx.NSamplesPerSec)
		as.channels = int(wfx.NChannels)
		block := int(wfx.NBlockAlign)
		ready <- nil

		// Wait for ffmpeg to connect, then pump packets to the pipe.
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			select {
			case <-done:
				return
			default:
			}
			var packet uint32
			if err := acc.GetNextPacketSize(&packet); err != nil {
				return // device unplugged or session ended
			}
			if packet == 0 {
				if as.loopback {
					// Windows WASAPI loopback produces no packets when the audio engine is idle;
					// write silence to keep the pipe fed and synchronized.
					silenceFrames := as.rate / 50 // 20ms
					silenceBytes := make([]byte, silenceFrames*block)
					if _, err := conn.Write(silenceBytes); err != nil {
						return
					}
					sink.write(silenceBytes, as.rate, as.channels)
					time.Sleep(20 * time.Millisecond)
					continue
				}
				time.Sleep(5 * time.Millisecond)
				continue
			}
			var data *byte
			var frames, bflags uint32
			var devPos, qpcPos uint64
			if err := acc.GetBuffer(&data, &frames, &bflags, &devPos, &qpcPos); err != nil {
				return
			}
			if frames > 0 {
				n := int(frames) * block
				var b []byte
				if bflags&audclntBufferflagsSilent != 0 {
					b = make([]byte, n)
				} else {
					b = unsafe.Slice(data, n)
				}
				if _, err := conn.Write(b); err != nil {
					acc.ReleaseBuffer(frames)
					return
				}
				if bflags&audclntBufferflagsSilent == 0 {
					sink.write(b, as.rate, as.channels)
				}
			}
			if err := acc.ReleaseBuffer(frames); err != nil {
				return
			}
		}
	}()

	if err := <-ready; err != nil {
		as.stop()
		return nil, err
	}
	return as, nil
}
