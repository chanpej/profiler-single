package unityprofiler

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"unityperf/internal/profileframe"
	"unityperf/internal/protocol"
)

// 以下固件全部截自 Unity 2022.3 Editor 与 Android 真机（示例游戏，包 com.company.game）
// 之间的实际报文，作为协议实现的回归基准。
const (
	// Player 连上后推送的引擎信息 + 宣告（302 字节）。
	captureUSBIntroHex = "8F4EA567493DEA75ACB4E49D48808B721F0858D31F000000020000000000000B000000E2070000040000000C00000002000000010000008F4EA567150D0F33DB48E48938CFF9BAD22EAB07DF0000005B49505D2031302E32312E34322E313539205B506F72745D203535303030205B466C6167735D2032313437343833363530205B477569645D2033333738363138383932205B456469746F7249645D2031303235303539343636205B56657273696F6E5D2031303438383332205B49645D20416E64726F6964506C617965722831312C5869616F6D695F4D695F4D49585F32534031302E32312E34322E31353929205B44656275675D2030205B5061636B6167654E616D655D20416E64726F6964506C61796572205B50726F6A6563744E616D655D2044656D6F2047616D6500"

	// Editor 连上后立刻下发的前 3 条报文：Profiler 初始化、ProfilerArea 掩码、分类名单。
	captureEditorStartHex = "8f4ea5674cd890f7f8ae6443498b0fb8b0550da504000000000000008f4ea567227564d6e0e074ad9828c60fe48631c504000000fd7f00008f4ea567774a10e6b66824b0e8ed34ede4663a5f4b000000566964656f0050687973696373324400506879736963730052656e646572005669727475616c20546578747572696e6700417564696f004d656d6f7279004c69676874696e670047756900"

	// Profiler 数据流首包（56 字节）。
	captureStreamHeaderHex = "5044335501000b00280322200100000000000000010000000000000050ed0c8572000000e2070000040000000c0000000200000001000000"

	// 元数据流真实记录：Memory 分类、Total Used Memory 计数器定义、主线程定义。
	captureCategoryMemoryHex = "3e0017000c000100060000004d656d6f7279"
	captureMarkerTotalUsed   = "01007c0600008800170011000000546f74616c2055736564204d656d6f727901040200000000"
	captureThreadMainHex     = "210050ed0c857200000032f2da599852000000000000000000000b0000004d61696e20546872656164"

	// 事件流真实记录：帧边界（第 1 帧）与 Total Used Memory 取值。
	captureFrameBoundaryHex = "220001000000df2a6c6998520000"
	captureCounterUsedHex   = "2c00007c060000cdadbb6898520000010433889d1200000000"

	// 以下三条截自开启 Call Stacks 后的同一台真机（Unity 2022.3.47f1，IL2CPP 包）。
	// 元数据流里的托管方法符号：地址 0x71f7b5747c → UIPanel.FillDrawCall()。
	captureMethodInfoHex = "080000000000000000007c74b5f7710000000000000036000000417373656d626c792d4353686172702d6669727374706173732e646c6c213a3a554950616e656c2e46696c6c4472617743616c6c28290000000000000000"
	// 一次带调用栈的分配采样：UnsafeUtility.Malloc 开始 → 两条取值 → Profiler.Callstack
	// 采集开销采样 → 25 层调用栈（0x03 后跟 0x00）→ UnsafeUtility.Malloc 结束。
	captureAllocCallstackHex = "2a00008f04000064aa558f5c630000020408000000000000000204000000240000ce0400006dac558f5c630000250000ce040000b519598f5c63000003000000000019c4a8f37f72000000ccbed77f72000000fc739f7f72000000f4729f7f72000000ec24d3fe710000006823d3fe71000000fcd7c5f77100000034d4c5f7710000004875b5f771000000e864b5f7710000009c5eb5f771000000e0170eff710000001c170eff71000000f05cde7f72000000a083df7f7200000024d4e07f720000008c94b77f720000001c94b77f72000000900cc67f72000000b837c57f72000000f837c57f72000000f83ac57f72000000d0fceb7f72000000d035ee7f720000003c216bac720000002500008f040000fb29598f5c630000"
	// 采样自带的 3 个元数据取值（instanceID + UTF-16 名字 + 计数），首字节同为 0x03。
	captureSampleMeta3Hex = "030100000000091a00000055007000640061007400650057006900640067006500740073000301000000"
	// 带 Unity 对象上下文的采样开始（0x2b）+ 4 个取值（InstanceID、对象名 SFX_Weapon_Empty、Int32、Int64）。
	// 截自 Android 真机 Unity 2022.3.47f1 主线程流；未识别时会让整段采集失步。
	captureBeginObjectHex = "2b0000450400004c944ba90843010004016e1a000008100000005346585f576561706f6e5f456d70747902420c0000040100000000000000"
	// 脚本线程时钟戳（0x38）：type + pad + int64。截自切后台时 Scripting Threads/#0 的整条流。
	captureThreadClockHex = "3800ab3dbc7fa4430100"
)

// captureMainThreadID 是流头与线程定义里一致的主线程 ID。
const captureMainThreadID int64 = 0x72850ced50

// captureTotalUsedBytes 是上面那条计数器记录携带的字节数。
const captureTotalUsedBytes = 312313907

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("固件 hex 解析失败: %v", err)
	}
	return b
}

func TestMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, MsgIDProfilerAreas, EncodeAreaMask(EditorAreaMask)); err != nil {
		t.Fatal(err)
	}
	msg, err := ReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != MsgIDProfilerAreas {
		t.Fatalf("id=%s", msg.ID)
	}
	if len(msg.Body) != 4 || EncodeAreaMask(EditorAreaMask)[0] != msg.Body[0] {
		t.Fatalf("body=%x", msg.Body)
	}
	if buf.Len() != 0 {
		t.Fatalf("leftover=%d", buf.Len())
	}
}

func TestReadMessageBadMagic(t *testing.T) {
	data := make([]byte, ConnHeaderSize)
	if _, err := ReadMessage(bytes.NewReader(data)); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("err=%v", err)
	}
}

func TestReadMessageTruncated(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteMessage(&buf, MsgIDProfilerData, []byte{1, 2, 3, 4})
	data := buf.Bytes()[:buf.Len()-2]
	if _, err := ReadMessage(bytes.NewReader(data)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v", err)
	}
}

