package solana

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

// btcAddress ports compass-ts convertHexToBtcAddress: first byte is the address kind, the rest the hash.
func btcAddress(b []byte) (string, error) {
	if len(b) < 1 {
		return "", fmt.Errorf("btc receiver must have version prefix")
	}
	version, hash := b[0], b[1:]
	switch version {
	case 0x01:
		if len(hash) == 20 {
			return base58Check(0x00, hash), nil
		}
		if len(hash) == 32 {
			return segwitAddress("bc", 1, hash)
		}
		return "", fmt.Errorf("P2PKH Or P2TR v0 requires 20 or 32-byte hash")
	case 0x05:
		if len(hash) != 20 {
			return "", fmt.Errorf("P2SH requires 20-byte hash")
		}
		return base58Check(0x05, hash), nil
	case 0x00:
		if len(hash) == 20 || len(hash) == 32 {
			return segwitAddress("bc", 0, hash)
		}
		return "", fmt.Errorf("Bech32 v0 requires 20 or 32-byte hash")
	default:
		return "", fmt.Errorf("Unsupported version prefix: 0x%02x", version)
	}
}

func base58Check(version byte, payload []byte) string {
	data := append([]byte{version}, payload...)
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return base58.Encode(append(data, second[:4]...))
}

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// segwitAddress encodes with bech32 for witness v0 and bech32m otherwise (BIP-173/BIP-350).
func segwitAddress(hrp string, witnessVersion byte, program []byte) (string, error) {
	words, err := convertBits(program, 8, 5, true)
	if err != nil {
		return "", err
	}
	data := append([]byte{witnessVersion}, words...)
	constant := uint32(1)
	if witnessVersion != 0 {
		constant = 0x2bc830a3
	}
	values := append(hrpExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	mod := bech32Polymod(values) ^ constant
	var sb strings.Builder
	sb.WriteString(hrp)
	sb.WriteByte('1')
	for _, d := range data {
		sb.WriteByte(bech32Charset[d])
	}
	for i := 0; i < 6; i++ {
		sb.WriteByte(bech32Charset[(mod>>uint(5*(5-i)))&31])
	}
	return sb.String(), nil
}

func bech32Polymod(values []byte) uint32 {
	gen := []uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (top>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func hrpExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

func convertBits(data []byte, from, to uint, pad bool) ([]byte, error) {
	acc, bits := uint32(0), uint(0)
	maxv := uint32(1)<<to - 1
	var out []byte
	for _, b := range data {
		acc = acc<<from | uint32(b)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if pad && bits > 0 {
		out = append(out, byte(acc<<(to-bits)&maxv))
	} else if !pad && (bits >= from || acc<<(to-bits)&maxv != 0) {
		return nil, fmt.Errorf("invalid padding")
	}
	return out, nil
}
