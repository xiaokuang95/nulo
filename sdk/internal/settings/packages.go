package settings

import (
	"bytes"
	"encoding/json"
	"sort"
)

// PackageSource is Pi's packages entry: a bare source string or an object with
// resource filters. Object form is kept distinct because Pi treats any object
// as filtered, even without filter fields.
type PackageSource struct {
	Source     string   `json:"source,omitempty"`
	Autoload   *bool    `json:"autoload,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	Skills     []string `json:"skills,omitempty"`
	Prompts    []string `json:"prompts,omitempty"`
	Themes     []string `json:"themes,omitempty"`

	WasObject bool `json:"-"`
	keyOrder  []string
	unknown   map[string]json.RawMessage
}

// Filtered reports whether the object form (or an explicit filter) is used.
func (p PackageSource) Filtered() bool {
	return p.WasObject || p.Autoload != nil || p.Extensions != nil || p.Skills != nil || p.Prompts != nil || p.Themes != nil
}

// MarshalJSON writes the string or object wire shape.
func (p PackageSource) MarshalJSON() ([]byte, error) {
	if !p.Filtered() {
		return marshalJSONValue(p.Source)
	}
	members := map[string]json.RawMessage{}
	put := func(key string, value any) {
		if _, exists := members[key]; exists {
			return
		}
		raw, err := marshalJSONValue(value)
		if err == nil {
			members[key] = raw
		}
	}
	put("source", p.Source)
	if p.Autoload != nil {
		put("autoload", *p.Autoload)
	}
	if p.Extensions != nil {
		put("extensions", p.Extensions)
	}
	if p.Skills != nil {
		put("skills", p.Skills)
	}
	if p.Prompts != nil {
		put("prompts", p.Prompts)
	}
	if p.Themes != nil {
		put("themes", p.Themes)
	}
	for key, value := range p.unknown {
		if _, exists := members[key]; !exists {
			members[key] = value
		}
	}
	order := make([]string, 0, len(members))
	seen := map[string]bool{}
	add := func(key string) {
		if _, ok := members[key]; !ok || seen[key] {
			return
		}
		seen[key] = true
		order = append(order, key)
	}
	for _, key := range p.keyOrder {
		add(key)
	}
	for _, key := range []string{"source", "autoload", "extensions", "skills", "prompts", "themes"} {
		add(key)
	}
	unknownKeys := make([]string, 0, len(p.unknown))
	for key := range p.unknown {
		unknownKeys = append(unknownKeys, key)
	}
	sort.Strings(unknownKeys)
	for _, key := range unknownKeys {
		add(key)
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range order {
		if i > 0 {
			buf.WriteByte(',')
		}
		encodedKey, _ := marshalJSONValue(key)
		buf.Write(encodedKey)
		buf.WriteByte(':')
		buf.Write(members[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON reads the string or object wire shape.
func (p *PackageSource) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var source string
		if err := json.Unmarshal(trimmed, &source); err != nil {
			return err
		}
		*p = PackageSource{Source: source}
		return nil
	}
	type wire PackageSource
	var object wire
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return err
	}
	*p = PackageSource(object)
	p.WasObject = true
	var members map[string]json.RawMessage
	if json.Unmarshal(trimmed, &members) == nil {
		p.keyOrder = objectKeyOrder(trimmed)
		for key, value := range members {
			switch key {
			case "source", "autoload", "extensions", "skills", "prompts", "themes":
			default:
				if p.unknown == nil {
					p.unknown = map[string]json.RawMessage{}
				}
				p.unknown[key] = value
			}
		}
	}
	return nil
}

// objectKeyOrder returns the authored keys of a JSON object in source order,
// including duplicates, so an object package keeps Pi's member order.
func objectKeyOrder(data []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil
	}
	var order []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return order
		}
		key, ok := token.(string)
		if !ok {
			return order
		}
		order = append(order, key)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return order
		}
	}
	return order
}

func decodePackageSources(raw json.RawMessage) ([]PackageSource, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	if trimmed[0] != '[' {
		return nil, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, false
	}
	out := make([]PackageSource, 0, len(items))
	for _, item := range items {
		var source PackageSource
		if err := json.Unmarshal(item, &source); err != nil {
			continue
		}
		out = append(out, source)
	}
	return out, true
}

func clonePackageSources(items []PackageSource) []PackageSource {
	if items == nil {
		return nil
	}
	out := make([]PackageSource, len(items))
	for i, item := range items {
		out[i] = item
		if item.Autoload != nil {
			value := *item.Autoload
			out[i].Autoload = &value
		}
		out[i].Extensions = cloneStrings(item.Extensions)
		out[i].Skills = cloneStrings(item.Skills)
		out[i].Prompts = cloneStrings(item.Prompts)
		out[i].Themes = cloneStrings(item.Themes)
		if item.unknown != nil {
			out[i].unknown = map[string]json.RawMessage{}
			for key, value := range item.unknown {
				out[i].unknown[key] = append(json.RawMessage(nil), value...)
			}
		}
		out[i].keyOrder = append([]string(nil), item.keyOrder...)
	}
	return out
}