func TestPlayerIntroFromCapture(t *testing.T) {
	r := bytes.NewReader(mustHex(t, captureUSBIntroHex))

	first, err := ReadMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != MsgIDPlayerInfo {
		t.Fatalf("首条报文 id=%s，期望 MsgIDPlayerInfo", first.ID)
	}
	engine, err := ParsePlayerEngineInfo(first.Body)
	if err != nil {
		t.Fatal(err)
	}
	if engine.Version != "2018.4.12f1" {
		t.Fatalf("engine version=%q", engine.Version)
	}
	if engine.Platform != platformAndroid {
		t.Fatalf("platform=%d", engine.Platform)
	}

	second, err := ReadMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != MsgIDAnnounce {
		t.Fatalf("第二条报文 id=%s，期望 MsgIDAnnounce", second.ID)
	}
	info, err := ParseAnnouncement(AnnouncementFromBody(second.Body))
	if err != nil {
		t.Fatal(err)
	}
	if info.ProjectName != "Demo Game" || info.Port != 55000 {
		t.Fatalf("%+v", info)
	}
	if !info.SupportsProfile() {
		t.Fatalf("flags=%d 应含 SupportsProfile", info.Flags)
	}
	if r.Len() != 0 {
		t.Fatalf("leftover=%d", r.Len())
	}
}

// TestWriteProfilerStartMatchesEditor 保证本工具下发的开启报文与 Editor 逐字节一致，
// 这是「连上但采不到数据」的根因所在。
func TestWriteProfilerStartMatchesEditor(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteProfilerStart(&buf, EditorAreaMask, EditorCategories); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, captureEditorStartHex)
	got := buf.Bytes()
	if len(got) < len(want) {
		t.Fatalf("下发 %d 字节，少于 Editor 的前 3 条报文 %d 字节", len(got), len(want))
	}
	if !bytes.Equal(got[:len(want)], want) {
		t.Fatalf("与 Editor 报文不一致:\n got=%x\nwant=%x", got[:len(want)], want)
	}
	// 第 4 条是本工具自报的宣告，只校验消息类型。
	last, err := ReadMessage(bytes.NewReader(got[len(want):]))
	if err != nil {
		t.Fatal(err)
	}
	if last.ID != MsgIDAnnounce {
		t.Fatalf("第 4 条 id=%s，期望宣告", last.ID)
	}
	if _, err := ParseAnnouncement(AnnouncementFromBody(last.Body)); err != nil {
		t.Fatalf("自报宣告无法解析: %v", err)
	}
}

func TestParseStreamHeaderFromCapture(t *testing.T) {
	h, err := ParseStreamHeader(mustHex(t, captureStreamHeaderHex))
	if err != nil {
		t.Fatal(err)
	}
	if h.FormatVersion != Unity2022StreamVersion {
		t.Fatalf("format=%#x", h.FormatVersion)
	}
	if !IsSupportedStreamVersion(h.FormatVersion) {
		t.Fatal("Unity 2022 数据流应受支持")
	}
	if h.MainThreadID != captureMainThreadID {
		t.Fatalf("mainThreadID=%#x", h.MainThreadID)
	}
	if h.EngineVersion != "2018.4.12f1" {
		t.Fatalf("engine=%q", h.EngineVersion)
	}
}

func TestChunkRoundTrip(t *testing.T) {
	payload := []byte("profiler-records")
	chunk, err := ParseChunk(EncodeChunk(7, captureMainThreadID, payload))
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Seq != 7 || chunk.ThreadID != captureMainThreadID {
		t.Fatalf("%+v", chunk)
	}
	if !bytes.Equal(chunk.Payload, payload) {
		t.Fatalf("payload=%q", chunk.Payload)
	}
}

func TestParseChunkRejectsCorruptFooter(t *testing.T) {
	raw := EncodeChunk(1, metaThreadID, []byte{1, 2, 3})
	raw[len(raw)-1] ^= 0xFF
	if _, err := ParseChunk(raw); err == nil {
		t.Fatal("尾部魔数损坏应报错")
	}
}

// captureFeed 用实测记录喂解码器：流头 + 元数据 + 两个帧边界夹一条计数器取值。
func captureFeed(t *testing.T, d *Decoder, chunkSize int) {
	t.Helper()
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.buf = append(meta.buf, mustHex(t, captureCategoryMemoryHex)...)
	meta.buf = append(meta.buf, mustHex(t, captureMarkerTotalUsed)...)
	meta.buf = append(meta.buf, mustHex(t, captureThreadMainHex)...)

	var events RecordWriter
	events.buf = append(events.buf, mustHex(t, captureFrameBoundaryHex)...)
	events.buf = append(events.buf, mustHex(t, captureCounterUsedHex)...)
	events.FrameBoundary(2, 0x529869_6c2adf+33_000_000)

	feed := func(threadID int64, data []byte) {
		var seq int32
		for off := 0; off < len(data); off += chunkSize {
			end := off + chunkSize
			if end > len(data) {
				end = len(data)
			}
			if err := d.Feed(EncodeChunk(seq, threadID, data[off:end])); err != nil {
				t.Fatalf("Feed: %v", err)
			}
			seq++
		}
	}
	feed(metaThreadID, meta.Bytes())
	feed(captureMainThreadID, events.Bytes())
}

func TestDecoderParsesRealCaptureRecords(t *testing.T) {
	var got []FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { got = append(got, f) })
	captureFeed(t, d, 4096)

	if len(got) != 1 {
		t.Fatalf("解出 %d 帧，期望 1 帧（stats=%+v）", len(got), d.Stats())
	}
	snap := got[0]
	if snap.Index != 1 {
		t.Fatalf("帧号=%d", snap.Index)
	}
	v, ok := snap.Counters["Memory/Total Used Memory"]
	if !ok {
		t.Fatalf("缺少 Total Used Memory，实际计数器=%v", snap.Counters)
	}
	if int64(v) != captureTotalUsedBytes {
		t.Fatalf("Total Used Memory=%v，期望 %d", v, captureTotalUsedBytes)
	}
	if d.Stats().Desynced {
		t.Fatalf("不应失步: %+v", d.Stats())
	}
	if name := d.MainThreadName(); name != "Main Thread" {
		t.Fatalf("主线程名=%q", name)
	}
}

// TestDecoderReassemblesRecordsAcrossChunks 用 1 字节分块逼出跨块记录重组路径。
func TestDecoderReassemblesRecordsAcrossChunks(t *testing.T) {
	var got []FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { got = append(got, f) })
	captureFeed(t, d, 1)

	if len(got) != 1 {
		t.Fatalf("解出 %d 帧，期望 1 帧（stats=%+v）", len(got), d.Stats())
	}
	if int64(got[0].Counters["Memory/Total Used Memory"]) != captureTotalUsedBytes {
		t.Fatalf("跨块重组后取值错误: %v", got[0].Counters)
	}
}

