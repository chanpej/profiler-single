package unityprofiler

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"unityperf/internal/profileframe"
	"unityperf/internal/protocol"
)

// Profiler 原始数据流（MsgIDProfilerData 报文体）结构，实测自 Unity 2022.3 Player：
//
//	首包：56 字节流头，magic "PD3U" + 格式版本 + 主线程 ID + 引擎版本；
//	其后：分块，每块 = 头(20) + 负载(size) + 尾(8)。
//	  头：magic 0xB10C7EAD | seq(4) | threadIdLow(4) | threadIdHigh(4) | size(4)
//	  尾：seq+1(4) | magic 0xB10CF007
//
// 分块按线程多路复用：threadId = high<<32|low，与线程元数据记录里的 ID 一致；
// threadId 为 -1 的是全局元数据流。同一线程的负载按 seq 顺序拼成一条连续字节流，
// 记录可跨块，因此必须缓冲后再解析。

const (
	streamMagic   uint32 = 0x55334450 // "PD3U"
	chunkMagic    uint32 = 0xB10C7EAD
	chunkEndMagic uint32 = 0xB10CF007

	streamHeaderSize = 56
	chunkHeaderSize  = 20
	chunkFooterSize  = 8

	// metaThreadID 是全局元数据流的线程 ID。
	metaThreadID int64 = -1
)

// Unity2022StreamVersion 是 Unity 2022 Profiler 数据流的格式版本戳。
const Unity2022StreamVersion = protocol.Unity2022StreamVersion

// IsSupportedStreamVersion 判断 Profiler 数据流格式是否受支持。
func IsSupportedStreamVersion(v uint32) bool { return protocol.IsSupportedProfilerVersion(v) }

func IsStreamHeader(body []byte) bool {
	return len(body) >= streamHeaderSize && binary.LittleEndian.Uint32(body) == streamMagic
}

// StreamHeader 是 Profiler 数据流首包。
type StreamHeader struct {
	// FormatVersion 是数据流格式版本，Unity 2022 为 0x20220328。
	FormatVersion uint32
	// MainThreadID 指出哪条线程流承载主线程的逐帧事件。
	MainThreadID int64
	// EngineVersion 是流内自报的引擎版本。
	EngineVersion string
}

// ParseStreamHeader 解析 56 字节流头。
func ParseStreamHeader(b []byte) (StreamHeader, error) {
	if len(b) < streamHeaderSize {
		return StreamHeader{}, fmt.Errorf("流头长度 %d 字节，需要 %d 字节", len(b), streamHeaderSize)
	}
	le := binary.LittleEndian
	if got := le.Uint32(b[0:4]); got != streamMagic {
		return StreamHeader{}, fmt.Errorf("流头魔数无效 %#x", got)
	}
	formatVersion := le.Uint32(b[8:12])
	major, minor, patch := le.Uint32(b[36:40]), le.Uint32(b[40:44]), le.Uint32(b[44:48])
	releaseType, releaseNumber := le.Uint32(b[48:52]), le.Uint32(b[52:56])
	engineVersion := formatEngineVersion(major, minor, patch, releaseType, releaseNumber)
	if formatVersion == protocol.Tuanjie2022StreamVersion && releaseType == 4 {
		engineVersion = fmt.Sprintf("%d.%d.%dt%d", major, minor, patch, releaseNumber)
	}
	return StreamHeader{
		FormatVersion: formatVersion,
		MainThreadID:  int64(le.Uint64(b[28:36])),
		EngineVersion: engineVersion,
	}, nil
}

// Chunk 是一块线程流负载。
type Chunk struct {
	Seq      int32
	ThreadID int64
	Payload  []byte
}

func (c Chunk) IsMetadata() bool { return c.ThreadID == metaThreadID }

// ParseChunk 解析一块 Profiler 数据流分块。
func ParseChunk(b []byte) (Chunk, error) {
	if len(b) < chunkHeaderSize+chunkFooterSize {
		return Chunk{}, fmt.Errorf("unityprofiler: chunk %d bytes too short", len(b))
	}
	le := binary.LittleEndian
	if got := le.Uint32(b[0:4]); got != chunkMagic {
		return Chunk{}, fmt.Errorf("unityprofiler: bad chunk magic %08x", got)
	}
	size := int(le.Uint32(b[16:20]))
	if chunkHeaderSize+size+chunkFooterSize != len(b) {
		return Chunk{}, fmt.Errorf("unityprofiler: chunk size %d does not match body %d", size, len(b))
	}
	if got := le.Uint32(b[chunkHeaderSize+size+4:]); got != chunkEndMagic {
		return Chunk{}, fmt.Errorf("unityprofiler: bad chunk footer magic %08x", got)
	}
	lo := le.Uint32(b[8:12])
	hi := le.Uint32(b[12:16])
	return Chunk{
		Seq:      int32(le.Uint32(b[4:8])),
		ThreadID: int64(hi)<<32 | int64(lo),
		Payload:  b[chunkHeaderSize : chunkHeaderSize+size],
	}, nil
}

