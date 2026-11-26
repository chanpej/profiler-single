package unityprofiler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"unityperf/internal/profileframe"
	"unityperf/internal/protocol"
)

// DialTimeout / HandshakeTimeout 与 Editor 连接 Player 的等待相当。
const (
	DialTimeout      = 5 * time.Second
	HandshakeTimeout = 5 * time.Second
	// maxChunksWithoutHeader 是允许在数据流头之前到达的分块数上限（约几秒的量）。
	maxChunksWithoutHeader = 2000
	// IdleTimeout 是读取循环每次等下一条报文的上限。超时后不断开、继续等：
	// Android 弹出系统对话框时游戏会 OnApplicationPause，Player 暂停上报，
	// 对话框关掉后同一条连接会继续吐帧。真正的对端关闭（EOF/RST）或用户点
	// 断开才会结束会话。采集中每秒有几十帧，正常绝不会触发这次等待。
	IdleTimeout = 10 * time.Second
	// EditorIdentity 是本工具在宣告报文里自报的身份，前缀与 Editor 一致，
	// 便于 Player 侧日志区分连接来源。
	EditorIdentity = "UnityEditor.Profiler"
)

// readIdleTimeout 是读取循环每次 Read 的截止时间，默认等于 IdleTimeout。
// 单测可改短，避免为「切后台再恢复」等十几秒。
var readIdleTimeout = IdleTimeout

// ErrNotConnected 表示尚未连上 Player。
var ErrNotConnected = errors.New("unityprofiler: not connected")

// ErrPlayerSilent 表示 TCP 已连上、开启报文也发出去了，但 Player 一个字节都没回。
// 绝大多数情况是 Player 的 Profiler 槽位被别的客户端占着（Unity Editor 的 Profiler，
// 或本工具/上次进程残留的未断开连接），此时连接会一直排在 Player 的等待队列里。
var ErrPlayerSilent = errors.New("unityprofiler: Player 接受了连接但未上报任何数据")

// PlayerRejectError 表示 Player 明确不支持 Profiler 采集。
type PlayerRejectError struct {
	Info   PlayerInfo
	Reason string
}

func (e *PlayerRejectError) Error() string {
	return fmt.Sprintf("unityprofiler: player %q 拒绝 Profiler 采集：%s", e.Info.ID, e.Reason)
}

// IsPlayerReject 判断错误是否为 Player 侧拒绝。
func IsPlayerReject(err error) bool {
	var target *PlayerRejectError
	return errors.As(err, &target)
}