// TestDecoderMergesCountersFromOtherThreads 校验渲染统计口径：Draw Calls 之类的计数器
// 由渲染线程上报，必须并入当前帧，否则真机上这些指标恒为 0。
func TestDecoderMergesCountersFromOtherThreads(t *testing.T) {
	const renderThreadID int64 = 0x7211112222
	const ctrID uint32 = 910

	var got []FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { got = append(got, f) })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}

	var meta RecordWriter
	meta.CategoryDef(1, "Render")
	meta.CounterDef(ctrID, 1, "Draw Calls Count", DataTypeInt32, UnitCount)
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	var main RecordWriter
	main.FrameBoundary(1, 1_000_000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, main.Bytes())); err != nil {
		t.Fatal(err)
	}

	var render RecordWriter
	render.FrameBoundary(1, 1_100_000) // 非主线程的帧记录不得切帧
	render.CounterInt32(ctrID, 1_200_000, 137)
	if err := d.Feed(EncodeChunk(0, renderThreadID, render.Bytes())); err != nil {
		t.Fatal(err)
	}

	main.Reset()
	main.FrameBoundary(2, 2_000_000)
	if err := d.Feed(EncodeChunk(1, captureMainThreadID, main.Bytes())); err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("解出 %d 帧，期望 1 帧（非主线程帧记录不应切帧）: %+v", len(got), d.Stats())
	}
	if v := got[0].Counters["Render/Draw Calls Count"]; v != 137 {
		t.Fatalf("渲染线程计数器未并入当前帧: %v", got[0].Counters)
	}
	// 非主线程的采样不应污染分类自耗时（合计须等于主线程根采样）。
	if len(got[0].CategoryNS) != 0 {
		t.Fatalf("非主线程采样污染了分类自耗时: %v", got[0].CategoryNS)
	}
}

// TestDecoderIsolatesBadThreadStream 校验单条非主线程流失步只被单独放弃，
// 主线程与元数据流照常出帧。
func TestDecoderIsolatesBadThreadStream(t *testing.T) {
	var got []FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { got = append(got, f) })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	if err := d.Feed(EncodeChunk(0, 0x7211112222, []byte{0xEE, 0xEE, 0xEE})); err != nil {
		t.Fatalf("非主线程流的坏数据不应向上报错: %v", err)
	}
	if st := d.Stats(); st.DesyncedThreads != 1 || st.Desynced {
		t.Fatalf("坏流未被隔离: %+v", st)
	}

	var main RecordWriter
	main.FrameBoundary(1, 1_000_000)
	main.FrameBoundary(2, 2_000_000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, main.Bytes())); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || d.Stats().Desynced {
		t.Fatalf("坏流影响了主线程出帧: 帧数=%d stats=%+v", len(got), d.Stats())
	}
}

func TestDecoderDetectsSequenceGap(t *testing.T) {
	d := NewDecoder(nil)
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	if err := d.Feed(EncodeChunk(0, metaThreadID, mustHex(t, captureCategoryMemoryHex))); err != nil {
		t.Fatal(err)
	}
	if err := d.Feed(EncodeChunk(5, metaThreadID, mustHex(t, captureCategoryMemoryHex))); err == nil {
		t.Fatal("序号断裂应报错")
	}
	if !d.Stats().Desynced {
		t.Fatal("应标记失步")
	}
}

func TestDecoderFlagsUnknownRecordWithoutPanic(t *testing.T) {
	d := NewDecoder(nil)
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, []byte{0xF7, 0, 0, 0})); err != nil {
		t.Fatal(err)
	}
	st := d.Stats()
	if !st.Desynced || st.UnknownRecords["0xf7"] == 0 {
		t.Fatalf("stats=%+v", st)
	}
}

// TestDecoderCategorySelfTimeSumsToRoot 校验采样栈自耗时口径：各分类之和等于根采样时长。
func TestDecoderCategorySelfTimeSumsToRoot(t *testing.T) {
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })

	if err := d.Feed(EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion, MainThreadID: captureMainThreadID,
	}, 2022, 3, 47, 2, 1)); err != nil {
		t.Fatal(err)
	}

	var meta RecordWriter
	meta.CategoryDef(catRender, "Render")
	meta.CategoryDef(catScripts, "Scripts")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkRender, 0, catRender, "Camera.Render")
	meta.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	const msNS = int64(1_000_000)
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.Begin(mkRender, 2*msNS)  // Render 嵌套在根采样内
	ev.Begin(mkScripts, 3*msNS) // Scripts 再嵌套在 Render 内
	ev.End(mkScripts, 7*msNS)
	ev.End(mkRender, 9*msNS)
	ev.End(mkMainThread, 20*msNS)
	ev.FrameBoundary(2, 20*msNS)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}

	if snap == nil {
		t.Fatalf("未解出帧: %+v", d.Stats())
	}
	if snap.RootNS != 20*msNS {
		t.Fatalf("根采样=%d ns", snap.RootNS)
	}
	// Scripts 自耗时 4ms，Render 自耗时 (9-2)-4 = 3ms，根采样自耗时 20-9+2 = 13ms。
	if snap.CategoryNS["Scripts"] != 4*msNS {
		t.Fatalf("Scripts=%d", snap.CategoryNS["Scripts"])
	}
	if snap.CategoryNS["Render"] != 3*msNS {
		t.Fatalf("Render=%d", snap.CategoryNS["Render"])
	}
	var sum int64
	for _, ns := range snap.CategoryNS {
		sum += ns
	}
	if sum != snap.RootNS {
		t.Fatalf("分类自耗时合计 %d ns，应等于根采样 %d ns", sum, snap.RootNS)
	}
}