// EncodeChunk 组装一块分块（供假 Player 与单测使用）。
func EncodeChunk(seq int32, threadID int64, payload []byte) []byte {
	out := make([]byte, chunkHeaderSize+len(payload)+chunkFooterSize)
	le := binary.LittleEndian
	le.PutUint32(out[0:4], chunkMagic)
	le.PutUint32(out[4:8], uint32(seq))
	le.PutUint32(out[8:12], uint32(threadID&0xFFFFFFFF))
	le.PutUint32(out[12:16], uint32(threadID>>32))
	le.PutUint32(out[16:20], uint32(len(payload)))
	copy(out[chunkHeaderSize:], payload)
	le.PutUint32(out[chunkHeaderSize+len(payload):], uint32(seq+1))
	le.PutUint32(out[chunkHeaderSize+len(payload)+4:], chunkEndMagic)
	return out
}

// 记录类型。元数据流与线程事件流各自独立编号。
const (
	// 元数据流
	recMetaFlush    = 0x00
	recMetaMarker   = 0x01
	recMetaClass    = 0x0A
	recMetaObject   = 0x0B
	recMetaObject2  = 0x0C
	recMetaThread   = 0x21
	recMetaCategory = 0x3E
	recMetaCatFlag  = 0x3F
	// recMetaMethod 是托管方法符号：开启 Call Stacks 后 Player 会一次性把方法表全量下发，
	// 用于把调用栈里的代码地址还原成方法名。
	recMetaMethod = 0x08

	// 线程事件流
	recFrame        = 0x22
	recBeginPlain   = 0x24
	recEnd          = 0x25
	recBeginExtra   = 0x27
	recBeginMeta    = 0x2A
	recBeginObject  = 0x2B // 带 Unity 对象上下文的 Begin（随后跟 InstanceID、对象名等取值），布局同 0x24/0x2A
	recCounter      = 0x2C
	recThreadClock  = 0x38 // 脚本线程时钟戳（pad + int64），Android 切后台时 Scripting Threads 会单独发
	recInstanceHint = 0x39
	recLegacyStats  = 0x04
	recAudioStats   = 0x05
	// recCallstack 是当前采样的调用栈（地址数组），开启 Call Stacks 后才出现。
	recCallstack = 0x03

	// legacyStatsSize 是 Profiler.CollectLegacyStats 内定长统计结构（含 pad）的大小。
	legacyStatsSize = 1273
	// audioStatsHeadSize 是音频统计记录里字符串前的定长部分。
	audioStatsHeadSize = 13
)

// markerFlagCounter 标识该 marker 是一个 ProfilerCounter。
const markerFlagCounter = 0x80

// ProfilerMarkerDataType 的定长字节数；8/9/11 为带 u32 长度前缀的变长数据。
var valueFixedSize = map[byte]int{0x01: 4, 0x02: 4, 0x03: 4, 0x04: 8, 0x05: 8, 0x06: 4, 0x07: 8, 0x0C: 8}

func isVarLenValue(dt byte) bool { return dt == 0x08 || dt == 0x09 || dt == 0x0B }

// errShortRecord 表示当前缓冲区还凑不出一条完整记录，需要等更多分块。
var errShortRecord = errors.New("unityprofiler: incomplete record")

// cursor 是带边界检查的小端读取器；数据不足时 panic errShortRecord，由调用方 recover。
type cursor struct {
	b []byte
	p int
}

func (c *cursor) take(n int) []byte {
	if n < 0 || c.p+n > len(c.b) {
		panic(errShortRecord)
	}
	v := c.b[c.p : c.p+n]
	c.p += n
	return v
}

func (c *cursor) u8() byte      { return c.take(1)[0] }
func (c *cursor) u16() uint16   { return binary.LittleEndian.Uint16(c.take(2)) }
func (c *cursor) u32() uint32   { return binary.LittleEndian.Uint32(c.take(4)) }
func (c *cursor) i32() int32    { return int32(c.u32()) }
func (c *cursor) i64() int64    { return int64(binary.LittleEndian.Uint64(c.take(8))) }
func (c *cursor) peek() byte    { return c.b[c.p] }
func (c *cursor) hasMore() bool { return c.p < len(c.b) }

