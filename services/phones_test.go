package services

import (
	"path/filepath"
	"testing"
	"unityperf/internal/adbforward"
	"unityperf/internal/protocol"
	"unityperf/internal/unityprofiler"
	"unityperf/internal/upac"
)

func TestPhoneDetailsAndWirelessPort(t *testing.T) {
	data := "Samsung\nSM-G9650\n10\n29\narm64-v8a\n34567\nPhysical size: 1440x2960\nMemTotal: 123 kB\n9: wlan0 inet 10.2.3.4/24 scope global wlan0"
	phone := readPhone(adbforward.Device{Serial: "USB001"}, data)
	if phone.Endpoint != "10.2.3.4:34567" || phone.Screen != "1440x2960" || phone.Android != "10" || phone.Memory != "123 kB" {
		t.Fatal(phone)
	}
	phone = readPhone(adbforward.Device{Serial: "10.2.3.4:44000"}, data)
	if phone.Endpoint != phone.Serial {
		t.Fatal(phone)
	}
}

func TestImportedRecordingPhoneSurvivesExportAndClearsForLegacy(t *testing.T) {
	svc := NewPerfService()
	defer svc.ServiceShutdown()
	dir := t.TempDir()
	phone := &upac.PhoneInfo{Model: "Pixel", Brand: "Google", Android: "13", SDK: "33", ABI: "arm64-v8a", Screen: "1080x2400", Memory: "123 kB", IP: "10.0.0.8", Endpoint: "10.0.0.8:5555", Serial: "abc"}
	for _, want := range []*upac.PhoneInfo{phone, nil} {
		input := upac.File{Phone: want, Handshake: protocol.Handshake{UnityVersion: "2022.3.47f1", DeviceName: "Player"}, Raw: unityprofiler.SampleProfilerCapture()}
		src, dst := filepath.Join(dir, "in.upac"), filepath.Join(dir, "out.upac")
		if err := upac.SaveFile(src, input); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ImportCapture(src); err != nil {
			t.Fatal(err)
		}
		if err := svc.ExportCapture(dst); err != nil {
			t.Fatal(err)
		}
		got, err := upac.LoadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if got.Handshake != input.Handshake || (got.Phone == nil) != (want == nil) || (want != nil && *got.Phone != *want) {
			t.Fatalf("metadata lost: %+v / %+v", got.Handshake, got.Phone)
		}
	}
}