func TestToFrameDataMapsCounters(t *testing.T) {
	snap := FrameSnapshot{
		Index:      9,
		DurationNS: 20_000_000,
		RootNS:     18_000_000,
		CategoryNS: map[string]int64{"Render": 6_000_000, "Scripts": 4_000_000, "Physics2D": 1_000_000, "Internal": 7_000_000},
		Counters: map[string]float64{
			ctrCPUTotal:       16_600_000,
			ctrGPUFrame:       11_000_000,
			ctrDrawCalls:      321,
			ctrTotalUsed:      512 * bytesPerMB,
			ctrCommitted:      768 * bytesPerMB,
			ctrTextureMemory:  30 * bytesPerMB,
			ctrMeshMemory:     3 * bytesPerMB,
			ctrMaterialCount:  1003,
			ctrObjectCount:    4310,
			ctrGCAllocInFrame: 4 * bytesPerKB,
		},
	}
	f := ToFrameData(snap)
	if f.FrameIndex != 9 {
		t.Fatalf("frameIndex=%d", f.FrameIndex)
	}
	// CPU Usage 跟 Hierarchy：用主线程采样 18ms，不用 Render 计数器 16.6ms。
	if math.Abs(float64(f.CPUTimeMs)-18) > 0.001 {
		t.Fatalf("cpu=%v", f.CPUTimeMs)
	}
	if math.Abs(float64(f.GPUTimeMs)-11) > 0.001 {
		t.Fatalf("gpu=%v", f.GPUTimeMs)
	}
	if math.Abs(float64(f.FPS)-50) > 0.001 {
		t.Fatalf("fps=%v", f.FPS)
	}
	if f.DrawCalls != 321 {
		t.Fatalf("drawCalls=%d", f.DrawCalls)
	}
	if math.Abs(float64(f.UsedMemMB)-512) > 0.001 {
		t.Fatalf("usedMem=%v", f.UsedMemMB)
	}
	wantMemoryMask := protocol.MemoryCounterTotalUsed | protocol.MemoryCounterCommitted |
		protocol.MemoryCounterTexture | protocol.MemoryCounterMesh |
		protocol.MemoryCounterMaterialCount | protocol.MemoryCounterObjectCount |
		protocol.MemoryCounterGCAllocatedFrame
	if f.MemoryCounterMask != wantMemoryMask {
		t.Fatalf("memory counter mask=%#x, want %#x", f.MemoryCounterMask, wantMemoryMask)
	}
	if f.CommittedMemMB != 768 || f.TextureMemMB != 30 || f.MeshMemMB != 3 || f.MaterialCount != 1003 || f.ObjectCount != 4310 {
		t.Fatalf("memory editor counters=%+v", f)
	}
	if math.Abs(float64(f.GCAllocKB)-4) > 0.001 {
		t.Fatalf("gcAlloc=%v", f.GCAllocKB)
	}
	// 缺失计数器时回退到采样时长。
	if math.Abs(float64(f.MainThreadMs)-18) > 0.001 {
		t.Fatalf("mainThread=%v", f.MainThreadMs)
	}
	if math.Abs(float64(f.CPURenderingMs)-6) > 0.001 || math.Abs(float64(f.CPUScriptsMs)-4) > 0.001 {
		t.Fatalf("分类耗时=%+v", f)
	}
	if math.Abs(float64(f.CPUPhysicsMs)-1) > 0.001 {
		t.Fatalf("physics=%v", f.CPUPhysicsMs)
	}
	// Internal 不属于图表分段，应计入 Others。
	if math.Abs(float64(f.CPUOthersMs)-7) > 0.001 {
		t.Fatalf("others=%v", f.CPUOthersMs)
	}
}

// 资源预加载会把主线程卡住一整帧，但 Render/CPU Total Frame Time 只统计忙碌时间。
// CPU 模块必须跟 Hierarchy 的 Main Thread，否则顶栏 20ms、树里却是 258ms。
func TestToFrameDataCPUUsageMatchesMainThreadWhenWaiting(t *testing.T) {
	snap := FrameSnapshot{
		Index:      29,
		DurationNS: 256_000_000,
		RootNS:     258_200_000,
		CategoryNS: map[string]int64{
			"Render":    3_100_000,
			"Scripts":   4_400_000,
			"Physics":   1_400_000,
			"Animation": 700_000,
		},
		Counters: map[string]float64{
			ctrCPUTotal: 20_800_000,
			ctrCPUMain:  20_800_000,
		},
	}
	f := ToFrameData(snap)
	if math.Abs(float64(f.CPUTimeMs)-258.2) > 0.05 {
		t.Fatalf("cpu=%v，应等于 Main Thread 258.2ms", f.CPUTimeMs)
	}
	if math.Abs(float64(f.MainThreadMs)-258.2) > 0.05 {
		t.Fatalf("mainThread=%v", f.MainThreadMs)
	}
	if math.Abs(float64(f.FPS)-1000.0/256.0) > 0.05 {
		t.Fatalf("fps=%v", f.FPS)
	}
	wantOthers := 258.2 - 3.1 - 4.4 - 1.4 - 0.7
	if math.Abs(float64(f.CPUOthersMs)-wantOthers) > 0.05 {
		t.Fatalf("others=%v，期望 %v", f.CPUOthersMs, wantOthers)
	}
}