func (c *cursor) str() string { return string(c.take(int(c.u32()))) }

func (c *cursor) skip(n int) { c.take(n) }

// value 是一条 marker 元数据取值。
type value struct {
	Type byte
	Raw  []byte
}

// Number 把取值转为 float64；非数值类型返回 false。
func (v value) Number() (float64, bool) {
	le := binary.LittleEndian
	switch v.Type {
	case 0x01, 0x02:
		if len(v.Raw) == 4 {
			return float64(int32(le.Uint32(v.Raw))), true
		}
	case 0x03:
		if len(v.Raw) == 4 {
			return float64(le.Uint32(v.Raw)), true
		}
	case 0x04, 0x0C:
		if len(v.Raw) == 8 {
			return float64(int64(le.Uint64(v.Raw))), true
		}
	case 0x05:
		if len(v.Raw) == 8 {
			return float64(le.Uint64(v.Raw)), true
		}
	case 0x06:
		if len(v.Raw) == 4 {
			return float64(math.Float32frombits(le.Uint32(v.Raw))), true
		}
	case 0x07:
		if len(v.Raw) == 8 {
			return math.Float64frombits(le.Uint64(v.Raw)), true
		}
	}
	return 0, false
}

func (c *cursor) values(n int) []value {
	out := make([]value, 0, n)
	for i := 0; i < n; i++ {
		dt := c.u8()
		switch {
		case isVarLenValue(dt):
			out = append(out, value{Type: dt, Raw: c.take(int(c.u32()))})
		default:
			size, ok := valueFixedSize[dt]
			if !ok {
				panic(fmt.Errorf("unityprofiler: unknown marker data type 0x%02x", dt))
			}
			out = append(out, value{Type: dt, Raw: c.take(size)})
		}
	}
	return out
}

// markerInfo 是一个 marker / counter 的定义。
type markerInfo struct {
	Name     string
	Flags    uint16
	Category uint16
	// MetaCount 是该 marker 声明的元数据个数。
	MetaCount     int
	categoryField int
	nameField     int
}

// IsCounter 判断该 marker 是否为 ProfilerCounter。
func (m markerInfo) IsCounter() bool { return m.Flags&markerFlagCounter != 0 }

// methodSymbol 是一条托管方法符号。Addr 是方法入口地址，调用栈里的地址落在
// 方法体内部（返回地址），因此解析时取不超过 maxMethodSpan 的最近下界。
type methodSymbol struct {
	Addr uint64
	Name string
}

const (
	// maxMethodSpan 是方法体大小上限：地址与入口的距离超过它就认为不属于该方法
	// （Player 不上报方法体长度，末尾符号之后的原生地址必须落空而不是错认）。
	maxMethodSpan = 1 << 16
	// maxMethodSymbols 限制方法表条数。实测 Unity 2022 手机包约 33 万条；
	// 反复开关 Call Stacks 会重复下发，去重后仍超上限就丢弃，避免内存无界。
	maxMethodSymbols = 500_000
)

// methodTable 是地址 → 方法名的查询表，按需排序去重。
type methodTable struct {
	syms   []methodSymbol
	sorted bool
	// unique 是上次去重后的条数，用于判断是否该提前去重。
	unique  int
	dropped int
}

// add 追加一条方法符号；同一地址重复下发时后者生效。
func (t *methodTable) add(addr uint64, name string) {
	if addr == 0 || name == "" {
		return
	}
	if len(t.syms) >= maxMethodSymbols {
		t.dropped++
		return
	}
	t.syms = append(t.syms, methodSymbol{Addr: addr, Name: name})
	t.sorted = false
	// 反复开关会把整张表再发一遍，这里及时去重以免堆积。
	if len(t.syms) > 2*t.unique+1024 {
		t.compact()
	}
}

func (t *methodTable) compact() {
	sort.SliceStable(t.syms, func(i, j int) bool { return t.syms[i].Addr < t.syms[j].Addr })
	out := t.syms[:0]
	for i, s := range t.syms {
		// 同地址只留最后一条（最近一次下发）。
		if i+1 < len(t.syms) && t.syms[i+1].Addr == s.Addr {
			continue
		}
		out = append(out, s)
	}
	t.syms = out
	t.unique = len(out)
	t.sorted = true
}

