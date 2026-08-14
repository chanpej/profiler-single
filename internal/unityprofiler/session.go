package unityprofiler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"unityperfagent/internal/protocol"
)

// DialTimeout / HandshakeTimeout 与 Editor 连接 Player 的等待相当。
const (
	DialTimeout      = 5 * time.Second
	HandshakeTimeout = 5 * time.Second
	// maxChunksWithoutHeader 是允许在数据流头之前到达的分块数上限（约几秒的量）。
	maxChunksWithoutHeader = 2000
	// IdleTimeout 是读取循环允许的最长静默。Player 同时只服务一个 Profiler 客户端，
	// 阻塞在读上不退出会一直占着这个槽位，导致之后谁都连不上（连上也收不到数据），
	// 所以静默超时必须主动断开。采集中每秒有几十帧，正常绝不会触发。
	IdleTimeout = 10 * time.Second
	// EditorIdentity 是本工具在宣告报文里自报的身份，前缀与 Editor 一致，
	// 便于 Player 侧日志区分连接来源。
	EditorIdentity = "UnityEditor.Profiler"
)

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

// FrameHandler 每解析出一帧 Profiler 数据时回调。
type FrameHandler func(protocol.FrameData)

// Session 是连到 Unity Development Player 的 Profiler 客户端。
type Session struct {
	// AreaMask 与 Categories 指定向 Player 申请的采集范围，零值表示沿用 Editor 实测默认。
	// 需在 Connect 之前设置。
	AreaMask   uint32
	Categories []string

	mu        sync.Mutex
	conn      net.Conn
	handshake protocol.Handshake
	decoder   *Decoder
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewSession 创建未连接的会话。
func NewSession() *Session { return &Session{} }

// Connect 以 Editor Profiler 身份连到 Player 并开启采集。
//
// 与 Editor 一致：连上后立即下发 Profiler 开启报文，不等 Player 宣告；随后读取
// Player 的引擎信息与宣告，作为握手结果返回。
func (s *Session) Connect(host string, port int, onFrame FrameHandler, onClosed func(error)) (protocol.Handshake, error) {
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
	conn, err := net.DialTimeout("tcp", addr, DialTimeout)
	if err != nil {
		return protocol.Handshake{}, fmt.Errorf("unityprofiler: dial %s: %w", addr, err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	mask, categories := s.AreaMask, s.Categories
	if mask == 0 {
		mask = EditorAreaMask
	}
	if len(categories) == 0 {
		categories = EditorCategories
	}
	if err := WriteProfilerStart(conn, mask, categories); err != nil {
		conn.Close()
		return protocol.Handshake{}, fmt.Errorf("unityprofiler: 下发 Profiler 开启报文失败: %w", err)
	}

	br := bufio.NewReaderSize(conn, 1<<20)
	_ = conn.SetReadDeadline(time.Now().Add(HandshakeTimeout))
	intro, err := readPlayerIntro(br)
	if err != nil {
		conn.Close()
		if isTimeout(err) {
			return protocol.Handshake{}, fmt.Errorf("%w（%s 内无响应）：Player 同时只服务一个 Profiler 客户端，"+
				"请确认 Unity Editor 的 Profiler 已断开、且本工具没有残留连接；仍不行就重启游戏进程", ErrPlayerSilent, HandshakeTimeout)
		}
		return protocol.Handshake{}, err
	}
	if !intro.Info.SupportsProfile() {
		conn.Close()
		return protocol.Handshake{}, &PlayerRejectError{
			Info:   intro.Info,
			Reason: fmt.Sprintf("宣告 Flags=%d 未置 SupportsProfile，需 Development Build", intro.Info.Flags),
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	hs := handshakeFromIntro(intro)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	s.conn = conn
	s.handshake = hs
	s.decoder = NewDecoder(nil)
	s.cancel = cancel
	s.done = done
	s.mu.Unlock()

	go s.readLoop(ctx, conn, br, intro.Pending, onFrame, onClosed, done)
	return hs, nil
}

// WriteProfilerStart 下发 Editor 打开 Profiler 时的报文序列。
func WriteProfilerStart(w interface{ Write([]byte) (int, error) }, mask uint32, categories []string) error {
	if err := WriteMessage(w, MsgIDProfilerSetup, EncodeAreaMask(0)); err != nil {
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
		host = "UnityPerfAgent"
	}
	return FormatAnnouncement(PlayerInfo{
		IP:          "127.0.0.1",
		Port:        DefaultPort,
		Flags:       FlagSupportsProfile,
		GUID:        "1",
		ID:          fmt.Sprintf("%s(%s)", EditorIdentity, host),
		Debug:       true,
		PackageName: "UnityPerfAgent",
		ProjectName: "UnityPerfAgent",
	})
}

// playerIntro 是 Player 连接后推送的引擎信息与宣告。
type playerIntro struct {
	Engine PlayerEngineInfo
	Info   PlayerInfo
	// Pending 是握手期间先到的数据流报文体，必须原样交给解码器。
	// 连接时若 Player 已在采集，数据流头会排在宣告之前，丢掉它就再也拿不到主线程 ID。
	Pending [][]byte
}

// readPlayerIntro 读取 Player 的引擎信息与宣告报文，其间到达的数据流报文会被留存。
func readPlayerIntro(r *bufio.Reader) (playerIntro, error) {
	var intro playerIntro
	haveEngine, haveInfo := false, false
	for i := 0; i < 64 && !(haveEngine && haveInfo); i++ {
		msg, err := ReadMessage(r)
		if err != nil {
			return playerIntro{}, fmt.Errorf("unityprofiler: 读取 Player 宣告失败: %w", err)
		}
		switch msg.ID {
		case MsgIDProfilerData:
			intro.Pending = append(intro.Pending, msg.Body)
		case MsgIDPlayerInfo:
			engine, err := ParsePlayerEngineInfo(msg.Body)
			if err != nil {
				return playerIntro{}, err
			}
			intro.Engine = engine
			haveEngine = true
		case MsgIDAnnounce:
			info, err := ParseAnnouncement(AnnouncementFromBody(msg.Body))
			if err != nil {
				return playerIntro{}, err
			}
			intro.Info = info
			haveInfo = true
		}
	}
	if !haveInfo {
		return playerIntro{}, errors.New("unityprofiler: 未收到 Player 宣告报文")
	}
	return intro, nil
}

func handshakeFromIntro(intro playerIntro) protocol.Handshake {
	hs := protocol.Handshake{
		DeviceName:   intro.Info.ID,
		AppVersion:   intro.Info.ProjectName,
		UnityVersion: intro.Engine.Version,
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

func (s *Session) readLoop(ctx context.Context, conn net.Conn, br *bufio.Reader, pending [][]byte, onFrame FrameHandler, onClosed func(error), done chan struct{}) {
	defer close(done)

	dec := NewDecoder(func(snap FrameSnapshot) {
		if onFrame != nil {
			onFrame(ToFrameData(snap))
		}
	})
	s.mu.Lock()
	s.decoder = dec
	s.mu.Unlock()

	var loopErr error
	checkedVersion := false
	// 先补上握手期间到达的数据流报文，其中通常包含数据流头。
	for _, body := range pending {
		s.mu.Lock()
		_ = dec.Feed(body)
		loopErr = s.checkStreamVersion(dec, &checkedVersion)
		s.mu.Unlock()
		if loopErr != nil {
			break
		}
	}
	for loopErr == nil {
		// 每条报文都续一次静默超时：宁可断开也不能阻塞在读上占着 Player 的槽位。
		_ = conn.SetReadDeadline(time.Now().Add(IdleTimeout))
		msg, err := ReadMessage(br)
		if err != nil {
			if ctx.Err() == nil {
				loopErr = err
				if isTimeout(err) {
					loopErr = fmt.Errorf("unityprofiler: Player 静默超过 %s，已主动断开（游戏可能已切到后台或停止上报）", IdleTimeout)
				}
			}
			break
		}
		if msg.ID != MsgIDProfilerData {
			continue
		}
		s.mu.Lock()
		// 单块出错不终止采集：Feed 内部已标记失步，后续块继续尝试。
		_ = dec.Feed(msg.Body)
		loopErr = s.checkStreamVersion(dec, &checkedVersion)
		if loopErr == nil && !checkedVersion && dec.Stats().Chunks > maxChunksWithoutHeader {
			// 没有流头就拿不到主线程 ID，所有分块都会被丢弃。此时宁可报错，
			// 也不要让界面一直停在 0 帧却显示"已连接"。
			loopErr = fmt.Errorf("unityprofiler: 已收到 %d 个数据分块但始终没有数据流头，无法解析（请重新连接）",
				dec.Stats().Chunks)
		}
		s.mu.Unlock()
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

// checkStreamVersion 在数据流头到达后回填流版本并判定兼容性；调用方须持有 s.mu。
func (s *Session) checkStreamVersion(dec *Decoder, checked *bool) error {
	h, ok := dec.Header()
	if !ok || *checked {
		return nil
	}
	*checked = true
	s.handshake.ProfilerStreamVersion = h.FormatVersion
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
	dec := s.decoder
	s.mu.Unlock()
	if dec == nil {
		return DecodeStats{}
	}
	return dec.Stats()
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
