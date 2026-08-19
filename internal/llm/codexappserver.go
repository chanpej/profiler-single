package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unityperfagent/internal/procutil"
)

// codexServer is the persistent Codex backend used by one Agent Chat.
// Start creates one app-server process and one thread; Prompt adds turns to it.
type codexServer interface {
	Start(ctx context.Context, dir string, profile AgentProfile) error
	Prompt(ctx context.Context, text string, profile AgentProfile, emit func(string)) error
	Close()
}

type codexServerFactory func() codexServer

func newCodexAppServerFactory() codexServerFactory {
	return newCodexAppServer
}

func newCodexAppServer() codexServer {
	return newCodexAppServerWithStarter(startCodexAppProcess)
}

type codexAppProcess interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() string
	Done() <-chan error
	Close()
}

type codexAppProcessStarter func(command string, args []string, dir string) (codexAppProcess, error)

type codexAppServer struct {
	startProcess codexAppProcessStarter

	opMu    sync.Mutex
	writeMu sync.Mutex
	stateMu sync.Mutex

	process  codexAppProcess
	messages <-chan codexRPCRead
	readStop chan struct{}
	threadID string
	model    string
	effort   string
	closed   bool
	nextID   atomic.Int64
}

func newCodexAppServerWithStarter(starter codexAppProcessStarter) *codexAppServer {
	return &codexAppServer{startProcess: starter}
}

func (s *codexAppServer) Start(ctx context.Context, dir string, profile AgentProfile) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return fmt.Errorf("llm: codex app-server is closed")
	}
	if s.process != nil {
		s.stateMu.Unlock()
		return fmt.Errorf("llm: codex app-server already started")
	}
	s.stateMu.Unlock()

	if s.startProcess == nil {
		return fmt.Errorf("llm: codex app-server process starter is nil")
	}
	dir = codexAppServerDir(dir, profile.WorkDir)
	args := append([]string{"app-server", "--stdio"}, codexMCPConfigArgs(profile.mcpServers)...)
	process, err := s.startProcess(profile.Command, args, dir)
	if err != nil {
		return fmt.Errorf("llm: start codex app-server: %w", err)
	}
	stop := make(chan struct{})
	messages := readCodexRPC(process.Stdout(), stop)

	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		close(stop)
		process.Close()
		return fmt.Errorf("llm: codex app-server is closed")
	}
	s.process = process
	s.messages = messages
	s.readStop = stop
	s.stateMu.Unlock()

	started := false
	defer func() {
		if !started {
			s.Close()
		}
	}()

	_, _, err = s.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "UnityPerfAgent",
			"title":   "UnityPerfAgent",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return s.withStderr(fmt.Errorf("llm: codex app-server initialize: %w", err))
	}
	if err := s.notify("initialized", nil); err != nil {
		return s.withStderr(fmt.Errorf("llm: codex app-server initialized: %w", err))
	}

	threadParams := map[string]any{
		"approvalPolicy":   "never",
		"baseInstructions": chatSystemHint,
		"cwd":              dir,
		"ephemeral":        true,
		"sandbox":          "danger-full-access",
	}
	model := strings.TrimSpace(profile.Model)
	if model != "" {
		threadParams["model"] = model
	}
	effort := ""
	if strings.TrimSpace(profile.Effort) != "" {
		effort = NormalizeEffort(profile.Effort)
		threadParams["config"] = map[string]any{"model_reasoning_effort": effort}
	}
	result, _, err := s.call(ctx, "thread/start", threadParams)
	if err != nil {
		return s.withStderr(fmt.Errorf("llm: codex app-server thread/start: %w", err))
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &thread); err != nil {
		return s.withStderr(fmt.Errorf("llm: decode codex thread/start: %w", err))
	}
	if strings.TrimSpace(thread.Thread.ID) == "" {
		return s.withStderr(fmt.Errorf("llm: codex thread/start returned no thread id"))
	}

	s.stateMu.Lock()
	s.threadID = thread.Thread.ID
	s.model = model
	s.effort = effort
	s.stateMu.Unlock()
	started = true
	return nil
}