// resolve 把调用栈地址还原为方法名；表内没有覆盖该地址的方法时返回 false。
func (t *methodTable) resolve(addr uint64) (string, bool) {
	if !t.sorted {
		t.compact()
	}
	if len(t.syms) == 0 {
		return "", false
	}
	i := sort.Search(len(t.syms), func(k int) bool { return t.syms[k].Addr > addr }) - 1
	if i < 0 || addr-t.syms[i].Addr > maxMethodSpan {
		return "", false
	}
	return t.syms[i].Name, true
}

// metaTables 保存元数据流建立的查询表。
type metaTables struct {
	markers    map[uint32]markerInfo
	categories map[uint16]string
	threads    map[int64]string
	methods    methodTable
}

func newMetaTables() *metaTables {
	return &metaTables{
		markers:    make(map[uint32]markerInfo),
		categories: make(map[uint16]string),
		threads:    make(map[int64]string),
	}
}

func (m *metaTables) categoryName(id uint16) string {
	if n, ok := m.categories[id]; ok {
		return n
	}
	return fmt.Sprintf("Category%d", id)
}

// counterKey 以 `分类/名字` 定位计数器，避免不同分类下的同名计数器互相覆盖。
func (m *metaTables) counterKey(id uint32) string {
	info, ok := m.markers[id]
	if !ok {
		return ""
	}
	return m.categoryName(info.Category) + "/" + info.Name
}

// threadStream 是一条线程流的重组状态。
type threadStream struct {
	id                int64
	workerFrame       *frameAccum
	buf               []byte
	nextSeq           int32
	haveSeq           bool
	desynced          bool
	waitingForFrame   bool
	lastFrameIndex    uint32
	lastFrameNS       int64
	haveFrameBoundary bool
	// isMain 标识主线程流：只有它决定帧边界、根采样时长与分类自耗时。
	isMain bool
	// isMeta 标识元数据流：它失步等于查询表不可用，必须视为整体失败。
	isMeta bool
	// stack 是采样栈，用于计算各分类自耗时。
	stack []sampleEntry
}

type sampleEntry struct {
	markerID uint32
	startNS  int64
	childNS  int64
	kids     []*treeNode
	meta     []value
	// callstack 是 Call Stacks 开启后 Player 上报的代码地址，尚未解析为方法名。
	callstack []uint64
}

// treeNode 是一帧主线程采样树中的一个节点（尚未合并同名兄弟）。
type treeNode struct {
	markerID  uint32
	startNS   int64
	totalNS   int64
	selfNS    int64
	kids      []*treeNode
	meta      []value
	callstack []uint64
}

// DecodeStats 是解码过程的可观测指标，便于自检与排障。
type DecodeStats struct {
	Chunks              int
	MetaRecords         int
	EventRecords        int
	Frames              int
	DroppedChunks       int
	MissingMainChunks   uint64
	DiscardedMainChunks int
	FrameResyncs        int
	WaitingForFrame     bool
	SkippedThreads      int
	// DesyncedThreads 是被放弃的非主线程流条数；它们只贡献计数器，丢掉不影响逐帧指标。
	DesyncedThreads int
	Desynced        bool
	LastError       string
	UnknownRecords  map[string]int
	MissingCounters []string
}

// SnapshotHandler 在解析出一帧时被调用。
type SnapshotHandler func(frame FrameSnapshot)

// FrameSnapshot 是一帧的解析结果，字段与 UI 展示的指标一一对应。
type FrameSnapshot struct {
	// Index 是 Player 侧的帧序号。
	Index uint32
	// StartNS / DurationNS 是该帧在 Player 时钟上的起点与跨度。
	StartNS    int64
	DurationNS int64
	// RootNS 是主线程根采样（Main Thread）的时长。
	RootNS int64
	// CategoryNS 是各 ProfilerCategory 的自耗时，合计等于 RootNS。
	CategoryNS map[string]int64
	// Counters 是该帧的计数器取值，键为 `分类/名字`。
	Counters map[string]float64
	// Hierarchy 是主线程 CPU 采样树，Deep Profile 开启后会包含脚本方法。
	Hierarchy []protocol.SampleNode
	Timeline  *profileframe.Timeline
}

