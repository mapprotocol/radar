package solana

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
)

// undefined marks a key whose JS value is undefined: JSON.stringify drops it but the key keeps its slot.
type undefined struct{}

// orderedMap mirrors JSON.stringify(Object.fromEntries(map)): insertion order, re-set keeps the first slot.
type orderedMap struct {
	keys []string
	vals map[string]interface{}
}

func newOrderedMap() *orderedMap {
	return &orderedMap{vals: make(map[string]interface{})}
}

func (m *orderedMap) Set(key string, val interface{}) {
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

func (m *orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, k := range m.keys {
		v := m.vals[k]
		if _, skip := v.(undefined); skip {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, err := jsMarshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := jsMarshal(v)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *orderedMap) String() (string, error) {
	b, err := jsMarshal(m)
	return string(b), err
}

// jsMarshal encodes without Go's HTML escaping, as JSON.stringify does.
func jsMarshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// numberHex is compass-ts normalizeEventNumberHex: lower hex, no 0x, padded to even length.
func numberHex(v *big.Int) string {
	if v == nil {
		return "00"
	}
	h := v.Text(16)
	if len(h)%2 != 0 {
		h = "0" + h
	}
	return h
}

func u64Hex(v uint64) string {
	return numberHex(new(big.Int).SetUint64(v))
}

func byteArray(b []byte) []int {
	out := make([]int, len(b))
	for i, x := range b {
		out[i] = int(x)
	}
	return out
}

func u16Array(v []uint16) []int {
	out := make([]int, len(v))
	for i, x := range v {
		out[i] = int(x)
	}
	return out
}

// formatUnits matches ethers v6 formatUnits: trailing zeros trimmed, at least one decimal unless decimals is 0.
func formatUnits(v *big.Int, decimals int) string {
	s := v.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if decimals == 0 {
		if neg {
			return "-" + s
		}
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	whole, frac := s[:len(s)-decimals], strings.TrimRight(s[len(s)-decimals:], "0")
	if frac == "" {
		frac = "0"
	}
	if neg {
		whole = "-" + whole
	}
	return whole + "." + frac
}

func isZeroBytes(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}
