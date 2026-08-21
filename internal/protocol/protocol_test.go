package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func sampleFrame() FrameData {
	return FrameData{
		FrameIndex:         42,
		TimestampMs:        1755072000123,
		FPS:                59.7,
		CPUTimeMs:          16.4,
		GPUTimeMs:          12.1,
		MainThreadMs:       14.2,
		RenderMs:           6.3,
		GCAllocKB:          128.5,
		UsedMemMB:          512.25,
		ReservedMB:         768,
		DrawCalls:          321,
		SetPassCalls:       45,
		Batches:            280,
		Triangles:          1200000,
		Vertices:           900000,
		CPURenderingMs:     5.2,
		CPUScriptsMs:       4.1,
		CPUPhysicsMs:       1.8,
		CPUAnimationMs:     1.2,
		CPUGCMs:            0.4,
		CPUVSyncMs:         0.9,
		CPUUIMs:            1.1,
		CPUOthersMs:        1.7,
		GCUsedMB:           180.5,
		GCReservedMB:       240,
		GfxUsedMB:          160.25,
		GfxReservedMB:      200,
		AudioUsedMB:        8.5,
		VideoUsedMB:        0.5,
		ProfilerUsedMB:     12,
		ProfilerReservedMB: 20,
		SystemUsedMB:       150.5,
		CommittedMemMB:     768,
		TextureMemMB:       30.2,
		MeshMemMB:          3.4,
		MaterialCount:      1003,
		ObjectCount:        4310,
		TextureCount:       594,
		MeshCount:          166,
		GameObjectCount:    4310,
		SceneObjectCount:   15974,
		MemoryCounterMask:  MemoryCounterCommitted | MemoryCounterTexture | MemoryCounterObjectCount,
		ShadowCasters:      18,
		PlayingSources:     9,
		AudioVoices:        14,
		ActiveRigidbodies:  42,
		Colliders:          128,
		UIBatches:          24,
		UIVertices:         4800,
	}
}

func TestFrameEncodeDecodeRoundTrip(t *testing.T) {
	want := sampleFrame()
	payload := EncodeFrame(&want)
	if len(payload) != FramePayloadSize {
		t.Fatalf("payload size = %d, want %d", len(payload), FramePayloadSize)
	}
	got, err := DecodeFrame(payload)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestFramePayloadSize(t *testing.T) {
	if FramePayloadSize != 204 {
		t.Fatalf("FramePayloadSize = %d, want 204", FramePayloadSize)
	}
	if ExtendedFramePayloadSize != 164 {
		t.Fatalf("ExtendedFramePayloadSize = %d, want 164", ExtendedFramePayloadSize)
	}
	if LegacyFramePayloadSize != 68 {
		t.Fatalf("LegacyFramePayloadSize = %d, want 68", LegacyFramePayloadSize)
	}
	if len(EncodeFrame(&FrameData{})) != FramePayloadSize {
		t.Fatal("EncodeFrame 长度应等于 FramePayloadSize")
	}
}

func TestDecodeExtendedFrameKeepsV2Compatibility(t *testing.T) {
	full := sampleFrame()
	got, err := DecodeFrame(EncodeFrame(&full)[:ExtendedFramePayloadSize])
	if err != nil {
		t.Fatalf("DecodeFrame v2: %v", err)
	}
	if got.UsedMemMB != full.UsedMemMB || got.GCUsedMB != full.GCUsedMB || got.DrawCalls != full.DrawCalls {
		t.Fatalf("v2 fields changed: %+v", got)
	}
	if got.CommittedMemMB != 0 || got.TextureMemMB != 0 || got.MemoryCounterMask != 0 {
		t.Fatalf("v2 must not invent v3 memory fields: %+v", got)
	}
}

func TestDecodeLegacyFrame(t *testing.T) {
	full := sampleFrame()
	legacy := EncodeFrame(&full)[:LegacyFramePayloadSize]
	got, err := DecodeFrame(legacy)
	if err != nil {
		t.Fatalf("DecodeFrame v1: %v", err)
	}
	if got.FrameIndex != full.FrameIndex || got.FPS != full.FPS || got.DrawCalls != full.DrawCalls {
		t.Fatalf("v1 基础字段丢失: %+v", got)
	}
	if got.CPUScriptsMs != 0 || got.GCUsedMB != 0 || got.ShadowCasters != 0 {
		t.Fatalf("v1 扩展字段应保持零值: %+v", got)
	}
}

func TestDecodeFrameWrongSize(t *testing.T) {
	if _, err := DecodeFrame(make([]byte, 3)); err == nil {
		t.Fatal("expected error for short payload")
	}
}

func TestMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	frame := sampleFrame()
	if err := WriteMessage(&buf, MsgFrame, EncodeFrame(&frame)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	if err := WriteMessage(&buf, MsgPing, nil); err != nil {
		t.Fatalf("WriteMessage ping: %v", err)
	}

	typ, payload, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if typ != MsgFrame {
		t.Fatalf("type = %d, want MsgFrame", typ)
	}
	got, err := DecodeFrame(payload)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if got != frame {
		t.Fatalf("frame mismatch: got %+v", got)
	}

	typ, payload, err = ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage ping: %v", err)
	}
	if typ != MsgPing || len(payload) != 0 {
		t.Fatalf("ping mismatch: type=%d len=%d", typ, len(payload))
	}
}

func TestReadMessageBadMagic(t *testing.T) {
	data := []byte{'X', 'X', 'X', 'X', 1, 0, 0, 0, 0}
	if _, _, err := ReadMessage(bytes.NewReader(data)); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("err = %v, want ErrBadMagic", err)
	}
}

func TestReadMessageTruncated(t *testing.T) {
	var buf bytes.Buffer
	frame := sampleFrame()
	if err := WriteMessage(&buf, MsgFrame, EncodeFrame(&frame)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	data := buf.Bytes()[:buf.Len()-4]
	if _, _, err := ReadMessage(bytes.NewReader(data)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestWriteMessageTooLarge(t *testing.T) {
	err := WriteMessage(io.Discard, MsgHandshake, make([]byte, MaxPayloadSize+1))
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("err = %v, want ErrPayloadTooLarge", err)
	}
}