// Decoder 把 Profiler 原始数据流解成逐帧数据。
//
// 帧边界、根采样时长与分类自耗时只取自主线程流；其余线程流仅贡献计数器，
// 因为渲染统计（Draw Calls 等）由渲染线程上报。任一非主线程流失步只会被单独
// 放弃，不影响逐帧指标。
type Decoder struct {
	header        StreamHeader
	haveHead      bool
	meta          *metaTables
	streams       map[int64]*threadStream
	frame         *frameAccum
	onFrame       SnapshotHandler
	stats         DecodeStats
	metaDone      bool
	skipTIDs      map[int64]bool
	timelines     map[uint32]*profileframe.Timeline
	timelineOrder []uint32
}

type frameAccum struct {
	index      uint32
	startNS    int64
	rootNS     int64
	categoryNS map[uint16]int64
	counters   map[uint32]float64
	roots      []*treeNode
}

// NewDecoder 创建解码器。
func NewDecoder(onFrame SnapshotHandler) *Decoder {
	return &Decoder{
		meta:      newMetaTables(),
		streams:   make(map[int64]*threadStream),
		onFrame:   onFrame,
		skipTIDs:  make(map[int64]bool),
		timelines: make(map[uint32]*profileframe.Timeline),
		stats:     DecodeStats{UnknownRecords: make(map[string]int)},
	}
}

// Header 返回流头（尚未收到时 haveHead 为 false）。
func (d *Decoder) Header() (StreamHeader, bool) { return d.header, d.haveHead }

// Stats 返回解码指标快照。
func (d *Decoder) Stats() DecodeStats {
	out := d.stats
	out.UnknownRecords = make(map[string]int, len(d.stats.UnknownRecords))
	for k, v := range d.stats.UnknownRecords {
		out.UnknownRecords[k] = v
	}
	return out
}

// Feed 处理一条 MsgIDProfilerData 报文体。
func (d *Decoder) Feed(body []byte) error {
	if len(body) >= 4 && binary.LittleEndian.Uint32(body[0:4]) == streamMagic {
		h, err := ParseStreamHeader(body)
		if err != nil {
			return err
		}
		d.header = h
		d.haveHead = true
		return nil
	}
	chunk, err := ParseChunk(body)
	if err != nil {
		d.stats.DroppedChunks++
		d.stats.LastError = err.Error()
		return err
	}
	d.stats.Chunks++

	isMeta := chunk.ThreadID == metaThreadID
	if !isMeta && !d.haveHead {
		// 流头未到时无从判断主线程，先丢弃：主线程流在流头之后才开始发数据。
		if !d.skipTIDs[chunk.ThreadID] {
			d.skipTIDs[chunk.ThreadID] = true
			d.stats.SkippedThreads++
		}
		return nil
	}

	st := d.streams[chunk.ThreadID]
	if st == nil {
		st = &threadStream{id: chunk.ThreadID, isMeta: isMeta, isMain: !isMeta && chunk.ThreadID == d.header.MainThreadID}
		d.streams[chunk.ThreadID] = st
	}
	if st.desynced {
		return nil
	}
	if st.haveSeq && chunk.Seq != st.nextSeq {
		d.stats.LastError = fmt.Sprintf("线程 %#x 分块序号断裂：收到 %d，期望 %d", chunk.ThreadID, chunk.Seq, st.nextSeq)
		missing := uint32(chunk.Seq) - uint32(st.nextSeq)
		if st.isMain && d.header.FormatVersion == protocol.Tuanjie2022StreamVersion && missing < 1<<31 {
			d.stats.MissingMainChunks += uint64(missing)
			d.waitForTuanjieFrame(st)
		} else {
			st.desynced = true
			if !isMeta && !st.isMain {
				d.stats.DesyncedThreads++
				return nil
			}
			d.stats.Desynced = true
			return errors.New(d.stats.LastError)
		}
	}
	st.haveSeq = true
	st.nextSeq = chunk.Seq + 1
	if st.waitingForFrame {
		if !st.isTuanjieFrameStart(chunk.Payload) {
			d.stats.DiscardedMainChunks++
			return nil
		}
		st.waitingForFrame = false
		d.stats.WaitingForFrame = false
		d.stats.FrameResyncs++
	}
	st.buf = append(st.buf, chunk.Payload...)

	if d.header.FormatVersion == protocol.Tuanjie2022StreamVersion {
		if isMeta {
			d.drain(st, d.decodeTuanjieMeta)
		} else {
			d.drain(st, d.decodeTuanjieEvent)
		}
	} else if isMeta {
		d.drain(st, d.decodeMetaRecord)
	} else {
		d.drain(st, d.decodeEventRecord)
	}
	return nil
}

