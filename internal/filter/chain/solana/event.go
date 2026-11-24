package solana

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	defaultChainPoolProgram = "mosv35NjuYkM8iMdihM96cG2ZziU2RJYZfSaGuV8uF6"
	defaultReceiverProgram  = "rcvr2wL88Bg3eBGH4Hr7VjugbDLm5YafBraggmmuRGJ"
)

var (
	eventCpiPrefix = []byte{0xe4, 0x45, 0xa5, 0x2e, 0x51, 0xcb, 0x9a, 0x1d}

	discCrossOut = eventDiscriminator("CrossOutEvent")
	discCrossIn  = eventDiscriminator("CrossInEvent")
	discRefund   = eventDiscriminator("RefundEvent")

	errShortData = errors.New("borsh: unexpected end of data")
)

func eventDiscriminator(name string) []byte {
	h := sha256.Sum256([]byte("event:" + name))
	return h[:8]
}

type CrossOutEvent struct {
	OrderId      [32]byte
	User         solana.PublicKey
	FromChain    uint64
	ToChain      uint64
	BridgeMint   solana.PublicKey
	BridgeAmount uint64
	ToToken      [32]byte
	Receiver     [33]byte
	MinAmountOut *big.Int // u128
	SourceToken  [32]byte
	SourceAmount uint64
	RefererId    []uint16
	FeeRatio     []uint16
}

// CrossInEvent carries both receiver events; nil fields are absent from the emitted event.
type CrossInEvent struct {
	Name                  string // CrossInEvent or RefundEvent
	OrderId               [32]byte
	User                  solana.PublicKey
	FromChain             *uint64
	BridgeMint            solana.PublicKey
	BridgeAmount          *uint64
	DestToken             *solana.PublicKey
	Amount                *uint64
	AmountOut             *uint64
	SignedMinAmountOut    *uint64
	EffectiveMinAmountOut *uint64
}

type parsedCrossOut struct {
	event   *CrossOutEvent
	program solana.PublicKey
}

type parsedCrossIn struct {
	event   *CrossInEvent
	program solana.PublicKey
}

type parser struct {
	chainPool, receiver solana.PublicKey
	aliases             []solana.PublicKey
}

func newParser(eventPrograms []solana.PublicKey) *parser {
	return &parser{
		chainPool: solana.MustPublicKeyFromBase58(defaultChainPoolProgram),
		receiver:  solana.MustPublicKeyFromBase58(defaultReceiverProgram),
		aliases:   eventPrograms,
	}
}

func (p *parser) isAlias(program solana.PublicKey) bool {
	for _, a := range p.aliases {
		if a.Equals(program) {
			return true
		}
	}
	return false
}

// accountKeys returns static keys followed by loaded writable and readonly addresses (v0 lookups).
func accountKeys(tx *rpc.GetTransactionResult) (solana.PublicKeySlice, error) {
	if tx == nil || tx.Transaction == nil {
		return nil, nil
	}
	parsed, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, nil
	}
	keys := append(solana.PublicKeySlice{}, parsed.Message.AccountKeys...)
	if tx.Meta != nil {
		keys = append(keys, tx.Meta.LoadedAddresses.Writable...)
		keys = append(keys, tx.Meta.LoadedAddresses.ReadOnly...)
	}
	return keys, nil
}

// parse extracts emit_cpi events from inner instructions the way compass-ts SolEventParser does.
func (p *parser) parse(tx *rpc.GetTransactionResult) ([]parsedCrossOut, []parsedCrossIn, error) {
	keys, err := accountKeys(tx)
	if err != nil || keys == nil || tx.Meta == nil {
		return nil, nil, err
	}
	var outs []parsedCrossOut
	var ins []parsedCrossIn
	for _, group := range tx.Meta.InnerInstructions {
		for _, ix := range group.Instructions {
			if int(ix.ProgramIDIndex) >= len(keys) {
				continue
			}
			program := keys[ix.ProgramIDIndex]
			body := eventBody(ix.Data)
			switch {
			case program.Equals(p.chainPool):
				if ev := decodeCrossOut(body); ev != nil {
					outs = append(outs, parsedCrossOut{event: ev, program: program})
				}
			case program.Equals(p.receiver):
				if ev := decodeCrossIn(body); ev != nil {
					ins = append(ins, parsedCrossIn{event: ev, program: program})
				}
			case p.isAlias(program):
				if ev := decodeCrossOut(body); ev != nil {
					outs = append(outs, parsedCrossOut{event: ev, program: program})
				} else if ev := decodeCrossIn(body); ev != nil {
					ins = append(ins, parsedCrossIn{event: ev, program: program})
				}
			}
		}
	}
	return outs, ins, nil
}