func TestParseAnnouncement(t *testing.T) {
	raw := `[IP] 10.0.1.152 [Port] 55000 [Flags] 3 [Guid] 2575380029 [EditorId] 4264788666 [Version] 1048832 [Id] AndroidPlayer(Pixel):56000 [Debug] 1 [PackageName] AndroidPlayer [ProjectName] MyGame`
	p, err := ParseAnnouncement(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.IP != "10.0.1.152" || p.Port != 55000 || !p.Debug {
		t.Fatalf("%+v", p)
	}
	if p.ID != "AndroidPlayer(Pixel):56000" || p.ProjectName != "MyGame" {
		t.Fatalf("%+v", p)
	}
	if !p.SupportsProfile() {
		t.Fatalf("flags=%d 应支持采集", p.Flags)
	}
	p2, err := ParseAnnouncement(FormatAnnouncement(p))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Port != p.Port || p2.IP != p.IP {
		t.Fatalf("roundtrip %+v", p2)
	}
}

func TestParseAnnouncementReject(t *testing.T) {
	if _, err := ParseAnnouncement("hello"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeviceModelFromID(t *testing.T) {
	cases := map[string]string{
		"AndroidPlayer(11,Xiaomi_Mi_MIX_2S@10.21.42.159)": "Xiaomi Mi MIX 2S",
		"AndroidPlayer(Pixel):56000":                      "Pixel",
		"WindowsEditor":                                   "",
	}
	for id, want := range cases {
		if got := deviceModelFromID(id); got != want {
			t.Fatalf("deviceModelFromID(%q)=%q，期望 %q", id, got, want)
		}
	}
}

func TestEncodeCallstackFlags(t *testing.T) {
	for _, want := range []uint32{
		0,
		CallstackFlagManagedAlloc,
		CallstackFlagNativeAlloc,
		CallstackFlagNativeAllocator,
		CallstackFlagsDefault,
	} {
		if got := ParseCallstackFlags(EncodeCallstackFlags(want)); got != want {
			t.Fatalf("flags 往返 %#x -> %#x", want, got)
		}
	}
	if ParseCallstackFlags(nil) != 0 {
		t.Fatal("空报文体应视为未开启")
	}
	// 勾选 Call Stacks 必须包含托管分配那一位：GC.Alloc 的调用栈才是用户要看的，
	// 只发原生分配位（历史实现）会导致界面上一条调用栈都没有。
	if CallstackFlagsDefault&CallstackFlagManagedAlloc == 0 {
		t.Fatal("默认掩码缺少托管分配调用栈")
	}
}

func TestEncodePlayerEngineInfoCapabilities(t *testing.T) {
	body := EncodePlayerEngineInfo(platformAndroid, 2022, 3, 47, 2, 1)
	info, err := ParsePlayerEngineInfo(body)
	if err != nil {
		t.Fatal(err)
	}
	if !info.DeepProfilerSupported || !info.ManagedCallstacksSupported {
		t.Fatalf("假 Player 应声明 Deep Profile / Call Stacks 能力: %+v", info)
	}
	if info.Version != "2022.3.47f1" {
		t.Fatalf("version=%q", info.Version)
	}
}

func TestDecoderBuildsHierarchyAndCallstack(t *testing.T) {
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion, MainThreadID: captureMainThreadID,
	}, 2022, 3, 47, 2, 1)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catScripts, "Scripts")
	meta.CategoryDef(catGC, "GC")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	meta.MarkerDef(mkPlayerUpd, 0, catScripts, "PlayerController.Update")
	meta.MarkerDef(mkGCAlloc, 0, catGC, "GC.Alloc")
	meta.MethodDef(fakeMethodPlayerUpd, "Assembly-CSharp.dll!::PlayerController.Update()", "", 0)
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	const ns = int64(1_000_000)
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.Begin(mkScripts, 0)
	ev.Begin(mkPlayerUpd, 0)
	ev.Begin(mkGCAlloc, ns)
	ev.Int64Meta(4096)
	ev.Callstack(fakeMethodPlayerUpd+0x40, fakeNativeAddr)
	ev.End(mkGCAlloc, ns+1000)
	ev.End(mkPlayerUpd, 4*ns)
	ev.End(mkScripts, 5*ns)
	ev.End(mkMainThread, 10*ns)
	ev.FrameBoundary(2, 10*ns)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		t.Fatalf("未解出帧: %+v", d.Stats())
	}
	if !hierarchyHas(snap.Hierarchy, "PlayerController.Update") {
		t.Fatalf("缺少 Deep Profile 方法: %+v", snap.Hierarchy)
	}
	alloc := findSample(snap.Hierarchy, "GC.Alloc")
	if alloc == nil {
		t.Fatal("缺少 GC.Alloc")
	}
	if alloc.GCAllocKB < 3.9 || alloc.GCAllocKB > 4.1 {
		t.Fatalf("GC.Alloc KB=%v", alloc.GCAllocKB)
	}
	// 分配量必须上浮到各级祖先：GC Alloc 列是"含子树"的，父行若为 0，
	// 用户在界面上从根往下看每行都是空的，无从知道该展开哪条分支。
	for _, name := range []string{"Main Thread", "BehaviourUpdate", "PlayerController.Update"} {
		anc := findSample(snap.Hierarchy, name)
		if anc == nil {
			t.Fatalf("缺少祖先 %s", name)
		}
		if anc.GCAllocKB < 3.9 || anc.GCAllocKB > 4.1 {
			t.Fatalf("%s 的 GC Alloc 应含子树 4KB，实际 %v", name, anc.GCAllocKB)
		}
	}
	// 托管帧按最近下界还原成方法名，方法表覆盖不到的原生帧保留地址。
	want := []string{"Assembly-CSharp.dll!::PlayerController.Update()", "0x727ff3a8c4"}
	if !reflect.DeepEqual(alloc.Callstack, want) {
		t.Fatalf("callstack=%v，期望 %v", alloc.Callstack, want)
	}
}

// Timeline 必须保留每次采样的开始时间，且不能把同名兄弟合并成一条，否则无法按 Unity 那样沿时间轴摆放。
func TestDecoderKeepsTimelineStartTimes(t *testing.T) {
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion, MainThreadID: captureMainThreadID,
	}, 2022, 3, 47, 2, 1)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catScripts, "Scripts")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	const ns = int64(1_000_000)
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.Begin(mkScripts, 2*ns)
	ev.End(mkScripts, 5*ns)
	ev.Begin(mkScripts, 6*ns)
	ev.End(mkScripts, 8*ns)
	ev.End(mkMainThread, 10*ns)
	ev.FrameBoundary(2, 10*ns)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		t.Fatal("未解出帧")
	}
	root := findSample(snap.Hierarchy, "Main Thread")
	if root == nil {
		t.Fatalf("缺少 Main Thread: %+v", snap.Hierarchy)
	}
	if math.Abs(float64(root.StartMs)) > 0.001 || math.Abs(float64(root.TotalMs)-10) > 0.001 {
		t.Fatalf("Main Thread start=%v total=%v", root.StartMs, root.TotalMs)
	}
	kids := root.Children
	if len(kids) != 2 {
		t.Fatalf("同名采样应保留两次实例，实际 %d: %+v", len(kids), kids)
	}
	if kids[0].Name != "BehaviourUpdate" || kids[1].Name != "BehaviourUpdate" {
		t.Fatalf("children=%+v", kids)
	}
	if math.Abs(float64(kids[0].StartMs)-2) > 0.001 || math.Abs(float64(kids[0].TotalMs)-3) > 0.001 {
		t.Fatalf("第一次 BehaviourUpdate = %+v", kids[0])
	}
	if math.Abs(float64(kids[1].StartMs)-6) > 0.001 || math.Abs(float64(kids[1].TotalMs)-2) > 0.001 {
		t.Fatalf("第二次 BehaviourUpdate = %+v", kids[1])
	}
}

