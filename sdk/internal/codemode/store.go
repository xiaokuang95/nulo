package codemode

import "encoding/json"

func StoreType() string { return storeType }

func EncodeStore(m map[string]any) json.RawMessage {
	if m == nil {
		m = map[string]any{}
	}
	b, _ := json.Marshal(m)
	return b
}

func DecodeStore(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
