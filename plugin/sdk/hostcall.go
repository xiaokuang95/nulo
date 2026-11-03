package sdk

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/xiaokuang95/nulo/plugin"
)

var nextCallID atomic.Uint64

const hostBufferSize = 8 << 20

func (s *server) hostCall(method string, params any, out any) error {
	if s.hostCallPtr == 0 && s.hostCallFn == nil {
		return fmt.Errorf("plugin: host callback is not available")
	}
	var rawParams json.RawMessage
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		rawParams = raw
	}
	call := plugin.HostCall{ID: nextCallID.Add(1), Plugin: s.name, Method: method, Params: rawParams}
	request, err := json.Marshal(call)
	if err != nil {
		return err
	}
	response, err := s.invokeHost(request)
	if err != nil {
		return err
	}
	return s.decodeHostReply(method, response, out)
}

func (s *server) decodeHostReply(method string, response []byte, out any) error {
	var reply struct {
		ID     uint64           `json:"id"`
		Result json.RawMessage  `json:"result"`
		Error  *plugin.APIError `json:"error"`
	}
	if err := json.Unmarshal(response, &reply); err != nil {
		return fmt.Errorf("plugin: decode host reply: %w", err)
	}
	if reply.Error != nil {
		return reply.Error
	}
	if out != nil && len(reply.Result) > 0 && string(reply.Result) != "null" {
		if err := json.Unmarshal(reply.Result, out); err != nil {
			return fmt.Errorf("plugin: decode host result for %s: %w", method, err)
		}
	}
	return nil
}

func (s *server) hostCallVoid(method string, params any) error {
	return s.hostCall(method, params, nil)
}

func (s *server) invokeHost(request []byte) ([]byte, error) {
	if s.hostCallFn != nil {
		return s.hostCallFn(request)
	}
	out := make([]byte, hostBufferSize)
	var needed int32
	reqPtr := uintptr(unsafe.Pointer(&request[0]))
	res, _, callErr := purego.SyscallN(
		s.hostCallPtr,
		reqPtr,
		uintptr(len(request)),
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(len(out)),
		uintptr(unsafe.Pointer(&needed)),
	)
	if callErr != 0 {
		return nil, fmt.Errorf("plugin: host call: errno %d", callErr)
	}
	if res == 0 {
		return nil, fmt.Errorf("plugin: host response needs %d bytes; the bridge limit is %d", needed, len(out))
	}
	if needed < 0 || int(needed) > len(out) {
		return nil, fmt.Errorf("plugin: host returned an invalid response length %d", needed)
	}
	return append([]byte(nil), out[:needed]...), nil
}
