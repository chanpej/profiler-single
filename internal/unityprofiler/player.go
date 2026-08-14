package unityprofiler

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

// Player 是协议单测用的假 Unity 2022 Development Player：走真实 PlayerConnection
// 报文与 Profiler 原始数据流（PD3U + 分块 + 记录），因此单测覆盖的是真机同一套解码路径。
type Player struct {
	// EngineVersion 各字段对应引擎版本号，默认 2022.3.47f1。
	Major, Minor, Patch, ReleaseType, ReleaseNumber uint32
	// StreamVersion 是数据流格式版本，默认 Unity2022StreamVersion。
	StreamVersion uint32
	// ProjectName / DeviceID 出现在宣告报文里。
	ProjectName string
	DeviceID    string
	// Flags 是宣告 Flags，默认置 SupportsProfile。
	Flags int
	// Interval 是发帧间隔。
	Interval time.Duration
	// ChunkSize 是分块负载上限，故意取小值以覆盖跨块记录重组。
	ChunkSize int
	// Announce 决定是否向组播宣告。
	Announce bool
	// HeaderBeforeAnnounce 模拟"连接时 Player 已在采集"：数据流头先于宣告到达。
	HeaderBeforeAnnounce bool

	ln     net.Listener
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// 假 Player 使用的分类、marker 与计数器编号。
const (
	fakeMainThreadID int64 = 0x72850ced50
	// fakeRenderThreadID 让假 Player 也从渲染线程上报渲染统计，与真机一致。
	fakeRenderThreadID int64 = 0x72850ced88

	catRender     uint16 = 0
	catScripts    uint16 = 1
	catGui        uint16 = 4
	catPhysics    uint16 = 5
	catAnimation  uint16 = 6
	catGC         uint16 = 17
	catVSync      uint16 = 18
	catPlayerLoop uint16 = 20
	catMemory     uint16 = 23
	catInternal   uint16 = 24
	catAudio      uint16 = 25
	catUIDetails  uint16 = 26

	mkMainThread uint32 = 100
	mkRender     uint32 = 110
	mkScripts    uint32 = 111
	mkPhysics    uint32 = 112
	mkAnimation  uint32 = 113
	mkGC         uint32 = 114
	mkVSync      uint32 = 115
	mkUI         uint32 = 116

	ctrIDCPUTotal   uint32 = 200
	ctrIDCPUMain    uint32 = 201
	ctrIDCPURender  uint32 = 202
	ctrIDGPU        uint32 = 203
	ctrIDDrawCalls  uint32 = 210
	ctrIDSetPass    uint32 = 211
	ctrIDBatches    uint32 = 212
	ctrIDTriangles  uint32 = 213
	ctrIDVertices   uint32 = 214
	ctrIDShadowCast uint32 = 215

	ctrIDPlayingSrc uint32 = 250
	ctrIDAudioVoice uint32 = 251
	ctrIDDynBodies  uint32 = 252
	ctrIDColliders  uint32 = 253
	ctrIDUIBatches  uint32 = 254
	ctrIDUIVertices uint32 = 255
	ctrIDTotalUsed  uint32 = 220
	ctrIDTotalRsvd  uint32 = 221
	ctrIDGCUsed     uint32 = 222
	ctrIDGCRsvd     uint32 = 223
	ctrIDGfxUsed    uint32 = 224
	ctrIDGfxRsvd    uint32 = 225
	ctrIDAudioUsed  uint32 = 226
	ctrIDVideoUsed  uint32 = 227
	ctrIDProfUsed   uint32 = 228
	ctrIDProfRsvd   uint32 = 229
	ctrIDSystemUsed uint32 = 230
	ctrIDGCAlloc    uint32 = 231
)

// NewPlayer 创建默认的假 Unity 2022.3 Player。
func NewPlayer() *Player {
	return &Player{
		Major: 2022, Minor: 3, Patch: 47, ReleaseType: 2, ReleaseNumber: 1,
		StreamVersion: Unity2022StreamVersion,
		ProjectName:   "UnityPerfAgentSim",
		DeviceID:      "AndroidPlayer(11,UnityPerfAgent_Simulator@127.0.0.1)",
		Flags:         FlagSupportsProfile,
		Interval:      5 * time.Millisecond,
		ChunkSize:     512,
	}
}

// EngineVersion 返回该假 Player 自报的引擎版本字符串。
func (p *Player) EngineVersion() string {
	return formatEngineVersion(p.Major, p.Minor, p.Patch, p.ReleaseType, p.ReleaseNumber)
}

// Start 在 127.0.0.1 随机端口监听并返回端口号。
func (p *Player) Start() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.ln = ln
	p.cancel = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.serve(ctx, conn)
			}()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	if p.Announce {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.announceLoop(ctx, port)
		}()
	}
	return port, nil
}

