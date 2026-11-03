package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProtocolOptions configures an out-of-process, stdio protocol plugin.
type ProtocolOptions struct {
	Command string
	Args    []string
	Env     map[string]string
	Dir     string
	Timeout time.Duration
}

type protocolProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	encoder *json.Encoder
	host    *Host
	timeout time.Duration

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan ProtocolMessage
	nextID  atomic.Uint64
	initCh  chan ProtocolMessage
	done    chan struct{}
	once    sync.Once
	err     error
	stderr  *limitedOutput
}

type limitedOutput struct {
	mu  sync.Mutex
	buf []byte
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	const max = 8 << 10
	if len(p) >= max {
		w.buf = append(w.buf[:0], p[len(p)-max:]...)
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) > max {
		w.buf = append(w.buf[:0], w.buf[len(w.buf)-max:]...)
	}
	return len(p), nil
}

func (w *limitedOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

func startProtocolProcess(h *Host, opt ProtocolOptions) (*protocolProcess, error) {
	if strings.TrimSpace(opt.Command) == "" {
		return nil, fmt.Errorf("protocol plugin has no command")
	}
	cmd := exec.Command(opt.Command, opt.Args...)
	cmd.Dir = opt.Dir
	if len(opt.Env) > 0 {
		env := append([]string(nil), os.Environ()...)
		seen := map[string]bool{}
		for _, item := range env {
			if i := strings.IndexByte(item, '='); i >= 0 {
				seen[item[:i]] = true
			}
		}
		for key, value := range opt.Env {
			if seen[key] {
				continue
			}
			env = append(env, key+"="+value)
		}
		// Explicit plugin env overrides inherited values.
		for key, value := range opt.Env {
			entry := key + "=" + value
			replaced := false
			for i, existing := range env {
				if strings.HasPrefix(existing, key+"=") {
					env[i] = entry
					replaced = true
					break
				}
			}
			if !replaced {
				env = append(env, entry)
			}
		}
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	output := &limitedOutput{}
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}
	p := &protocolProcess{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		encoder: json.NewEncoder(stdin),
		host:    h,
		timeout: opt.Timeout,
		pending: map[uint64]chan ProtocolMessage{},
		initCh:  make(chan ProtocolMessage, 1),
		done:    make(chan struct{}),
		stderr:  output,
	}
	go p.readLoop()
	return p, nil
}

func (p *protocolProcess) init(info HostInfo) ([]byte, error) {
	msg := ProtocolMessage{Type: "init", HostInfo: &info}
	if err := p.write(msg); err != nil {
		p.fail(err)
		return nil, err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case reply := <-p.initCh:
		if reply.Error != "" {
			return nil, fmt.Errorf("protocol plugin init: %s", reply.Error)
		}
		if reply.Registration == nil {
			return nil, fmt.Errorf("protocol plugin init returned no registration")
		}
		raw, err := json.Marshal(reply.Registration)
		if err != nil {
			return nil, err
		}
		return raw, nil
	case <-p.done:
		return nil, p.processError("protocol plugin exited before registration")
	case <-timer.C:
		return nil, fmt.Errorf("protocol plugin timed out waiting for registration")
	}
}

func (p *protocolProcess) call(ctx context.Context, invoke Invoke) ([]byte, error) {
	id := p.nextID.Add(1)
	ch := make(chan ProtocolMessage, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	if err := p.write(ProtocolMessage{Type: "invoke", ID: id, Invoke: &invoke}); err != nil {
		p.removePending(id)
		return nil, err
	}
	wait := p.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
	}
	if wait <= 0 {
		wait = p.timeout
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case reply := <-ch:
		if reply.Error != "" {
			return nil, fmt.Errorf("protocol plugin: %s", reply.Error)
		}
		if reply.Result == nil {
			return []byte(`{"result":null}`), nil
		}
		raw, err := json.Marshal(reply.Result)
		if err != nil {
			return nil, err
		}
		return raw, nil
	case <-ctx.Done():
		p.removePending(id)
		return nil, ctx.Err()
	case <-p.done:
		return nil, p.processError("protocol plugin exited")
	case <-timer.C:
		p.removePending(id)
		return nil, fmt.Errorf("protocol plugin call timed out")
	}
}

func (p *protocolProcess) removePending(id uint64) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *protocolProcess) readLoop() {
	decoder := json.NewDecoder(bufio.NewReader(p.stdout))
	for {
		var msg ProtocolMessage
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				p.fail(err)
			} else {
				p.fail(io.EOF)
			}
			return
		}
		switch msg.Type {
		case "registration":
			select {
			case p.initCh <- msg:
			default:
			}
		case "result":
			p.mu.Lock()
			ch := p.pending[msg.ID]
			delete(p.pending, msg.ID)
			p.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
		case "host_call":
			go p.handleHostCall(msg)
		default:
			// Unknown frame types are ignored so the protocol can grow.
		}
	}
}

