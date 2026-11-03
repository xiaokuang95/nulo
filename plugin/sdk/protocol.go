package sdk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/xiaokuang95/nulo/plugin"
)

// ServeProtocol runs a plugin as a standalone process speaking the stdio
// protocol. The plugin executable calls this from main after registering its
// factory with Serve or ServeNamed. Frames are newline-delimited JSON.
func ServeProtocol() error { return ServeProtocolIO(os.Stdin, os.Stdout) }

// ServeProtocolIO is ServeProtocol with explicit streams, useful in tests.
func ServeProtocolIO(in io.Reader, out io.Writer) error {
	decoder := json.NewDecoder(bufio.NewReader(in))
	var init plugin.ProtocolMessage
	if err := decoder.Decode(&init); err != nil {
		return fmt.Errorf("protocol plugin: read init: %w", err)
	}
	if init.Type != "init" || init.HostInfo == nil {
		return fmt.Errorf("protocol plugin: first frame must be init")
	}
	peer := &protocolPeer{
		encoder: json.NewEncoder(out),
		pending: map[uint64]chan plugin.ProtocolMessage{},
		done:    make(chan struct{}),
	}
	hostInfo, err := json.Marshal(init.HostInfo)
	if err != nil {
		return err
	}
	initialized := make(chan struct{})
	var reg plugin.Registration
	go func() {
		reg = current.initialize(string(hostInfo), 0, peer.call)
		close(initialized)
	}()
	go peer.readLoop(decoder)
	<-initialized
	if err := peer.write(plugin.ProtocolMessage{Type: "registration", Registration: &reg}); err != nil {
		peer.fail(err)
		return err
	}
	<-peer.done
	peer.mu.Lock()
	err = peer.err
	peer.mu.Unlock()
	return err
}

type protocolPeer struct {
	writeMu sync.Mutex
	encoder *json.Encoder
	mu      sync.Mutex
	pending map[uint64]chan plugin.ProtocolMessage
	nextID  atomic.Uint64
	done    chan struct{}
	once    sync.Once
	err     error
}

func (p *protocolPeer) readLoop(decoder *json.Decoder) {
	for {
		var msg plugin.ProtocolMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				p.fail(nil)
			} else {
				p.fail(err)
			}
			return
		}
		switch msg.Type {
		case "host_reply":
			p.mu.Lock()
			ch := p.pending[msg.ID]
			delete(p.pending, msg.ID)
			p.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
		case "invoke":
			if msg.Invoke == nil {
				_ = p.write(plugin.ProtocolMessage{Type: "result", ID: msg.ID, Result: &plugin.InvokeResult{Error: "invoke frame has no invoke"}})
				continue
			}
			invoke := *msg.Invoke
			go func(id uint64) {
				result := dispatchInvoke(invoke)
				_ = p.write(plugin.ProtocolMessage{Type: "result", ID: id, Result: &result})
			}(msg.ID)
		case "shutdown":
			p.fail(nil)
			return
		}
	}
}

func (p *protocolPeer) call(request []byte) ([]byte, error) {
	var call plugin.HostCall
	if err := json.Unmarshal(request, &call); err != nil {
		return nil, err
	}
	id := p.nextID.Add(1)
	ch := make(chan plugin.ProtocolMessage, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	if err := p.write(plugin.ProtocolMessage{Type: "host_call", ID: id, Call: &call}); err != nil {
		p.removePending(id)
		return nil, err
	}
	select {
	case msg := <-ch:
		if msg.Error != "" {
			return nil, fmt.Errorf("protocol host: %s", msg.Error)
		}
		if msg.Reply == nil {
			return nil, fmt.Errorf("protocol host returned no reply")
		}
		return json.Marshal(msg.Reply)
	case <-p.done:
		return nil, fmt.Errorf("protocol host closed")
	}
}

func (p *protocolPeer) removePending(id uint64) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *protocolPeer) write(msg plugin.ProtocolMessage) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.encoder.Encode(msg)
}

func (p *protocolPeer) fail(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	})
}