// Main Thread 在真机上会跨过帧边界仍保持打开。切帧时必须把未闭合采样写入当前帧，
// 否则 Hierarchy 只在栈弹空的那几帧有数据，卡顿帧只剩分类汇总。
func TestDecoderSnapshotsOpenSamplesAtFrameBoundary(t *testing.T) {
	var snaps []FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snaps = append(snaps, f) })
	if err := d.Feed(EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion, MainThreadID: captureMainThreadID,
	}, 2022, 3, 47, 2, 1)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catScripts, "Scripts")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	const ns = int64(1_000_000)
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.Begin(mkScripts, 2*ns)
	ev.End(mkScripts, 4*ns)
	ev.Begin(mkScripts, 5*ns)
	ev.FrameBoundary(2, 10*ns)
	ev.End(mkScripts, 13*ns)
	ev.End(mkMainThread, 20*ns)
	ev.FrameBoundary(3, 20*ns)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("frames=%d，期望 2", len(snaps))
	}

	root := findSample(snaps[0].Hierarchy, "Main Thread")
	if root == nil {
		t.Fatalf("第 1 帧缺少 Main Thread: %+v", snaps[0].Hierarchy)
	}
	if math.Abs(float64(root.TotalMs)-10) > 0.001 {
		t.Fatalf("第 1 帧 Main Thread total=%v", root.TotalMs)
	}
	if len(root.Children) != 2 {
		t.Fatalf("第 1 帧应保留已闭合与跨帧采样，实际 %d: %+v", len(root.Children), root.Children)
	}
	if math.Abs(float64(root.Children[0].TotalMs)-2) > 0.001 {
		t.Fatalf("已闭合 BehaviourUpdate total=%v", root.Children[0].TotalMs)
	}
	if math.Abs(float64(root.Children[1].TotalMs)-5) > 0.001 {
		t.Fatalf("跨帧 BehaviourUpdate 第 1 帧 total=%v", root.Children[1].TotalMs)
	}
	if snaps[0].RootNS != 10*ns {
		t.Fatalf("第 1 帧 RootNS=%d", snaps[0].RootNS)
	}
	if snaps[0].CategoryNS["Scripts"] != 7*ns {
		t.Fatalf("第 1 帧 Scripts=%d", snaps[0].CategoryNS["Scripts"])
	}

	root2 := findSample(snaps[1].Hierarchy, "Main Thread")
	if root2 == nil {
		t.Fatalf("第 2 帧缺少 Main Thread: %+v", snaps[1].Hierarchy)
	}
	if math.Abs(float64(root2.TotalMs)-10) > 0.001 {
		t.Fatalf("第 2 帧 Main Thread total=%v", root2.TotalMs)
	}
	open := findSample(snaps[1].Hierarchy, "BehaviourUpdate")
	if open == nil {
		t.Fatalf("第 2 帧缺少 BehaviourUpdate: %+v", snaps[1].Hierarchy)
	}
	if math.Abs(float64(open.TotalMs)-3) > 0.001 {
		t.Fatalf("跨帧 BehaviourUpdate 第 2 帧 total=%v", open.TotalMs)
	}
	if snaps[1].CategoryNS["Scripts"] != 3*ns {
		t.Fatalf("第 2 帧 Scripts=%d", snaps[1].CategoryNS["Scripts"])
	}
}

