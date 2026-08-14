package unityprofiler

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"unityperfagent/internal/protocol"
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
			ctrGCAllocInFrame: 4 * bytesPerKB,
		},
	}
	f := ToFrameData(snap)
	if f.FrameIndex != 9 {
		t.Fatalf("frameIndex=%d", f.FrameIndex)
	}
	if math.Abs(float64(f.CPUTimeMs)-16.6) > 0.001 {
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

func TestEncodeSetupFlags(t *testing.T) {
	if deep, stacks := ParseSetupFlags(EncodeSetupFlags(false, false)); deep || stacks {
		t.Fatal("zero flags should be off")
	}
	if deep, stacks := ParseSetupFlags(EncodeSetupFlags(true, false)); !deep || stacks {
		t.Fatalf("deep=%v stacks=%v", deep, stacks)
	}
	if deep, stacks := ParseSetupFlags(EncodeSetupFlags(false, true)); deep || !stacks {
		t.Fatalf("deep=%v stacks=%v", deep, stacks)
	}
	if deep, stacks := ParseSetupFlags(EncodeSetupFlags(true, true)); !deep || !stacks {
		t.Fatalf("deep=%v stacks=%v", deep, stacks)
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
	ev.StringMeta("GC.Alloc\nPlayerController.Update ()")
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
	if len(alloc.Callstack) < 2 {
		t.Fatalf("callstack=%v", alloc.Callstack)
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
	hs, err := sess.Connect("127.0.0.1", port, func(_ protocol.FrameData, h []protocol.SampleNode) {
		mu.Lock()
		last = h
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