func (p *protocolProcess) handleHostCall(msg ProtocolMessage) {
	if msg.Call == nil {
		return
	}
	raw := p.host.answerFrom(msg.Call)
	var reply HostReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		reply = HostReply{ID: msg.Call.ID, Error: &APIError{Message: err.Error()}}
	}
	reply.ID = msg.Call.ID
	_ = p.write(ProtocolMessage{Type: "host_reply", ID: msg.ID, Reply: &reply})
}

func (p *protocolProcess) write(msg ProtocolMessage) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if err := p.encoder.Encode(msg); err != nil {
		return err
	}
	return nil
}

func (p *protocolProcess) fail(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	})
}

func (p *protocolProcess) processError(reason string) error {
	p.mu.Lock()
	err := p.err
	p.mu.Unlock()
	stderr := strings.TrimSpace(p.stderr.String())
	if stderr != "" {
		return fmt.Errorf("%s: %v (%s)", reason, err, stderr)
	}
	return fmt.Errorf("%s: %v", reason, err)
}

func (p *protocolProcess) close() error {
	_ = p.write(ProtocolMessage{Type: "shutdown"})
	_ = p.stdin.Close()
	wait := make(chan error, 1)
	go func() { wait <- p.cmd.Wait() }()
	select {
	case err := <-wait:
		p.fail(err)
		return err
	case <-time.After(2 * time.Second):
		_ = p.cmd.Process.Kill()
		<-wait
		p.fail(fmt.Errorf("protocol plugin did not exit"))
		return nil
	}
}

// LoadProtocol loads an out-of-process plugin declared by a plugin.json
// manifest. The manifest must set protocol:"stdio" (or just command) and
// command.
func (h *Host) LoadProtocol(manifestPath string) (*Instance, error) {
	abs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, err
	}
	manifest, err := readManifest(abs)
	if err != nil {
		return nil, err
	}
	if manifest.Protocol != "" && !strings.EqualFold(manifest.Protocol, "stdio") {
		return nil, fmt.Errorf("%s: unsupported plugin protocol %q", abs, manifest.Protocol)
	}
	if strings.TrimSpace(manifest.Command) == "" {
		return nil, fmt.Errorf("%s: protocol plugin has no command", abs)
	}
	opts := ProtocolOptions{
		Command: manifest.Command,
		Args:    append([]string(nil), manifest.Args...),
		Env:     manifest.Env,
		Dir:     filepath.Dir(abs),
	}
	if !filepath.IsAbs(opts.Command) && (strings.ContainsAny(opts.Command, `/\`) || strings.HasPrefix(opts.Command, ".")) {
		opts.Command = filepath.Join(opts.Dir, opts.Command)
	}
	if manifest.CWD != "" {
		if filepath.IsAbs(manifest.CWD) {
			opts.Dir = manifest.CWD
		} else {
			opts.Dir = filepath.Join(filepath.Dir(abs), manifest.CWD)
		}
	}
	if manifest.Timeout > 0 {
		opts.Timeout = time.Duration(manifest.Timeout) * time.Second
	}
	return h.loadProtocol(abs, opts, manifest)
}

// LoadProtocolCommand loads a protocol plugin from an explicit command. The
// key is used for de-duplication and diagnostics; it does not have to exist.
func (h *Host) LoadProtocolCommand(key string, opts ProtocolOptions) (*Instance, error) {
	if key == "" {
		key = opts.Command
	}
	abs, err := filepath.Abs(key)
	if err != nil {
		abs = key
	}
	return h.loadProtocol(abs, opts, Manifest{Name: filepath.Base(key)})
}

func (h *Host) loadProtocol(abs string, opts ProtocolOptions, manifest Manifest) (*Instance, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, fmt.Errorf("plugin host is closed")
	}
	if existing := h.byPath[abs]; existing != nil {
		h.mu.Unlock()
		return existing, nil
	}
	info := h.pluginInfoLocked()
	h.mu.Unlock()

	proc, err := startProtocolProcess(h, opts)
	if err != nil {
		return nil, err
	}
	inst := &Instance{Path: abs, host: h, proto: proc}
	raw, err := inst.init(info, 0)
	if err != nil {
		_ = proc.close()
		return nil, err
	}
	reg, err := DecodeRegistration(raw)
	if err != nil {
		_ = proc.close()
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	inst.Registration = reg
	return h.addInstance(abs, inst)
}

func (h *Host) addInstance(key string, inst *Instance) (*Instance, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("plugin host is closed")
	}
	if existing := h.byPath[key]; existing != nil {
		return existing, nil
	}
	h.byPath[key] = inst
	h.order = append(h.order, inst)
	return inst, nil
}

func (h *Host) pluginInfoLocked() HostInfo {
	info := h.info
	if info.APIVersion == 0 {
		info.APIVersion = APIVersion
	}
	if info.Host == "" {
		info.Host = "nulo"
	}
	return info
}

// answerFrom is answer with a typed call, used by protocol frames.
func (h *Host) answerFrom(call *HostCall) []byte {
	raw, _ := json.Marshal(call)
	return h.answer(raw)
}