func TestDecoderAcceptsThreadDefOnEventStream(t *testing.T) {
	d := NewDecoder(nil)
	if err := d.Feed(EncodeStreamHeader(StreamHeader{
		FormatVersion: Unity2022StreamVersion, MainThreadID: captureMainThreadID,
	}, 2022, 3, 47, 2, 1)); err != nil {
		t.Fatal(err)
	}
	const workerID int64 = 0x1111
	var ev RecordWriter
	ev.ThreadDef(workerID, 0, "Job", "Worker")
	if err := d.Feed(EncodeChunk(0, workerID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	st := d.Stats()
	if st.Desynced || st.DesyncedThreads != 0 {
		t.Fatalf("事件流线程定义不应失步: %+v", st)
	}
	if got := d.ThreadNames()[workerID]; got != "Job/Worker" {
		t.Fatalf("thread name=%q", got)
	}
}

// 用真机字节验证 Call Stacks：调用栈记录（0x03 后跟 0x00）与"3 个元数据取值"
// 记录（同为 0x03）撞号，必须各自解对，否则事件流会失步、整条流不再出帧。
func TestDecodeCaptureCallstack(t *testing.T) {
	const (
		mkUnsafeMalloc  uint32 = 1167
		mkProfCallstack uint32 = 1230
		// 固件里那条调用栈的时间戳落在这两点之间。
		captureAllocStartNS int64 = 0x635c8f550000
		captureAllocEndNS   int64 = 0x635c8f5a0000
	)
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkUnsafeMalloc, 0, catInternal, "UnsafeUtility.Malloc")
	meta.MarkerDef(mkProfCallstack, 0, catInternal, "Profiler.Callstack")
	meta.buf = append(meta.buf, mustHex(t, captureMethodInfoHex)...)
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	var ev RecordWriter
	ev.FrameBoundary(1, captureAllocStartNS)
	ev.Begin(mkMainThread, captureAllocStartNS)
	ev.buf = append(ev.buf, mustHex(t, captureSampleMeta3Hex)...)
	ev.buf = append(ev.buf, mustHex(t, captureAllocCallstackHex)...)
	ev.End(mkMainThread, captureAllocEndNS)
	ev.FrameBoundary(2, captureAllocEndNS)
	// 故意用小块喂入，覆盖调用栈记录跨分块重组。
	data := ev.Bytes()
	for seq, off := int32(0), 0; off < len(data); seq, off = seq+1, off+64 {
		end := off + 64
		if end > len(data) {
			end = len(data)
		}
		if err := d.Feed(EncodeChunk(seq, captureMainThreadID, data[off:end])); err != nil {
			t.Fatal(err)
		}
	}
	if st := d.Stats(); st.Desynced || st.LastError != "" {
		t.Fatalf("真机字节不应失步: %+v", st)
	}
	if snap == nil {
		t.Fatal("未解出帧")
	}
	alloc := findSample(snap.Hierarchy, "UnsafeUtility.Malloc")
	if alloc == nil {
		t.Fatalf("缺少分配采样: %+v", snap.Hierarchy)
	}
	if len(alloc.Callstack) != 25 {
		t.Fatalf("调用栈应为 25 层，实际 %d: %v", len(alloc.Callstack), alloc.Callstack)
	}
	// 第 9 帧地址 0x71f7b57548 落在方法表里 UIPanel.FillDrawCall 的方法体内。
	if got := alloc.Callstack[8]; got != "Assembly-CSharp-firstpass.dll!::UIPanel.FillDrawCall()" {
		t.Fatalf("托管帧未还原成方法名: %q", got)
	}
	// 方法表覆盖不到的引擎原生帧保留地址。
	if got := alloc.Callstack[0]; got != "0x727ff3a8c4" {
		t.Fatalf("原生帧应保留地址: %q", got)
	}
}

// 真机主线程会出现 0x2b（带 Unity 对象上下文的 Begin）。头部与 0x24/0x2a 同布局，
// 随后是取值记录；当成未知类型会让主线程流失步、采集停掉。
func TestDecodeCaptureBeginObject(t *testing.T) {
	const mkObject uint32 = 0x445 // 固件里那条 0x2b 的 marker ID
	const (
		objectStartNS = int64(0x14308a94b944c)
		objectEndNS   = objectStartNS + 1000
	)
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkObject, 0, catInternal, "Audio.Play")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	var ev RecordWriter
	ev.FrameBoundary(1, objectStartNS-1000)
	ev.Begin(mkMainThread, objectStartNS-1000)
	ev.buf = append(ev.buf, mustHex(t, captureBeginObjectHex)...)
	ev.End(mkObject, objectEndNS)
	ev.End(mkMainThread, objectEndNS+1000)
	ev.FrameBoundary(2, objectEndNS+1000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if st := d.Stats(); st.Desynced || st.LastError != "" {
		t.Fatalf("0x2b 真机字节不应失步: %+v", st)
	}
	if snap == nil {
		t.Fatal("未解出帧")
	}
	obj := findSample(snap.Hierarchy, "Audio.Play")
	if obj == nil {
		t.Fatalf("缺少对象采样: %+v", snap.Hierarchy)
	}
}

func TestDecoderBeginObjectRoundTrip(t *testing.T) {
	const mkObject uint32 = 2001
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.MarkerDef(mkObject, 0, catInternal, "VFX.Play")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.Begin(mkMainThread, 0)
	ev.BeginObject(mkObject, 100)
	ev.StringMeta("SFX_Weapon_Empty")
	ev.End(mkObject, 200)
	ev.End(mkMainThread, 1000)
	ev.FrameBoundary(2, 1000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if st := d.Stats(); st.Desynced {
		t.Fatalf("BeginObject 往返失步: %+v", st)
	}
	if snap == nil || findSample(snap.Hierarchy, "VFX.Play") == nil {
		t.Fatalf("缺少 VFX.Play: %+v", snap)
	}
}

// 真机 Scripting Threads 在 App 切后台时会发 0x38 时钟戳。未识别会放弃那条线程；
// 若落在主线程上更会让整段采集失步。
func TestDecodeCaptureThreadClock(t *testing.T) {
	const scriptTID int64 = 0x729718bd50
	const ctrID uint32 = 910
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catRender, "Render")
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	meta.CounterDef(ctrID, catRender, "Draw Calls Count", DataTypeInt32, UnitCount)
	meta.ThreadDef(scriptTID, 0, "Scripting Threads", "#0")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}

	var main RecordWriter
	main.FrameBoundary(1, 1_000_000)
	main.Begin(mkMainThread, 1_000_000)
	main.End(mkMainThread, 2_000_000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, main.Bytes())); err != nil {
		t.Fatal(err)
	}

	var script RecordWriter
	script.buf = append(script.buf, mustHex(t, captureThreadClockHex)...)
	script.CounterInt32(ctrID, 1_500_000, 42)
	if err := d.Feed(EncodeChunk(0, scriptTID, script.Bytes())); err != nil {
		t.Fatal(err)
	}

	main.Reset()
	main.FrameBoundary(2, 3_000_000)
	if err := d.Feed(EncodeChunk(1, captureMainThreadID, main.Bytes())); err != nil {
		t.Fatal(err)
	}

	if st := d.Stats(); st.Desynced || st.DesyncedThreads != 0 || st.UnknownRecords["0x38"] != 0 {
		t.Fatalf("0x38 不应失步: %+v", st)
	}
	if snap == nil {
		t.Fatal("未解出帧")
	}
	if v := snap.Counters["Render/Draw Calls Count"]; v != 42 {
		t.Fatalf("时钟戳之后的计数器应仍能并入当前帧: %v", snap.Counters)
	}
}

func TestDecoderThreadClockOnMainThread(t *testing.T) {
	var snap *FrameSnapshot
	d := NewDecoder(func(f FrameSnapshot) { snap = &f })
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	var meta RecordWriter
	meta.CategoryDef(catInternal, "Internal")
	meta.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	if err := d.Feed(EncodeChunk(0, metaThreadID, meta.Bytes())); err != nil {
		t.Fatal(err)
	}
	var ev RecordWriter
	ev.FrameBoundary(1, 0)
	ev.ThreadClock(100)
	ev.Begin(mkMainThread, 0)
	ev.End(mkMainThread, 1000)
	ev.FrameBoundary(2, 1000)
	if err := d.Feed(EncodeChunk(0, captureMainThreadID, ev.Bytes())); err != nil {
		t.Fatal(err)
	}
	if st := d.Stats(); st.Desynced || st.LastError != "" {
		t.Fatalf("主线程 0x38 不应失步: %+v", st)
	}
	if snap == nil || findSample(snap.Hierarchy, "Main Thread") == nil {
		t.Fatalf("缺少 Main Thread: %+v", snap)
	}
}

// 帧回调期间不得持有会话锁：调用方会在自己的锁里回查会话状态，持锁回调必然死锁。
func TestFrameCallbackDoesNotHoldSessionLock(t *testing.T) {
	p := NewPlayer()
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	sess := NewSession()
	reentered := make(chan struct{}, 1)
	if _, err := sess.Connect("127.0.0.1", port, func(_ profileframe.Frame) {
		// 回调里回查会话：若解码时仍持着会话锁，这里会永久阻塞。
		_ = sess.DecodeStats()
		_ = sess.Connected()
		select {
		case reentered <- struct{}{}:
		default:
		}
	}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reentered:
		sess.Close()
	case <-time.After(3 * time.Second):
		// 不能 Close：读取循环已卡死，等它退出会一起挂住。
		t.Fatal("帧回调被会话锁挡住了")
	}
}