// Stop 关闭监听与全部会话。
func (p *Player) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.ln != nil {
		p.ln.Close()
	}
	p.wg.Wait()
}

func (p *Player) info(port int) PlayerInfo {
	return PlayerInfo{
		IP:          "127.0.0.1",
		Port:        port,
		Flags:       p.Flags,
		GUID:        "1",
		ID:          p.DeviceID,
		PackageName: "AndroidPlayer",
		ProjectName: p.ProjectName,
	}
}

func (p *Player) serve(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	port := 0
	if p.ln != nil {
		port = p.ln.Addr().(*net.TCPAddr).Port
	}
	if err := WriteMessage(conn, MsgIDPlayerInfo,
		EncodePlayerEngineInfo(platformAndroid, p.Major, p.Minor, p.Patch, p.ReleaseType, p.ReleaseNumber)); err != nil {
		return
	}

	streamVersion := p.StreamVersion
	if streamVersion == 0 {
		streamVersion = Unity2022StreamVersion
	}
	header := EncodeStreamHeader(StreamHeader{
		FormatVersion: streamVersion,
		MainThreadID:  fakeMainThreadID,
	}, p.Major, p.Minor, p.Patch, p.ReleaseType, p.ReleaseNumber)
	announce := append([]byte(FormatAnnouncement(p.info(port))), 0)

	// 已在采集中的 Player 会先吐数据流头再补宣告，握手期必须保住这个头。
	if p.HeaderBeforeAnnounce {
		if err := WriteMessage(conn, MsgIDProfilerData, header); err != nil {
			return
		}
		if err := WriteMessage(conn, MsgIDAnnounce, announce); err != nil {
			return
		}
	} else {
		if err := WriteMessage(conn, MsgIDAnnounce, announce); err != nil {
			return
		}
		if err := WriteMessage(conn, MsgIDProfilerData, header); err != nil {
			return
		}
	}

	var metaSeq, mainSeq, renderSeq int32
	var meta RecordWriter
	p.writeMetadata(&meta)
	if err := p.sendStream(conn, metaThreadID, &metaSeq, meta.Bytes()); err != nil {
		return
	}

	interval := p.Interval
	if interval <= 0 {
		interval = 5 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	gen := newFrameGenerator(rand.New(rand.NewSource(time.Now().UnixNano())))
	var events, renderEvents RecordWriter
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events.Reset()
			renderEvents.Reset()
			gen.writeFrame(&events, &renderEvents)
			// 主线程块先发以打开当前帧，渲染统计随后并入这一帧。
			if err := p.sendStream(conn, fakeMainThreadID, &mainSeq, events.Bytes()); err != nil {
				return
			}
			if err := p.sendStream(conn, fakeRenderThreadID, &renderSeq, renderEvents.Bytes()); err != nil {
				return
			}
		}
	}
}

// sendStream 把一段线程流字节切成分块逐条发出，块边界故意落在记录内部。
func (p *Player) sendStream(conn net.Conn, threadID int64, seq *int32, data []byte) error {
	size := p.ChunkSize
	if size <= 0 {
		size = 512
	}
	for off := 0; off < len(data); off += size {
		end := off + size
		if end > len(data) {
			end = len(data)
		}
		if err := WriteMessage(conn, MsgIDProfilerData, EncodeChunk(*seq, threadID, data[off:end])); err != nil {
			return err
		}
		*seq++
	}
	return nil
}

