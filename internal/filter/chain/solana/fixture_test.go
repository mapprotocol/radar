package solana

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"

	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/stream"
)

const (
	testChainId = "1360108768460801"
	testUser    = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	testUSDC    = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	testWSOL    = "So11111111111111111111111111111111111111112"
)

var (
	testAlias = solana.PublicKeyFromBytes(bytes.Repeat([]byte{7}, 32))
	testOther = solana.PublicKeyFromBytes(bytes.Repeat([]byte{9}, 32))
)

// borsh payload builders, same layout as the anchor IDL of the chain pool and receiver programs

func le64(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

func le128(v *big.Int) []byte {
	b := make([]byte, 16)
	be := v.FillBytes(make([]byte, 16))
	for i := range be {
		b[15-i] = be[i]
	}
	return b
}

func vec16(v []uint16) []byte {
	b := make([]byte, 4, 4+2*len(v))
	binary.LittleEndian.PutUint32(b, uint32(len(v)))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint16(b, x)
	}
	return b
}

func seq(n int, start byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = start + byte(i)
	}
	return b
}

func pk(s string) []byte {
	p := solana.MustPublicKeyFromBase58(s)
	return p[:]
}

func cat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

type crossOutInput struct {
	fromChain, toChain, bridgeAmount, sourceAmount uint64
	mint                                           string
	toToken, receiver, sourceToken                 []byte
	minOut                                         *big.Int
	referer, fee                                   []uint16
}

func crossOutPayload(in crossOutInput) []byte {
	return cat(eventCpiPrefix, discCrossOut, seq(32, 1), pk(testUser), le64(in.fromChain), le64(in.toChain), pk(in.mint),
		le64(in.bridgeAmount), in.toToken, in.receiver, le128(in.minOut), in.sourceToken, le64(in.sourceAmount),
		vec16(in.referer), vec16(in.fee))
}

func crossInPayload(fromChain uint64, mint string, bridgeAmount uint64, dest string, amountOut, signedMin, effectiveMin uint64) []byte {
	return cat(eventCpiPrefix, discCrossIn, seq(32, 0x40), pk(testUser), le64(fromChain), pk(mint), le64(bridgeAmount),
		pk(dest), le64(amountOut), le64(signedMin), le64(effectiveMin))
}

func refundPayload(fromChain uint64, mint, dest string, amount uint64) []byte {
	return cat(eventCpiPrefix, discRefund, seq(32, 0x80), pk(testUser), le64(fromChain), pk(mint), pk(dest), le64(amount))
}

func releasePayload() []byte {
	return cat(eventCpiPrefix, eventDiscriminator("ReleaseEvent"), seq(32, 0), pk(testUSDC), le64(1), pk(testUser))
}

type innerIx struct {
	program int
	data    []byte
}

// rpcTx renders the getTransaction json response (encoding json) and decodes it like the rpc client does.
func rpcTx(t *testing.T, slot uint64, blockTime *int64, keys, writable, readonly []string, ixs []innerIx) *rpc.GetTransactionResult {
	t.Helper()
	instructions := make([]map[string]interface{}, 0, len(ixs))
	for _, ix := range ixs {
		instructions = append(instructions, map[string]interface{}{
			"programIdIndex": ix.program, "accounts": []int{}, "data": base58.Encode(ix.data), "stackHeight": 2,
		})
	}
	message := map[string]interface{}{
		"accountKeys":     keys,
		"header":          map[string]int{"numRequiredSignatures": 1, "numReadonlySignedAccounts": 0, "numReadonlyUnsignedAccounts": 1},
		"recentBlockhash": testWSOL,
		"instructions":    []interface{}{},
	}
	version := interface{}("legacy")
	if writable != nil || readonly != nil {
		version = 0
		message["addressTableLookups"] = []map[string]interface{}{{
			"accountKey": testOther.String(), "writableIndexes": seqInts(len(writable)), "readonlyIndexes": seqInts(len(readonly)),
		}}
	}
	if writable == nil {
		writable = []string{}
	}
	if readonly == nil {
		readonly = []string{}
	}
	body := map[string]interface{}{
		"slot":      slot,
		"blockTime": blockTime,
		"version":   version,
		"meta": map[string]interface{}{
			"err": nil, "fee": 5000, "preBalances": []int{}, "postBalances": []int{},
			"innerInstructions": []map[string]interface{}{{"index": 0, "instructions": instructions}},
			"loadedAddresses":   map[string]interface{}{"writable": writable, "readonly": readonly},
		},
		"transaction": map[string]interface{}{"signatures": []string{}, "message": message},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var out rpc.GetTransactionResult
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode tx: %v", err)
	}
	return &out
}