// isTimeout 判断是否为读写超时（区别于连接被对端关闭）。
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// IsLostConnection 判断是否为对端把 TCP 关掉（EOF / RST），而不是解码失步。
// Android 弹出 native Activity 时 Unity 会直接断开 PlayerConnection。
func IsLostConnection(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if isTimeout(err) {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, n := range []string{
		"eof", "forcibly closed", "broken pipe", "connection reset",
		"wsarecv", "connection aborted", "connection refused",
	} {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

// FrameHandler receives one normalized in-memory Profiler frame. Both live and
// offline sources produce this model before their data reaches the UI session.
type FrameHandler func(frame profileframe.Frame)

// Session 是连到 Unity Development Player 的 Profiler 客户端。
type Session struct {
	// AreaMask 与 Categories 指定向 Player 申请的采集范围，零值表示沿用 Editor 实测默认。
	// 需在 Connect 之前设置。
	AreaMask   uint32
	Categories []string

	mu         sync.Mutex
	conn       net.Conn
	handshake  protocol.Handshake
	decoder    *Decoder
	cancel     context.CancelFunc
	done       chan struct{}
	deep       bool
	callStacks bool
	memory     *memoryTransfer
	onRaw      func([]byte)
	onLog      func(string, string)
}

// NewSession 创建未连接的会话。
func NewSession() *Session { return &Session{} }

func (s *Session) SetLogHandler(fn func(string, string)) {
	s.mu.Lock()
	s.onLog = fn
	s.mu.Unlock()
}

// SetRawHandler 注册原始 Profiler 报文体回调。每条 MsgIDProfilerData 在解码前原样送达。
// 回调不得重入本会话的锁。
func (s *Session) SetRawHandler(fn func([]byte)) {
	s.mu.Lock()
	s.onRaw = fn
	s.mu.Unlock()
}

// Connect 以 Editor Profiler 身份连到 Player 并开启采集。
//
// 与 Editor 一致：连上后立即下发 Profiler 开启报文，不等 Player 宣告；随后读取
// Player 的引擎信息与宣告，作为握手结果返回。
func (s *Session) Connect(host string, port int, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
	return s.connectTCP(host, port, DialTimeout, onFrame, onClosed)
}

func (s *Session) ConnectDirect(host string, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, int, error) {
	return s.connectDirectRange(host, PlayerPortMin, PlayerPortMax, onFrame, onClosed)
}

func (s *Session) connectDirectRange(host string, first, last int, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, int, error) {
	var handshakeErr error
	for port := first; port <= last; port++ {
		handshake, err := s.connectTCP(host, port, 200*time.Millisecond, onFrame, onClosed)
		if err == nil {
			return handshake, port, nil
		}
		var dialErr *net.OpError
		if !errors.As(err, &dialErr) || dialErr.Op != "dial" {
			handshakeErr = err
		}
	}
	if handshakeErr != nil {
		return protocol.Handshake{}, 0, fmt.Errorf("%s 上的 Player 握手失败: %w", host, handshakeErr)
	}
	return protocol.Handshake{}, 0, fmt.Errorf("在 %s 的 TCP 端口 %d–%d 未找到 Unity Player，请确认设备与电脑网络互通且游戏是 Development Build", host, first, last)
}

func (s *Session) connectTCP(host string, port int, timeout time.Duration, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
	s.mu.Lock()
	if s.conn != nil {
		s.mu.Unlock()
		return protocol.Handshake{}, errors.New("unityprofiler: already connected")
	}
	s.mu.Unlock()

	if port <= 0 {
		port = DefaultPort
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return protocol.Handshake{}, fmt.Errorf("unityprofiler: dial %s: %w", addr, err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return s.connectTransport(conn, false, onFrame, onClosed)
}

func (s *Session) ConnectIncoming(conn net.Conn, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
	return s.connectTransport(conn, true, onFrame, onClosed)
}

func (s *Session) connectTransport(conn net.Conn, incoming bool, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {

	mask, categories := s.AreaMask, s.Categories
	if mask == 0 {
		mask = EditorAreaMask
	}
	if len(categories) == 0 {
		categories = EditorCategories
	}
	startProfiler := func() error { return WriteProfilerStart(conn, mask, categories) }
	_ = conn.SetWriteDeadline(time.Now().Add(HandshakeTimeout))
	if !incoming {
		if err := startProfiler(); err != nil {
			conn.Close()
			return protocol.Handshake{}, fmt.Errorf("unityprofiler: 下发 Profiler 开启报文失败: %w", err)
		}
	}
	br := bufio.NewReaderSize(conn, 1<<20)
	_ = conn.SetReadDeadline(time.Now().Add(HandshakeTimeout))
	s.mu.Lock()
	onLog := s.onLog
	s.mu.Unlock()
	intro, err := readPlayerIntro(br, incoming, startProfiler, onLog)
	_ = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		conn.Close()
		if incoming && (isTimeout(err) || errors.Is(err, context.DeadlineExceeded)) {
			return protocol.Handshake{}, fmt.Errorf("unityprofiler: 小游戏已连接，但未在 %s 内开始发送 Profiler 数据", HandshakeTimeout)
		}
		if !incoming && isTimeout(err) {
			return protocol.Handshake{}, fmt.Errorf("%w（%s 内无响应）：Player 同时只服务一个 Profiler 客户端，"+
				"请确认 Unity Editor 的 Profiler 已断开、且本工具没有残留连接；仍不行就重启游戏进程", ErrPlayerSilent, HandshakeTimeout)
		}
		return protocol.Handshake{}, err
	}
	if !incoming && !intro.Info.SupportsProfile() {
		conn.Close()
		return protocol.Handshake{}, &PlayerRejectError{
			Info:   intro.Info,
			Reason: fmt.Sprintf("宣告 Flags=%d 未置 SupportsProfile，需 Development Build", intro.Info.Flags),
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	hs := handshakeFromIntro(intro)
	if incoming {
		if hs.DeviceName == "" {
			hs.DeviceName = "Autoconnected Player"
		}
		hs.OS = "微信小游戏"
		hs.ProfilerStreamVersion = intro.Stream.FormatVersion
		hs.MainThreadID = intro.Stream.MainThreadID
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	if s.conn != nil {
		s.mu.Unlock()
		cancel()
		conn.Close()
		return protocol.Handshake{}, errors.New("unityprofiler: already connected")
	}
	s.memory = nil
	s.conn = conn
	s.handshake = hs
	s.decoder = NewDecoder(nil)
	s.cancel = cancel
	s.done = done
	s.deep = false
	s.callStacks = false
	s.mu.Unlock()

	go s.readLoop(ctx, conn, br, intro.Pending, incoming, onFrame, onLog, onClosed, done)
	return hs, nil
}

// WriteProfilerStart 下发 Editor 打开 Profiler 时的报文序列。
func WriteProfilerStart(w interface{ Write([]byte) (int, error) }, mask uint32, categories []string) error {
	if err := WriteMessage(w, MsgIDProfilerSetup, EncodeCallstackFlags(0)); err != nil {
		return err
	}
	if err := WriteMessage(w, MsgIDProfilerAreas, EncodeAreaMask(mask)); err != nil {
		return err
	}
	if err := WriteMessage(w, MsgIDProfilerCategories, EncodeCategories(categories)); err != nil {
		return err
	}
	return WriteMessage(w, MsgIDAnnounce, append([]byte(selfAnnouncement()), 0))
}

// WriteProfilerStop 下发停止采集报文（Editor 断开 Profiler 时的行为）。
func WriteProfilerStop(w interface{ Write([]byte) (int, error) }) error {
	if err := WriteMessage(w, MsgIDProfilerAreas, EncodeAreaMask(0)); err != nil {
		return err
	}
	return WriteMessage(w, MsgIDProfilerCategories, nil)
}

func selfAnnouncement() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unityperf"
	}
	return FormatAnnouncement(PlayerInfo{
		IP:          "127.0.0.1",
		Port:        DefaultPort,
		Flags:       FlagSupportsProfile,
		GUID:        "1",
		ID:          fmt.Sprintf("%s(%s)", EditorIdentity, host),
		Debug:       true,
		PackageName: "unityperf",
		ProjectName: "unityperf",
	})
}

// playerIntro 是 Player 连接后推送的引擎信息与宣告。
type playerIntro struct {
	Engine PlayerEngineInfo
	Info   PlayerInfo
	Stream StreamHeader
	// Pending 是握手期间先到的数据流报文体，必须原样交给解码器。
	// 连接时若 Player 已在采集，数据流头会排在宣告之前，丢掉它就再也拿不到主线程 ID。
	Pending [][]byte
}

// readPlayerIntro 读取 Player 的引擎信息与宣告报文，其间到达的数据流报文会被留存。
func readPlayerIntro(r *bufio.Reader, incoming bool, startProfiler func() error, onLog func(string, string)) (playerIntro, error) {
	var intro playerIntro
	haveEngine, haveInfo := false, false
	for index := 0; incoming || (index < 64 && !(haveEngine && haveInfo)); index++ {
		msg, err := ReadMessage(r)
		if err != nil {
			return playerIntro{}, fmt.Errorf("unityprofiler: 读取 Player 宣告失败: %w", err)
		}
		switch msg.ID {
		case MsgIDProfilerData:
			if incoming {
				if header, err := ParseStreamHeader(msg.Body); err == nil {
					intro.Stream = header
					intro.Engine.Version = header.EngineVersion
					intro.Pending = append(intro.Pending, msg.Body)
					return intro, nil
				}
			} else {
				intro.Pending = append(intro.Pending, msg.Body)
			}
		case MsgIDPlayerInfo:
			engine, err := ParsePlayerEngineInfo(msg.Body)
			if err != nil {
				return playerIntro{}, err
			}
			intro.Engine = engine
			if incoming && !haveEngine {
				if err := startProfiler(); err != nil {
					return playerIntro{}, fmt.Errorf("unityprofiler: 下发小游戏 Profiler 开启报文失败: %w", err)
				}
			}
			haveEngine = true
		case MsgIDAnnounce:
			info, err := ParseAnnouncement(AnnouncementFromBody(msg.Body))
			if err != nil {
				if incoming {
					continue
				}
				return playerIntro{}, err
			}
			intro.Info = info
			haveInfo = true
		default:
			dispatchPlayerLog(msg, onLog)
		}
	}
	if !haveInfo {
		return playerIntro{}, errors.New("unityprofiler: 未收到 Player 宣告报文")
	}
	return intro, nil
}

func handshakeFromIntro(intro playerIntro) protocol.Handshake {
	hs := protocol.Handshake{
		DeviceName:                 intro.Info.ID,
		AppVersion:                 intro.Info.ProjectName,
		UnityVersion:               intro.Engine.Version,
		DeepProfilerSupported:      intro.Engine.DeepProfilerSupported,
		ManagedCallstacksSupported: intro.Engine.ManagedCallstacksSupported,
	}
	if model := deviceModelFromID(intro.Info.ID); model != "" {
		hs.DeviceModel = model
	}
	if strings.Contains(strings.ToLower(intro.Info.ID), "android") || intro.Engine.Platform == platformAndroid {
		hs.OS = "Android"
	}
	return hs
}

// platformAndroid 是 RuntimePlatform.Android 的枚举值。
const platformAndroid uint32 = 11

// deviceModelFromID 从 `AndroidPlayer(11,Xiaomi_Mi_MIX_2S@10.21.42.159)` 中取出机型。
func deviceModelFromID(id string) string {
	open := strings.IndexByte(id, '(')
	close := strings.LastIndexByte(id, ')')
	if open < 0 || close <= open {
		return ""
	}
	inner := id[open+1 : close]
	if i := strings.IndexByte(inner, ','); i >= 0 {
		inner = inner[i+1:]
	}
	if i := strings.IndexByte(inner, '@'); i >= 0 {
		inner = inner[:i]
	}
	return strings.ReplaceAll(strings.TrimSpace(inner), "_", " ")
}

func (s *Session) readLoop(ctx context.Context, conn net.Conn, br *bufio.Reader, pending [][]byte, incoming bool, onFrame FrameHandler, onLog func(string, string), onClosed func(error), done chan struct{}) {
	defer close(done)

	// 解码出的帧先攒着，等放开会话锁之后再回调。
	//
	// 绝不能在持锁时回调：调用方（服务层）会在自己的锁里查会话状态（Connected 等），
	// 反向就是"会话锁 → 调用方锁"，两条路径撞上就整个卡死——界面连日志都不再动。
	var snaps []FrameSnapshot
	dec := NewDecoder(func(snap FrameSnapshot) { snaps = append(snaps, snap) })
	s.mu.Lock()
	s.decoder = dec
	s.mu.Unlock()
	flush := func() {
		for _, snap := range snaps {
			if onFrame != nil {
				onFrame(ToMemoryFrame(snap))
			}
		}
		snaps = snaps[:0]
	}

	var loopErr error
	checkedVersion := false
	emitRaw := func(body []byte) {
		s.mu.Lock()
		onRaw := s.onRaw
		s.mu.Unlock()
		if onRaw != nil {
			onRaw(body)
		}
	}

	// 先补上握手期间到达的数据流报文，其中通常包含数据流头。
	for _, body := range pending {
		emitRaw(body)
		s.mu.Lock()
		_ = dec.Feed(body)
		loopErr = s.checkStreamVersion(dec, &checkedVersion)
		if loopErr == nil {
			loopErr = checkDesync(dec)
		}
		s.mu.Unlock()
		flush()
		if loopErr != nil {
			break
		}
	}
	for loopErr == nil {
		// 每条报文都续一次读等待。超时通常发生在「下一条报文的第一个字节」
		// （App 进后台、Player 暂停上报），bufio 里没有半包，可以继续等。
		if !incoming {
			_ = conn.SetReadDeadline(time.Now().Add(readIdleTimeout))
		}
		msg, err := ReadMessage(br)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if isTimeout(err) {
				continue
			}
			loopErr = err
			break
		}
		if msg.ID != MsgIDProfilerData {
			s.receiveMemory(conn, msg)
			dispatchPlayerLog(msg, onLog)
			continue
		}
		emitRaw(msg.Body)
		s.mu.Lock()
		// 单块出错不终止采集：Feed 内部已标记失步，后续块继续尝试。
		_ = dec.Feed(msg.Body)
		loopErr = s.checkStreamVersion(dec, &checkedVersion)
		if loopErr == nil {
			loopErr = checkDesync(dec)
		}
		if loopErr == nil && !checkedVersion && dec.Stats().Chunks > maxChunksWithoutHeader {
			// 没有流头就拿不到主线程 ID，所有分块都会被丢弃。此时宁可报错，
			// 也不要让界面一直停在 0 帧却显示"已连接"。
			loopErr = fmt.Errorf("unityprofiler: 已收到 %d 个数据分块但始终没有数据流头，无法解析（请重新连接）",
				dec.Stats().Chunks)
		}
		s.mu.Unlock()
		flush()
	}

	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.mu.Unlock()
	conn.Close()
	if onClosed != nil {
		onClosed(loopErr)
	}
}

// checkDesync 把主线程流/元数据流失步当成致命错误。
//
// 这两条流一旦失步就再也解不出帧（缓冲区已丢弃，字节流无法重新对齐），
// 继续挂着只会让界面显示"已连接"却永远停在同一帧，还白占着 Player 唯一的
// Profiler 槽位；主动断开并报错，用户才看得见到底出了什么事。
func checkDesync(dec *Decoder) error {
	st := dec.Stats()
	if !st.Desynced {
		return nil
	}
	return fmt.Errorf("unityprofiler: Profiler 数据流解析失步，已停止采集：%s", st.LastError)
}

// checkStreamVersion 在数据流头到达后回填流版本并判定兼容性；调用方须持有 s.mu。
func (s *Session) checkStreamVersion(dec *Decoder, checked *bool) error {
	h, ok := dec.Header()
	if !ok || *checked {
		return nil
	}
	*checked = true
	s.handshake.ProfilerStreamVersion = h.FormatVersion
	s.handshake.MainThreadID = h.MainThreadID
	if h.EngineVersion != "" {
		s.handshake.UnityVersion = h.EngineVersion
	}
	if !IsSupportedStreamVersion(h.FormatVersion) {
		return fmt.Errorf("unityprofiler: Profiler 数据流格式 %#x 不受支持（仅支持 Unity %s，格式 %#x）",
			h.FormatVersion, protocol.SupportedUnityMajor, Unity2022StreamVersion)
	}
	return nil
}

// Handshake 返回当前 Player 信息。
func (s *Session) Handshake() (protocol.Handshake, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return protocol.Handshake{}, ErrNotConnected
	}
	return s.handshake, nil
}

// PatchHandshake 补全 USB 场景下从 logcat / getprop 得到的字段。
func (s *Session) PatchHandshake(fn func(*protocol.Handshake)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.handshake)
}

// DecodeStats 返回数据流解码指标，供自检与排障。
func (s *Session) DecodeStats() DecodeStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decoder == nil {
		return DecodeStats{}
	}
	return s.decoder.Stats()
}

// CounterNames 返回 Player 上报的计数器名单。
func (s *Session) CounterNames() []string {
	s.mu.Lock()
	dec := s.decoder
	s.mu.Unlock()
	if dec == nil {
		return nil
	}
	return dec.CounterNames()
}

// Connected 返回是否仍保持 Profiler 连接。
func (s *Session) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn != nil
}

// SetProfilerOptions 向 Player 下发 Deep Profile / Call Stacks 开关。
// 只影响之后的帧；已采集数据不变。未连接时返回 ErrNotConnected。
//
// deep 目前只记录状态、不下发：MsgIDProfilerSetup 整个 u32 逐位实测过，里面只有
// 分配调用栈的开关，没有一位能让采样树变深，Deep Profile 应是另一条尚未逆向出的
// 报文。此前把它塞进 bit0，反而悄悄改掉了调用栈模式。
func (s *Session) SetProfilerOptions(deep, callStacks bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return ErrNotConnected
	}
	s.deep = deep
	s.callStacks = callStacks
	var flags uint32
	if callStacks {
		flags = CallstackFlagsDefault
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	err := WriteMessage(s.conn, MsgIDProfilerSetup, EncodeCallstackFlags(flags))
	_ = s.conn.SetWriteDeadline(time.Time{})
	return err
}

// ProfilerOptions 返回当前已下发的 Deep Profile / Call Stacks 状态。
func (s *Session) ProfilerOptions() (deep, callStacks bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deep, s.callStacks
}

// Close 停止采集、断开连接并等待读取循环退出。
func (s *Session) Close() {
	s.mu.Lock()
	conn := s.conn
	cancel := s.cancel
	done := s.done
	s.conn = nil
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		_ = WriteProfilerStop(conn)
		conn.Close()
	}
	if done != nil {
		<-done
	}
}