// eventBody strips the emit_cpi prefix, returning discriminator+borsh or nil.
func eventBody(data []byte) []byte {
	if len(data) <= len(eventCpiPrefix) || !bytes.Equal(data[:len(eventCpiPrefix)], eventCpiPrefix) {
		return nil
	}
	return data[len(eventCpiPrefix):]
}

func decodeCrossOut(body []byte) *CrossOutEvent {
	if len(body) < 8 || !bytes.Equal(body[:8], discCrossOut) {
		return nil
	}
	r := &reader{buf: body[8:]}
	ev := &CrossOutEvent{}
	r.fixed(ev.OrderId[:])
	ev.User = r.pubkey()
	ev.FromChain = r.u64()
	ev.ToChain = r.u64()
	ev.BridgeMint = r.pubkey()
	ev.BridgeAmount = r.u64()
	r.fixed(ev.ToToken[:])
	r.fixed(ev.Receiver[:])
	ev.MinAmountOut = r.u128()
	r.fixed(ev.SourceToken[:])
	ev.SourceAmount = r.u64()
	ev.RefererId = r.vecU16()
	ev.FeeRatio = r.vecU16()
	if r.err != nil {
		return nil
	}
	return ev
}

func decodeCrossIn(body []byte) *CrossInEvent {
	if len(body) < 8 {
		return nil
	}
	r := &reader{buf: body[8:]}
	ev := &CrossInEvent{}
	switch {
	case bytes.Equal(body[:8], discCrossIn):
		ev.Name = "CrossInEvent"
		r.fixed(ev.OrderId[:])
		ev.User = r.pubkey()
		ev.FromChain = r.u64p()
		ev.BridgeMint = r.pubkey()
		ev.BridgeAmount = r.u64p()
		dest := r.pubkey()
		ev.DestToken = &dest
		ev.AmountOut = r.u64p()
		ev.SignedMinAmountOut = r.u64p()
		ev.EffectiveMinAmountOut = r.u64p()
	case bytes.Equal(body[:8], discRefund):
		ev.Name = "RefundEvent"
		r.fixed(ev.OrderId[:])
		ev.User = r.pubkey()
		ev.FromChain = r.u64p()
		ev.BridgeMint = r.pubkey()
		dest := r.pubkey()
		ev.DestToken = &dest
		ev.Amount = r.u64p()
	default:
		return nil
	}
	if r.err != nil {
		return nil
	}
	return ev
}

// reader decodes fixed borsh layouts; trailing bytes are ignored like buffer-layout does.
type reader struct {
	buf []byte
	off int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || len(r.buf)-r.off < n {
		r.err = errShortData
		return nil
	}
	b := r.buf[r.off : r.off+n]
	r.off += n
	return b
}

func (r *reader) fixed(dst []byte) {
	if b := r.take(len(dst)); b != nil {
		copy(dst, b)
	}
}

func (r *reader) pubkey() (pk solana.PublicKey) {
	r.fixed(pk[:])
	return pk
}

func (r *reader) u64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (r *reader) u64p() *uint64 {
	v := r.u64()
	return &v
}

func (r *reader) u128() *big.Int {
	b := r.take(16)
	if b == nil {
		return new(big.Int)
	}
	be := make([]byte, 16)
	for i := range b {
		be[15-i] = b[i]
	}
	return new(big.Int).SetBytes(be)
}

func (r *reader) vecU16() []uint16 {
	b := r.take(4)
	if b == nil {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n > (len(r.buf)-r.off)/2 {
		r.err = errShortData
		return nil
	}
	out := make([]uint16, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(r.take(2))
	}
	return out
}