func seqInts(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func i64(v int64) *int64 { return &v }

type fakeRPC struct {
	slot      uint64
	slotErr   error
	sigs      map[solana.PublicKey][]*rpc.TransactionSignature
	sigCalls  []*rpc.GetSignaturesForAddressOpts
	txs       map[solana.Signature]*rpc.GetTransactionResult
	txErr     map[solana.Signature]int // remaining failures
	txCalls   int
	decimals  map[solana.PublicKey]byte
	infoCalls int
}

func (f *fakeRPC) GetSlot(context.Context, rpc.CommitmentType) (uint64, error) {
	return f.slot, f.slotErr
}

// GetSignaturesForAddressWithOpts serves newest first, honoring until/before/limit like the node.
func (f *fakeRPC) GetSignaturesForAddressWithOpts(_ context.Context, account solana.PublicKey,
	opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	f.sigCalls = append(f.sigCalls, opts)
	all := f.sigs[account]
	start := 0
	if !opts.Before.IsZero() {
		for i, s := range all {
			if s.Signature == opts.Before {
				start = i + 1
			}
		}
	}
	var out []*rpc.TransactionSignature
	for _, s := range all[start:] {
		if s.Signature == opts.Until || len(out) == *opts.Limit {
			break
		}
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeRPC) GetTransaction(_ context.Context, sig solana.Signature, _ *rpc.GetTransactionOpts) (*rpc.GetTransactionResult, error) {
	f.txCalls++
	if f.txErr[sig] > 0 {
		f.txErr[sig]--
		return nil, fmt.Errorf("rpc 503")
	}
	if tx, ok := f.txs[sig]; ok {
		return tx, nil
	}
	return nil, rpc.ErrNotFound
}

func (f *fakeRPC) GetAccountInfoWithOpts(_ context.Context, account solana.PublicKey, _ *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	f.infoCalls++
	d, ok := f.decimals[account]
	if !ok {
		return nil, rpc.ErrNotFound
	}
	data := make([]byte, 82)
	data[44] = d
	raw := fmt.Sprintf(`{"context":{"slot":1},"value":{"data":["%s","base64"],"executable":false,"lamports":1,"owner":"%s","rentEpoch":0}}`,
		base64.StdEncoding.EncodeToString(data), solana.TokenProgramID)
	var out rpc.GetAccountInfoResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type fakeStore struct {
	rows     []*dao.Mos
	events   []*dao.Event
	scans    []uint64
	latest   []uint64
	failMos  int
	mosCalls int
}

func (s *fakeStore) Type() string { return constant.Mysql }
func (s *fakeStore) Mos(_ uint64, rows []*dao.Mos) error {
	s.mosCalls++
	if s.failMos > 0 {
		s.failMos--
		return fmt.Errorf("db timeout")
	}
	s.rows = append(s.rows, rows...)
	return nil
}
func (s *fakeStore) LatestBlockNumber(_ string, n uint64) error {
	s.latest = append(s.latest, n)
	return nil
}
func (s *fakeStore) ScanBlockNumber(_ string, n uint64) error {
	s.scans = append(s.scans, n)
	return nil
}
func (s *fakeStore) GetEvent(id int64) ([]*dao.Event, error) {
	var out []*dao.Event
	for _, e := range s.events {
		if e.Id > id {
			out = append(out, e)
		}
	}
	return out, nil
}

type memStore struct{ files map[string][]byte }

func (m *memStore) StoreBlock(*big.Int) error                  { return nil }
func (m *memStore) TryLoadLatestBlock() (*big.Int, error)      { return new(big.Int), nil }
func (m *memStore) DelFile(name string) error                  { delete(m.files, name); return nil }
func (m *memStore) ReadCustom(name string) ([]byte, error)     { return m.files[name], nil }
func (m *memStore) CustomStore(name string, data []byte) error { m.files[name] = data; return nil }

type harness struct {
	chain  *Chain
	rpc    *fakeRPC
	store  *fakeStore
	bs     *memStore
	butter map[string]interface{}
	reqs   []*stream.BridgeDataRequest
	slept  []time.Duration
}

func defaultEvents() []*dao.Event {
	return []*dao.Event{
		{Id: 98, ProjectId: 7, ChainId: testChainId, Address: defaultChainPoolProgram, Topic: TopicCrossOut},
		{Id: 124, ProjectId: 7, ChainId: testChainId, Address: defaultReceiverProgram, Topic: TopicCrossIn},
		{Id: 125, ProjectId: 7, ChainId: testChainId, Address: testAlias.String(), Topic: TopicCrossIn},
		{Id: 126, ProjectId: 8, ChainId: "56", Address: defaultChainPoolProgram, Topic: TopicCrossOut},
	}
}

func newHarness(t *testing.T, opts func(*Config)) *harness {
	t.Helper()
	cfg := &Config{
		Name:                  "solana",
		Id:                    testChainId,
		Mcs:                   []solana.PublicKey{solana.MustPublicKeyFromBase58(defaultChainPoolProgram), solana.MustPublicKeyFromBase58(defaultReceiverProgram), testAlias},
		StartBlock:            map[string]string{},
		SupportedBridgeMints:  map[string]struct{}{testUSDC: {}, testWSOL: {}, "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU": {}},
		NativeTokenOutAddress: defaultNativeTokenOutAddress,
		ButterEntrance:        "butter+",
	}
	if opts != nil {
		opts(cfg)
	}
	h := &harness{
		rpc: &fakeRPC{
			sigs:     map[solana.PublicKey][]*rpc.TransactionSignature{},
			txs:      map[solana.Signature]*rpc.GetTransactionResult{},
			txErr:    map[solana.Signature]int{},
			decimals: map[solana.PublicKey]byte{solana.MustPublicKeyFromBase58(testUSDC): 6, solana.MustPublicKeyFromBase58(testWSOL): 9},
		},
		store: &fakeStore{events: defaultEvents()},
		bs:    &memStore{files: map[string][]byte{}},
	}
	h.chain = newChain(cfg, nil, h.bs, h.rpc, false)
	h.chain.storages = append(h.chain.storages, h.store)
	h.chain.log = log.New()
	h.chain.sleep = func(d time.Duration) { h.slept = append(h.slept, d) }
	h.chain.bridgeData = func(req *stream.BridgeDataRequest) (map[string]interface{}, error) {
		h.reqs = append(h.reqs, req)
		if h.butter == nil {
			return nil, &stream.ExternalRequestError{URL: "u", Msg: "fetch failed"}
		}
		return h.butter, nil
	}
	if err := h.chain.getMatch(); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) rowsFor(topic string) []*dao.Mos {
	var out []*dao.Mos
	for _, r := range h.store.rows {
		if r.Topic == topic {
			out = append(out, r)
		}
	}
	return out
}

func mustAddrBytes(addr string, pad int) []byte {
	return cat(make([]byte, pad), common.HexToAddress(addr).Bytes())
}

func diffAt(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 40
			if lo < 0 {
				lo = 0
			}
			return fmt.Sprintf("first diff at %d:\n got ...%s\nwant ...%s", i, a[lo:min(len(a), i+40)], b[lo:min(len(b), i+40)])
		}
	}
	return fmt.Sprintf("length got %d want %d", len(a), len(b))
}

func assertJSON(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s log_data mismatch, %s", name, diffAt(got, want))
	}
	if strings.Contains(got, `\u003c`) {
		t.Fatalf("%s html escaped", name)
	}
}