func (p *Player) writeMetadata(w *RecordWriter) {
	for id, name := range map[uint16]string{
		catRender: "Render", catScripts: "Scripts", catGui: "Gui", catPhysics: "Physics",
		catAnimation: "Animation", catGC: "GC", catVSync: "VSync",
		catPlayerLoop: "PlayerLoop", catMemory: "Memory", catInternal: "Internal",
		catAudio: "Audio", catUIDetails: "UI Details",
	} {
		w.CategoryDef(id, name)
	}
	w.ThreadDef(fakeMainThreadID, 0, "", "Main Thread")
	w.ThreadDef(fakeRenderThreadID, 0, "", "Render Thread")

	w.MarkerDef(mkMainThread, 0x08, catInternal, "Main Thread")
	w.MarkerDef(mkRender, 0, catRender, "Camera.Render")
	w.MarkerDef(mkScripts, 0, catScripts, "BehaviourUpdate")
	w.MarkerDef(mkPhysics, 0, catPhysics, "Physics.Simulate")
	w.MarkerDef(mkAnimation, 0, catAnimation, "Animators.Update")
	w.MarkerDef(mkGC, 0, catGC, "GC.Collect")
	w.MarkerDef(mkVSync, 0, catVSync, "WaitForTargetFPS")
	w.MarkerDef(mkUI, 0, catGui, "UI.Repaint")

	w.CounterDef(ctrIDCPUTotal, catRender, "CPU Total Frame Time", DataTypeDouble, UnitTimeNanoseconds)
	w.CounterDef(ctrIDCPUMain, catRender, "CPU Main Thread Frame Time", DataTypeDouble, UnitTimeNanoseconds)
	w.CounterDef(ctrIDCPURender, catRender, "CPU Render Thread Frame Time", DataTypeDouble, UnitTimeNanoseconds)
	w.CounterDef(ctrIDGPU, catRender, "GPU Frame Time", DataTypeDouble, UnitTimeNanoseconds)
	w.CounterDef(ctrIDDrawCalls, catRender, "Draw Calls Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDSetPass, catRender, "SetPass Calls Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDBatches, catRender, "Batches Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDTriangles, catRender, "Triangles Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDVertices, catRender, "Vertices Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDShadowCast, catRender, "Shadow Casters Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDPlayingSrc, catAudio, "Playing Audio Sources", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDAudioVoice, catAudio, "Total Audio Voices", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDDynBodies, catPhysics, "Active Dynamic Bodies", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDColliders, catPhysics, "Static Colliders", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDUIBatches, catUIDetails, "Batches Count", DataTypeInt32, UnitCount)
	w.CounterDef(ctrIDUIVertices, catUIDetails, "Vertices Count", DataTypeInt32, UnitCount)
	for id, name := range map[uint32]string{
		ctrIDTotalUsed: "Total Used Memory", ctrIDTotalRsvd: "Total Reserved Memory",
		ctrIDGCUsed: "GC Used Memory", ctrIDGCRsvd: "GC Reserved Memory",
		ctrIDGfxUsed: "Gfx Used Memory", ctrIDGfxRsvd: "Gfx Reserved Memory",
		ctrIDAudioUsed: "Audio Used Memory", ctrIDVideoUsed: "Video Used Memory",
		ctrIDProfUsed: "Profiler Used Memory", ctrIDProfRsvd: "Profiler Reserved Memory",
		ctrIDSystemUsed: "System Used Memory", ctrIDGCAlloc: "GC Allocated In Frame",
	} {
		w.CounterDef(id, catMemory, name, DataTypeInt64, UnitBytes)
	}
	w.MetaFlush(0)
}

// frameGenerator 生成一帧的事件序列，各分类耗时之和等于主线程根采样时长。
type frameGenerator struct {
	rng   *rand.Rand
	index uint32
	nowNS int64
	memMB float64
}

func newFrameGenerator(rng *rand.Rand) *frameGenerator {
	return &frameGenerator{rng: rng, nowNS: 1_000_000_000, memMB: 480}
}

// FrameShape 是假 Player 生成的一帧目标形态，单测用它校验解码结果。
type FrameShape struct {
	TotalMs    float64
	RenderMs   float64
	ScriptsMs  float64
	PhysicsMs  float64
	AnimMs     float64
	GCMs       float64
	VSyncMs    float64
	UIMs       float64
	DrawCalls  int32
	UsedMemMB  float64
	GCAllocKB  float64
	GPUMs      float64
	MainMs     float64
	RenderThrM float64
	// 音频/物理/UI 模块的计数器取值。
	PlayingSources int32
	ActiveBodies   int32
	UIBatches      int32
}

func (g *frameGenerator) shape() FrameShape {
	total := 16.6 + 3*math.Sin(float64(g.index)/90) + g.rng.Float64()*2
	s := FrameShape{
		TotalMs:   total,
		RenderMs:  total * 0.30,
		ScriptsMs: total * 0.24,
		PhysicsMs: total * 0.10,
		AnimMs:    total * 0.08,
		GCMs:      total * 0.03,
		VSyncMs:   total * 0.05,
		UIMs:      total * 0.08,
		DrawCalls: int32(300 + g.rng.Intn(80)),
		GCAllocKB: 2 + g.rng.Float64()*8,

		PlayingSources: int32(4 + g.rng.Intn(6)),
		ActiveBodies:   int32(12 + g.rng.Intn(20)),
		UIBatches:      int32(8 + g.rng.Intn(10)),
	}
	g.memMB += 0.02
	s.UsedMemMB = g.memMB
	s.GPUMs = total * 0.7
	s.MainMs = total * 0.85
	s.RenderThrM = total * 0.35
	return s
}