// drain 反复解析记录直到数据不足；未消费的尾部留待下一块。
func (d *Decoder) drain(st *threadStream, decode func(*threadStream, *cursor) error) {
	c := cursor{b: st.buf}
	for c.hasMore() {
		start := c.p
		if err := d.decodeOne(st, &c, decode); err != nil {
			if errors.Is(err, errShortRecord) {
				c.p = start
				break
			}
			st.desynced = true
			d.stats.LastError = fmt.Sprintf("%v（分块 %d，偏移 %d）", err, st.nextSeq-1, start)
			if st.isMain || st.isMeta {
				d.stats.Desynced = true
			} else {
				d.stats.DesyncedThreads++
			}
			c.p = len(c.b)
			break
		}
	}
	if c.p > 0 {
		st.buf = append(st.buf[:0], st.buf[c.p:]...)
	}
}

func (d *Decoder) decodeOne(st *threadStream, c *cursor, decode func(*threadStream, *cursor) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
				return
			}
			panic(r)
		}
	}()
	return decode(st, c)
}

func (d *Decoder) decodeMetaRecord(_ *threadStream, c *cursor) error {
	t := c.u8()
	switch t {
	case recMetaMarker:
		c.u8()
		id := c.u32()
		info := markerInfo{Flags: c.u16(), Category: c.u16()}
		info.Name = c.str()
		info.MetaCount = int(c.u8())
		for i := 0; i < info.MetaCount; i++ {
			c.u8() // dataType
			c.u8() // unit
			info.recordField(i, c.str())
		}
		d.meta.markers[id] = info
	case recMetaCategory:
		c.u8()
		id := c.u16()
		c.u16() // color
		c.u16() // flags
		d.meta.categories[id] = c.str()
	case recMetaThread:
		d.parseThreadDef(c)
	case recMetaClass:
		c.u8()
		c.i32()
		c.i32()
		c.str()
	case recMetaObject:
		c.u8()
		c.i32()
		c.u32()
		c.str()
		c.skip(24)
	case recMetaObject2:
		c.u8()
		c.skip(24)
	case recMetaFlush:
		c.u8()
		c.skip(16)
		d.metaDone = true
	case recMetaCatFlag:
		c.u8()
		c.skip(4)
	case recMetaMethod:
		c.u8()
		c.skip(8) // 实测恒为 0
		addr := uint64(c.i64())
		c.u32() // 实测恒为 0
		name := c.str()
		c.str() // 源文件名，IL2CPP 包为空
		c.u32() // 源文件行号，IL2CPP 包为 0
		d.meta.methods.add(addr, name)
	default:
		return fmt.Errorf("unityprofiler: 元数据流未知记录类型 0x%02x", t)
	}
	d.stats.MetaRecords++
	return nil
}

func (d *Decoder) decodeEventRecord(st *threadStream, c *cursor) error {
	t := c.u8()
	switch t {
	case recFrame:
		c.u8()
		index := c.u32()
		ts := c.i64()
		// 每条线程流都有帧记录，但只让主线程划定帧边界，否则一帧会被切成多段。
		if st.isMain {
			d.rollFrame(index, ts, st)
		} else {
			d.rollWorkerFrame(index, ts, st)
		}
	case recMetaThread:
		// 工作线程也会在事件流里再发一遍线程定义（与元数据流同布局）。
		d.parseThreadDef(c)
	case recBeginPlain, recBeginExtra, recBeginMeta, recBeginObject:
		c.u16()
		id := c.u32()
		ts := c.i64()
		if t == recBeginExtra {
			c.u32()
		}
		st.stack = append(st.stack, sampleEntry{markerID: id, startNS: ts})
	case recEnd:
		c.u16()
		c.u32()
		ts := c.i64()
		d.closeSample(st, ts)
	case recCounter:
		c.u16()
		id := c.u32()
		c.i64()
		vals := c.values(int(c.u8()))
		// 计数器来自所有线程（渲染统计走渲染线程），一并记到当前帧上；渲染线程滞后
		// 主线程不足一帧，按当前帧归集与 Editor 显示一致。
		if d.frame != nil {
			for _, v := range vals {
				if n, ok := v.Number(); ok {
					d.frame.counters[id] = n
					break
				}
			}
		}
	case recInstanceHint:
		c.u16()
		c.u32()
	case recThreadClock:
		c.u8()
		c.i64()
	case recCallstack:
		// 与元数据取值列表撞号：调用栈记录第二个字节固定为 0，而取值类型不会是 0（None）。
		if !c.hasMore() || c.peek() != 0x00 {
			attachMeta(st, c.values(int(t)))
			break
		}
		c.u8()
		c.u32() // 实测恒为 0
		depth := int(c.u8())
		addrs := make([]uint64, 0, depth)
		for i := 0; i < depth; i++ {
			addrs = append(addrs, uint64(c.i64()))
		}
		// 调用栈属于当前打开的采样：Player 会先关掉 Profiler.Callstack 这个采集开销采样，
		// 再写出调用栈，此时栈顶正是发生分配的那个采样（GC.Alloc / UnsafeUtility.Malloc）。
		if len(st.stack) > 0 {
			top := &st.stack[len(st.stack)-1]
			top.callstack = append(top.callstack, addrs...)
		}
	case recLegacyStats:
		// 与元数据取值列表撞号：Profiler.CollectLegacyStats 里的定长统计结构以 0x00 开头。
		if !c.hasMore() || c.peek() != 0x00 {
			attachMeta(st, c.values(int(t)))
			break
		}
		c.skip(legacyStatsSize)
	case recAudioStats:
		if !c.hasMore() || c.peek() != 0x00 {
			attachMeta(st, c.values(int(t)))
			break
		}
		c.skip(audioStatsHeadSize)
		c.str()
	default:
		if t >= 0x01 && t <= 0x0F {
			// 记录类型即随后的元数据取值个数（Begin 之后的 GC.Alloc 大小、对象名等）。
			attachMeta(st, c.values(int(t)))
			break
		}
		key := fmt.Sprintf("0x%02x", t)
		d.stats.UnknownRecords[key]++
		return fmt.Errorf("unityprofiler: 事件流未知记录类型 %s", key)
	}
	d.stats.EventRecords++
	return nil
}

