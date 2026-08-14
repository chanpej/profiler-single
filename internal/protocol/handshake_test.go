package protocol

import (
	"strings"
	"testing"
)

func TestIsSupportedUnityVersion(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"2022.3.45f1", true},
		{"2022.1.0a1", true},
		{"2022", true},
		{" 2022.3.5f1 ", true},
		{"2021.3.30f1", false},
		{"2023.2.1f1", false},
		{"6000.0.23f1", false},
		{"20223", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsSupportedUnityVersion(c.version); got != c.want {
			t.Errorf("IsSupportedUnityVersion(%q) = %v, want %v", c.version, got, c.want)
		}
	}
}

func TestHandshakeRoundTrip(t *testing.T) {
	want := Handshake{
		DeviceName:   "Pixel 7",
		DeviceModel:  "Google Pixel 7",
		OS:           "Android 14",
		UnityVersion: "2022.3.45f1",
		AppVersion:   "1.2.3",
	}
	payload, err := EncodeHandshake(&want)
	if err != nil {
		t.Fatalf("EncodeHandshake: %v", err)
	}
	got, err := DecodeHandshake(payload)
	if err != nil {
		t.Fatalf("DecodeHandshake: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestDecodeHandshakeRejectsUnsupportedVersion(t *testing.T) {
	h := Handshake{DeviceName: "iPhone 15", UnityVersion: "2021.3.30f1"}
	payload, err := EncodeHandshake(&h)
	if err != nil {
		t.Fatalf("EncodeHandshake: %v", err)
	}
	_, err = DecodeHandshake(payload)
	if err == nil {
		t.Fatal("expected error for unsupported unity version")
	}
	if !strings.Contains(err.Error(), "unsupported unity version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeHandshakeBadJSON(t *testing.T) {
	if _, err := DecodeHandshake([]byte("{not json")); err == nil {
		t.Fatal("expected error for bad json")
	}
}