func codexAppServerDir(dir, profileDir string) string {
	if strings.TrimSpace(dir) != "" {
		return dir
	}
	if strings.TrimSpace(profileDir) != "" {
		return profileDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return dir
}

func (s *codexAppServer) Prompt(ctx context.Context, text string, profile AgentProfile, emit func(string)) error {
	if text == "" {
		return fmt.Errorf("llm: empty prompt")
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.stateMu.Lock()
	threadID := s.threadID
	currentModel := s.model
	currentEffort := s.effort
	process := s.process
	s.stateMu.Unlock()
	if process == nil || threadID == "" {
		return fmt.Errorf("llm: codex app-server not started")
	}

	params := map[string]any{
		"threadId": threadID,
		"input": []any{map[string]any{
			"type": "text",
			"text": text,
		}},
	}
	model := strings.TrimSpace(profile.Model)
	modelChanged := model != "" && model != currentModel
	if modelChanged {
		params["model"] = model
	}
	effort := ""
	if strings.TrimSpace(profile.Effort) != "" {
		effort = NormalizeEffort(profile.Effort)
	}
	effortChanged := effort != "" && effort != currentEffort
	if effortChanged {
		params["effort"] = effort
	}

	result, early, err := s.call(ctx, "turn/start", params)
	if err != nil {
		return s.withStderr(fmt.Errorf("llm: codex app-server turn/start: %w", err))
	}
	var turnResult struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &turnResult); err != nil {
		return s.withStderr(fmt.Errorf("llm: decode codex turn/start: %w", err))
	}
	turnID := strings.TrimSpace(turnResult.Turn.ID)
	if turnID == "" {
		return s.withStderr(fmt.Errorf("llm: codex turn/start returned no turn id"))
	}
	if modelChanged || effortChanged {
		s.stateMu.Lock()
		if modelChanged {
			s.model = model
		}
		if effortChanged {
			s.effort = effort
		}
		s.stateMu.Unlock()
	}

	for _, message := range early {
		done, eventErr := handleCodexTurnEvent(message, threadID, turnID, emit)
		if done {
			return s.withStderr(eventErr)
		}
	}

	for {
		select {
		case <-ctx.Done():
			s.interruptAndDrain(threadID, turnID)
			return ctx.Err()
		case read, ok := <-s.messages:
			if !ok {
				return s.withStderr(s.transportError("stdout closed"))
			}
			if read.err != nil {
				return s.withStderr(s.transportError(read.err.Error()))
			}
			done, eventErr := handleCodexTurnEvent(read.message, threadID, turnID, emit)
			if done {
				return s.withStderr(eventErr)
			}
		}
	}
}

func handleCodexTurnEvent(message codexRPCMessage, threadID, turnID string, emit func(string)) (bool, error) {
	switch message.Method {
	case "item/agentMessage/delta":
		var params struct {
			Delta    string `json:"delta"`
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.ThreadID == threadID && params.TurnID == turnID && params.Delta != "" && emit != nil {
			emit(params.Delta)
		}
	case "item/started", "item/completed":
		var params struct {
			ThreadID string         `json:"threadId"`
			TurnID   string         `json:"turnId"`
			Item     map[string]any `json:"item"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.ThreadID == threadID && params.TurnID == turnID && strings.EqualFold(stringField(params.Item, "type"), "mcpToolCall") {
			if progress := codexMCPProgress(message.Method, params.Item); progress != "" && emit != nil {
				emit("\n\n" + progress)
			}
		}
	case "turn/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		if json.Unmarshal(message.Params, &params) != nil || params.ThreadID != threadID || params.Turn.ID != turnID {
			return false, nil
		}
		switch params.Turn.Status {
		case "completed":
			return true, nil
		case "failed":
			message := "turn failed"
			if params.Turn.Error != nil && strings.TrimSpace(params.Turn.Error.Message) != "" {
				message = params.Turn.Error.Message
			}
			return true, fmt.Errorf("llm: codex turn failed: %s", message)
		case "interrupted":
			return true, fmt.Errorf("llm: codex turn interrupted")
		default:
			return true, fmt.Errorf("llm: codex turn completed with status %q", params.Turn.Status)
		}
	}
	return false, nil
}

func (s *codexAppServer) interruptAndDrain(threadID, turnID string) {
	_, _ = s.request("turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID})
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return
		case read, ok := <-s.messages:
			if !ok || read.err != nil {
				return
			}
			done, _ := handleCodexTurnEvent(read.message, threadID, turnID, nil)
			if done {
				return
			}
		}
	}
}

func (s *codexAppServer) call(ctx context.Context, method string, params any) (json.RawMessage, []codexRPCMessage, error) {
	id, err := s.request(method, params)
	if err != nil {
		return nil, nil, err
	}
	var notifications []codexRPCMessage
	for {
		select {
		case <-ctx.Done():
			return nil, notifications, ctx.Err()
		case read, ok := <-s.messages:
			if !ok {
				return nil, notifications, s.transportError("stdout closed")
			}
			if read.err != nil {
				return nil, notifications, s.transportError(read.err.Error())
			}
			message := read.message
			if len(message.ID) == 0 {
				notifications = append(notifications, message)
				continue
			}
			if !sameCodexRPCID(message.ID, id) {
				continue
			}
			if message.Error != nil {
				return nil, notifications, message.Error
			}
			return message.Result, notifications, nil
		}
	}
}

func (s *codexAppServer) request(method string, params any) (int64, error) {
	id := s.nextID.Add(1)
	return id, s.write(map[string]any{"id": id, "method": method, "params": params})
}

func (s *codexAppServer) notify(method string, params any) error {
	message := map[string]any{"method": method}
	if params != nil {
		message["params"] = params
	}
	return s.write(message)
}

func (s *codexAppServer) write(message any) error {
	line, err := json.Marshal(message)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.stateMu.Lock()
	process := s.process
	s.stateMu.Unlock()
	if process == nil {
		return fmt.Errorf("codex app-server is not running")
	}
	if _, err := process.Stdin().Write(line); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	return nil
}

func (s *codexAppServer) transportError(message string) error {
	s.stateMu.Lock()
	process := s.process
	s.stateMu.Unlock()
	if process != nil {
		select {
		case err, ok := <-process.Done():
			if ok && err != nil {
				return fmt.Errorf("%s: %w", message, err)
			}
		default:
		}
	}
	return errors.New(message)
}

func (s *codexAppServer) withStderr(err error) error {
	if err == nil {
		return nil
	}
	s.stateMu.Lock()
	process := s.process
	s.stateMu.Unlock()
	if process == nil {
		return err
	}
	stderr := strings.TrimSpace(process.Stderr())
	if stderr == "" {
		return err
	}
	const maxStderr = 1200
	if len(stderr) > maxStderr {
		stderr = stderr[len(stderr)-maxStderr:]
	}
	return fmt.Errorf("%w\ncodex stderr: %s", err, stderr)
}

func (s *codexAppServer) Close() {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return
	}
	s.closed = true
	process := s.process
	stop := s.readStop
	s.process = nil
	s.messages = nil
	s.threadID = ""
	s.stateMu.Unlock()

	if stop != nil {
		close(stop)
	}
	if process != nil {
		process.Close()
	}
}

type codexRPCMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *codexRPCError  `json:"error"`
}

type codexRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *codexRPCError) Error() string {
	if len(bytes.TrimSpace(e.Data)) > 0 && !bytes.Equal(bytes.TrimSpace(e.Data), []byte("null")) {
		return fmt.Sprintf("rpc error %d: %s (%s)", e.Code, e.Message, bytes.TrimSpace(e.Data))
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type codexRPCRead struct {
	message codexRPCMessage
	err     error
}

func readCodexRPC(reader io.Reader, stop <-chan struct{}) <-chan codexRPCRead {
	out := make(chan codexRPCRead, 128)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var message codexRPCMessage
			if err := json.Unmarshal(line, &message); err != nil {
				select {
				case out <- codexRPCRead{err: fmt.Errorf("decode response %q: %w", truncateCodexLine(line), err)}:
				case <-stop:
				}
				return
			}
			select {
			case out <- codexRPCRead{message: message}:
			case <-stop:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case out <- codexRPCRead{err: fmt.Errorf("read response: %w", err)}:
			case <-stop:
			}
		}
	}()
	return out
}

func truncateCodexLine(line []byte) string {
	const limit = 240
	if len(line) <= limit {
		return string(line)
	}
	return string(line[:limit]) + "..."
}

func sameCodexRPCID(raw json.RawMessage, id int64) bool {
	var got int64
	return json.Unmarshal(raw, &got) == nil && got == id
}

type execCodexAppProcess struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    appServerBuffer
	cancel    context.CancelFunc
	done      chan error
	closeOnce sync.Once
}

func startCodexAppProcess(command string, args []string, dir string) (codexAppProcess, error) {
	exe, argv, err := ResolveLaunch(command, args)
	if err != nil {
		return nil, err
	}
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := procutil.CommandContext(procCtx, exe, argv...)
	if dir != "" {
		cmd.Dir = dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	process := &execCodexAppProcess{
		cmd: cmd, stdin: stdin, stdout: stdout, cancel: cancel, done: make(chan error, 1),
	}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start %q: %w", exe, err)
	}
	go func() {
		process.done <- cmd.Wait()
		close(process.done)
	}()
	return process, nil
}

func (p *execCodexAppProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *execCodexAppProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *execCodexAppProcess) Stderr() string        { return p.stderr.String() }
func (p *execCodexAppProcess) Done() <-chan error    { return p.done }
func (p *execCodexAppProcess) Close() {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()
		p.cancel()
		_ = p.stdout.Close()
	})
}

type appServerBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *appServerBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *appServerBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

var _ codexServer = (*codexAppServer)(nil)