func (d *Decoder) parseThreadDef(c *cursor) {
	c.u8()
	tid := c.i64()
	c.i64() // 起始时间戳
	c.u32()
	group := c.str()
	name := c.str()
	if group != "" {
		name = group + "/" + name
	}
	d.meta.threads[tid] = name
}

// attachMeta 把元数据取值挂到当前打开的采样上。
func attachMeta(st *threadStream, vals []value) {
	if len(vals) == 0 || len(st.stack) == 0 {
		return
	}
	top := &st.stack[len(st.stack)-1]
	top.meta = append(top.meta, cloneValues(vals)...)
}

func (d *Decoder) closeSample(st *threadStream, endNS int64) {
	if len(st.stack) == 0 {
		return
	}
	top := st.stack[len(st.stack)-1]
	st.stack = st.stack[:len(st.stack)-1]
	dur := endNS - top.startNS
	if dur < 0 {
		dur = 0
	}
	if len(st.stack) > 0 {
		st.stack[len(st.stack)-1].childNS += dur
	}
	self := dur - top.childNS
	if self < 0 {
		self = 0
	}
	node := &treeNode{markerID: top.markerID, startNS: top.startNS, totalNS: dur, selfNS: self,
		kids: top.kids, meta: top.meta, callstack: top.callstack}
	if len(st.stack) > 0 {
		parent := &st.stack[len(st.stack)-1]
		parent.kids = append(parent.kids, node)
	}
	if !st.isMain {
		if st.workerFrame != nil && len(st.stack) == 0 {
			st.workerFrame.roots = append(st.workerFrame.roots, node)
		}
		return
	}
	if d.frame == nil {
		return
	}
	if info, ok := d.meta.markers[top.markerID]; ok {
		d.frame.categoryNS[info.Category] += self
	}
	if len(st.stack) == 0 {
		d.frame.roots = append(d.frame.roots, node)
		if dur > d.frame.rootNS {
			d.frame.rootNS = dur
		}
	}
}

// maxOpenSnapshotDepth 限制跨帧未闭合采样链写入 Hierarchy 的层数。
// 卡顿帧上未配对的 Begin 可能堆到上万层，全量展开会撑爆递归与 UI。
const maxOpenSnapshotDepth = 64

// rollFrame 收尾上一帧并开启新帧。
func (d *Decoder) rollFrame(index uint32, ts int64, st *threadStream) {
	if prev := d.frame; prev != nil {
		d.flushOpenSamples(st, prev, ts)
		prev.emit(d, ts)
	}
	d.frame = &frameAccum{
		index:      index,
		startNS:    ts,
		categoryNS: make(map[uint16]int64),
		counters:   make(map[uint32]float64),
	}
}