// 主线程流失步后再也解不出帧，必须变成致命错误而不是"已连接却停在同一帧"。
func TestCheckDesyncIsFatalForMainThread(t *testing.T) {
	d := NewDecoder(nil)
	if err := d.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	if err := checkDesync(d); err != nil {
		t.Fatalf("正常流不应报错: %v", err)
	}
	// 0x7f 不是任何已知记录类型：字节流无法重新对齐。
	_ = d.Feed(EncodeChunk(0, captureMainThreadID, []byte{0x7f, 0, 0, 0}))
	err := checkDesync(d)
	if err == nil {
		t.Fatal("主线程流失步应报错")
	}
	if !strings.Contains(err.Error(), "0x7f") {
		t.Fatalf("错误信息应带上原因: %v", err)
	}
	// 非主线程流失步只丢那一条流，不影响逐帧指标。
	d2 := NewDecoder(nil)
	if err := d2.Feed(mustHex(t, captureStreamHeaderHex)); err != nil {
		t.Fatal(err)
	}
	_ = d2.Feed(EncodeChunk(0, captureMainThreadID+1, []byte{0x7f, 0, 0, 0}))
	if err := checkDesync(d2); err != nil {
		t.Fatalf("其他线程失步不应中断采集: %v", err)
	}
}

func TestMethodTableResolve(t *testing.T) {
	var tb methodTable
	tb.add(0x1000, "A")
	tb.add(0x2000, "B")
	tb.add(0x2000, "B2") // 同地址重复下发，后者生效
	cases := []struct {
		addr uint64
		want string
		ok   bool
	}{
		{0x1000, "A", true},
		{0x1040, "A", true},
		{0x2004, "B2", true},
		{0x0fff, "", false},                     // 首条符号之前
		{0x2000 + maxMethodSpan + 1, "", false}, // 超出方法体上限的原生地址
	}
	for _, c := range cases {
		got, ok := tb.resolve(c.addr)
		if got != c.want || ok != c.ok {
			t.Fatalf("resolve(%#x)=%q,%v，期望 %q,%v", c.addr, got, ok, c.want, c.ok)
		}
	}
	if tb.unique != 2 {
		t.Fatalf("去重后应为 2 条，实际 %d", tb.unique)
	}
}

func TestMergeSampleNodesSumsCalls(t *testing.T) {
	got := mergeSampleNodes([]protocol.SampleNode{
		{Name: "Update", Category: "Scripts", TotalMs: 1, SelfMs: 0.5, Calls: 1},
		{Name: "Update", Category: "Scripts", TotalMs: 2, SelfMs: 1, Calls: 1, Callstack: []string{"A"}},
		{Name: "LateUpdate", Category: "Scripts", TotalMs: 0.4, SelfMs: 0.4, Calls: 1},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Name != "Update" || got[0].Calls != 2 || got[0].TotalMs != 3 {
		t.Fatalf("merged=%+v", got[0])
	}
	if len(got[0].Callstack) != 1 {
		t.Fatalf("callstack should keep first non-empty: %v", got[0].Callstack)
	}
}

func hierarchyHas(nodes []protocol.SampleNode, name string) bool {
	return findSample(nodes, name) != nil
}

func findSample(nodes []protocol.SampleNode, name string) *protocol.SampleNode {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
		if found := findSample(nodes[i].Children, name); found != nil {
			return found
		}
	}
	return nil
}

// 用户的实际操作：只勾 Call Stacks，不开 Deep Profile。托管分配 GC.Alloc 必须带上
// 调用栈——历史实现把 Call Stacks 映射到原生分配那一位，界面上一条调用栈都看不到。
func TestCallStacksAloneCarriesManagedAllocStacks(t *testing.T) {
	p := NewPlayer()
	p.Interval = 5 * time.Millisecond
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	var mu sync.Mutex
	var last []protocol.SampleNode
	sess := NewSession()
	if _, err := sess.Connect("127.0.0.1", port, func(frame profileframe.Frame) {
		mu.Lock()
		last = frame.MainThread
		mu.Unlock()
	}, nil); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.SetProfilerOptions(false, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		h := last
		mu.Unlock()
		if alloc := findSample(h, "GC.Alloc"); alloc != nil && len(alloc.Callstack) > 0 {
			if got := p.csFlags.Load(); got&CallstackFlagManagedAlloc == 0 {
				t.Fatalf("下发的掩码 %#x 缺少托管分配位", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("只勾 Call Stacks 时 GC.Alloc 没有调用栈")
}

// 关掉 Call Stacks 后不应再下发任何调用栈位，但 GC.Alloc 采样本身仍在（真机如此）。
func TestCallStacksOffKeepsAllocSampleWithoutStack(t *testing.T) {
	p := NewPlayer()
	p.Interval = 5 * time.Millisecond
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	var mu sync.Mutex
	var last []protocol.SampleNode
	sess := NewSession()
	if _, err := sess.Connect("127.0.0.1", port, func(frame profileframe.Frame) {
		mu.Lock()
		last = frame.MainThread
		mu.Unlock()
	}, nil); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.SetProfilerOptions(false, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		h := last
		mu.Unlock()
		if alloc := findSample(h, "GC.Alloc"); alloc != nil && alloc.GCAllocKB > 0 {
			if len(alloc.Callstack) > 0 {
				t.Fatalf("未勾 Call Stacks 却有调用栈: %v", alloc.Callstack)
			}
			if got := p.csFlags.Load(); got != 0 {
				t.Fatalf("下发的掩码应为 0，实际 %#x", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("未收到带分配量的 GC.Alloc 采样")
}

func TestSessionDeepProfileAndCallStacks(t *testing.T) {
	p := NewPlayer()
	p.Interval = 5 * time.Millisecond
	port, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	var mu sync.Mutex
	var last []protocol.SampleNode
	sess := NewSession()
	hs, err := sess.Connect("127.0.0.1", port, func(frame profileframe.Frame) {
		mu.Lock()
		last = frame.MainThread
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if !hs.DeepProfilerSupported || !hs.ManagedCallstacksSupported {
		t.Fatalf("capabilities: %+v", hs)
	}
	if err := sess.SetProfilerOptions(true, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		h := last
		mu.Unlock()
		if hierarchyHas(h, "PlayerController.Update") && hierarchyHas(h, "GC.Alloc") {
			if alloc := findSample(h, "GC.Alloc"); alloc == nil || len(alloc.Callstack) == 0 {
				t.Fatal("GC.Alloc 应带调用栈")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("超时未收到 Deep Profile / Call Stacks 采样")
}

func TestIsLostConnection(t *testing.T) {
	if IsLostConnection(nil) || IsLostConnection(errors.New("解析失步")) {
		t.Fatal("nil / 失步不应算连接丢失")
	}
	if !IsLostConnection(io.EOF) || !IsLostConnection(io.ErrUnexpectedEOF) {
		t.Fatal("EOF 应算连接丢失")
	}
	if !IsLostConnection(errors.New("read tcp 127.0.0.1:1: wsarecv: An existing connection was forcibly closed by the remote host.")) {
		t.Fatal("Windows RST 应算连接丢失")
	}
}