// writeFrame 写出一帧完整事件序列：主线程流写采样与内存计数器，渲染线程流写渲染统计
// （与真机一致，渲染统计不在主线程流上）。
func (g *frameGenerator) writeFrame(w, render *RecordWriter) FrameShape {
	g.index++
	s := g.shape()
	ms := func(v float64) int64 { return int64(v * 1e6) }

	start := g.nowNS
	w.FrameBoundary(g.index, start)
	w.Begin(mkMainThread, start)

	at := start
	for _, seg := range []struct {
		id uint32
		ms float64
	}{
		{mkRender, s.RenderMs}, {mkScripts, s.ScriptsMs}, {mkPhysics, s.PhysicsMs},
		{mkAnimation, s.AnimMs}, {mkGC, s.GCMs}, {mkVSync, s.VSyncMs}, {mkUI, s.UIMs},
	} {
		w.Begin(seg.id, at)
		at += ms(seg.ms)
		w.End(seg.id, at)
	}

	end := start + ms(s.TotalMs)
	if end < at {
		end = at
	}
	w.CounterDouble(ctrIDCPUTotal, at, s.TotalMs*1e6)
	w.CounterDouble(ctrIDCPUMain, at, s.MainMs*1e6)
	w.CounterDouble(ctrIDCPURender, at, s.RenderThrM*1e6)
	w.CounterDouble(ctrIDGPU, at, s.GPUMs*1e6)
	render.FrameBoundary(g.index, start)
	render.CounterInt32(ctrIDDrawCalls, at, s.DrawCalls)
	render.CounterInt32(ctrIDSetPass, at, s.DrawCalls/6)
	render.CounterInt32(ctrIDBatches, at, s.DrawCalls*8/10)
	render.CounterInt32(ctrIDTriangles, at, s.DrawCalls*3000)
	render.CounterInt32(ctrIDVertices, at, s.DrawCalls*2200)
	render.CounterInt32(ctrIDShadowCast, at, s.DrawCalls/10)

	w.CounterInt32(ctrIDPlayingSrc, at, s.PlayingSources)
	w.CounterInt32(ctrIDAudioVoice, at, s.PlayingSources*2)
	w.CounterInt32(ctrIDDynBodies, at, s.ActiveBodies)
	w.CounterInt32(ctrIDColliders, at, s.ActiveBodies*3)
	w.CounterInt32(ctrIDUIBatches, at, s.UIBatches)
	w.CounterInt32(ctrIDUIVertices, at, s.UIBatches*80)

	mb := func(v float64) int64 { return int64(v * bytesPerMB) }
	w.CounterInt64(ctrIDTotalUsed, at, mb(s.UsedMemMB))
	w.CounterInt64(ctrIDTotalRsvd, at, mb(s.UsedMemMB*1.4))
	w.CounterInt64(ctrIDGCUsed, at, mb(s.UsedMemMB*0.38))
	w.CounterInt64(ctrIDGCRsvd, at, mb(s.UsedMemMB*0.5))
	w.CounterInt64(ctrIDGfxUsed, at, mb(s.UsedMemMB*0.32))
	w.CounterInt64(ctrIDGfxRsvd, at, mb(s.UsedMemMB*0.38))
	w.CounterInt64(ctrIDAudioUsed, at, mb(6))
	w.CounterInt64(ctrIDVideoUsed, at, mb(0.2))
	w.CounterInt64(ctrIDProfUsed, at, mb(10))
	w.CounterInt64(ctrIDProfRsvd, at, mb(16))
	w.CounterInt64(ctrIDSystemUsed, at, mb(s.UsedMemMB*1.8))
	w.CounterInt64(ctrIDGCAlloc, at, int64(s.GCAllocKB*bytesPerKB))

	w.End(mkMainThread, end)
	g.nowNS = end
	// 下一条帧边界记录由后续帧写出；此处预留帧间空隙，使帧跨度等于 TotalMs。
	return s
}

func (p *Player) announceLoop(ctx context.Context, port int) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", MulticastGroup, MulticastPorts[0]))
	if err != nil {
		return
	}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer c.Close()
	msg := []byte(FormatAnnouncement(p.info(port)))
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = c.Write(msg)
		}
	}
}
