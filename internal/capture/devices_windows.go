//go:build windows

package capture

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func audioDevices() (speakers, microphones []AudioDevice, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil, nil, err
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		return nil, nil, fmt.Errorf("device enumerator: %w", err)
	}
	defer de.Release()
	speakers, speakerErr := listAudioDevices(de, uint32(wca.ERender))
	if speakerErr != nil {
		speakerErr = fmt.Errorf("speakers: %w", speakerErr)
	}
	microphones, microphoneErr := listAudioDevices(de, uint32(wca.ECapture))
	if microphoneErr != nil {
		microphoneErr = fmt.Errorf("microphones: %w", microphoneErr)
	}
	return speakers, microphones, errors.Join(speakerErr, microphoneErr)
}

// openAudioDevice finds an endpoint by ID without changing the system default.
func openAudioDevice(de *wca.IMMDeviceEnumerator, flow uint32, id string) (*wca.IMMDevice, error) {
	if id == "" {
		var mmd *wca.IMMDevice
		if err := de.GetDefaultAudioEndpoint(flow, wca.EConsole, &mmd); err != nil {
			return nil, fmt.Errorf("no default audio endpoint: %w", err)
		}
		return mmd, nil
	}
	var devices *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(flow, wca.DEVICE_STATE_ACTIVE, &devices); err != nil {
		return nil, fmt.Errorf("list audio endpoints: %w", err)
	}
	defer devices.Release()
	var count uint32
	if err := devices.GetCount(&count); err != nil {
		return nil, err
	}
	for i := uint32(0); i < count; i++ {
		var mmd *wca.IMMDevice
		if err := devices.Item(i, &mmd); err != nil {
			continue
		}
		var deviceID string
		if err := mmd.GetId(&deviceID); err != nil {
			mmd.Release()
			continue
		}
		if deviceID == id {
			return mmd, nil
		}
		mmd.Release()
	}
	return nil, fmt.Errorf("selected audio device is not available")
}

func listAudioDevices(de *wca.IMMDeviceEnumerator, flow uint32) ([]AudioDevice, error) {
	defaultID := ""
	var fallback *wca.IMMDevice
	if err := de.GetDefaultAudioEndpoint(flow, wca.EConsole, &fallback); err == nil {
		_ = fallback.GetId(&defaultID)
		fallback.Release()
	}

	var collection *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(flow, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()
	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}
	devices := make([]AudioDevice, 0, count)
	defaultDevice := AudioDevice{}
	for i := uint32(0); i < count; i++ {
		var mmd *wca.IMMDevice
		if err := collection.Item(i, &mmd); err != nil {
			continue
		}
		device, err := readAudioDevice(mmd)
		mmd.Release()
		if err != nil {
			continue // one broken endpoint must not hide every working one
		}
		if device.Name == "" {
			device.Name = fmt.Sprintf("Audio device %d", i+1)
		}
		if device.ID == defaultID {
			defaultDevice = device
		} else {
			devices = append(devices, device)
		}
	}
	if defaultDevice.ID != "" {
		devices = append([]AudioDevice{defaultDevice}, devices...)
	}
	return devices, nil
}

func readAudioDevice(mmd *wca.IMMDevice) (AudioDevice, error) {
	var id string
	if err := mmd.GetId(&id); err != nil {
		return AudioDevice{}, err
	}
	name := ""
	var store *wca.IPropertyStore
	if err := mmd.OpenPropertyStore(wca.STGM_READ, &store); err == nil {
		defer store.Release()
		for _, key := range []*wca.PROPERTYKEY{&wca.PKEY_Device_FriendlyName, &wca.PKEY_DeviceInterface_FriendlyName} {
			var value wca.PROPVARIANT
			if err := store.GetValue(key, &value); err == nil {
				if value.VT == ole.VT_LPWSTR {
					name = value.String() // go-wca releases the LPWSTR here
				} else {
					_ = value.Clear()
				}
				if name != "" {
					break
				}
			}
		}
	}
	return AudioDevice{ID: id, Name: name}, nil
}