// flushOpenSamples 在帧边界把仍未 End 的主线程采样写入当前帧 Hierarchy。
// Unity 的 Main Thread / PlayerLoop 会跨帧保持打开：只在栈弹空时才记 roots 的话，
// 卡顿帧只会留下分类自耗时，Hierarchy 是空的。
func (d *Decoder) flushOpenSamples(st *threadStream, frame *frameAccum, endNS int64) {
	if st == nil || !st.isMain || len(st.stack) == 0 || frame == nil {
		return
	}
	root := d.snapshotOpenStack(st.stack, frame, endNS)
	if root == nil {
		return
	}
	frame.roots = append(frame.roots, root)
	if root.totalNS > frame.rootNS {
		frame.rootNS = root.totalNS
	}
	resetOpenStack(st, endNS)
}

func resetOpenStack(st *threadStream, endNS int64) {
	for i := range st.stack {
		st.stack[i].startNS = endNS
		st.stack[i].childNS = 0
		st.stack[i].kids = nil
		st.stack[i].meta = nil
		st.stack[i].callstack = nil
	}
}

func (d *Decoder) snapshotOpenStack(stack []sampleEntry, frame *frameAccum, endNS int64) *treeNode {
	limit := len(stack)
	if limit > maxOpenSnapshotDepth {
		limit = maxOpenSnapshotDepth
	}
	var inner *treeNode
	for i := limit - 1; i >= 0; i-- {
		e := stack[i]
		start := e.startNS
		if frame.startNS != 0 && start < frame.startNS {
			start = frame.startNS
		}
		dur := endNS - start
		if dur < 0 {
			dur = 0
		}
		openChildNS := int64(0)
		if inner != nil {
			openChildNS = inner.totalNS
		} else if i == limit-1 && len(stack) > limit {
			nextStart := stack[limit].startNS
			if frame.startNS != 0 && nextStart < frame.startNS {
				nextStart = frame.startNS
			}
			openChildNS = endNS - nextStart
			if openChildNS < 0 {
				openChildNS = 0
			}
		}
		self := dur - e.childNS - openChildNS
		if self < 0 {
			self = 0
		}
		if info, ok := d.meta.markers[e.markerID]; ok && frame.categoryNS != nil {
			frame.categoryNS[info.Category] += self
		}
		kids := make([]*treeNode, len(e.kids), len(e.kids)+1)
		copy(kids, e.kids)
		if inner != nil {
			kids = append(kids, inner)
		}
		inner = &treeNode{
			markerID:  e.markerID,
			startNS:   start,
			totalNS:   dur,
			selfNS:    self,
			kids:      kids,
			meta:      cloneValues(e.meta),
			callstack: cloneAddrs(e.callstack),
		}
	}
	return inner
}

func (a *frameAccum) emit(d *Decoder, endNS int64) {
	d.stats.Frames++
	if d.onFrame == nil {
		return
	}
	snap := FrameSnapshot{
		Index:      a.index,
		StartNS:    a.startNS,
		DurationNS: endNS - a.startNS,
		RootNS:     a.rootNS,
		CategoryNS: make(map[string]int64, len(a.categoryNS)),
		Counters:   make(map[string]float64, len(a.counters)),
		Hierarchy:  buildHierarchy(d, a.roots, a.startNS),
		Timeline:   d.timelineFor(a.index),
	}
	for cat, ns := range a.categoryNS {
		snap.CategoryNS[d.meta.categoryName(cat)] += ns
	}
	for id, v := range a.counters {
		if key := d.meta.counterKey(id); key != "" {
			snap.Counters[key] = v
		}
	}
	d.onFrame(snap)
}

// CounterNames 返回已知计数器的 `分类/名字` 列表，便于排查缺失指标。
func (d *Decoder) CounterNames() []string {
	var out []string
	for id, info := range d.meta.markers {
		if info.IsCounter() {
			out = append(out, d.meta.counterKey(id))
		}
	}
	sort.Strings(out)
	return out
}

// ThreadNames 返回线程 ID 到名字的映射快照。
func (d *Decoder) ThreadNames() map[int64]string {
	out := make(map[int64]string, len(d.meta.threads))
	for k, v := range d.meta.threads {
		out[k] = v
	}
	return out
}

// MainThreadName 返回主线程名（用于自检输出）。
func (d *Decoder) MainThreadName() string {
	if !d.haveHead {
		return ""
	}
	if n, ok := d.meta.threads[d.header.MainThreadID]; ok {
		return strings.TrimPrefix(n, "/")
	}
	return ""
}
