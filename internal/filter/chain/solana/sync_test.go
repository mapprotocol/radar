package solana

import (
	"context"
	"encoding/binary"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/mapprotocol/filter/internal/pkg/dao"
)

func testSig(n int) solana.Signature {
	var s solana.Signature
	binary.BigEndian.PutUint32(s[:4], uint32(n+1))
	return s
}

var (
	poolProgram     = solana.MustPublicKeyFromBase58(defaultChainPoolProgram)
	receiverProgram = solana.MustPublicKeyFromBase58(defaultReceiverProgram)
)

// addSigs registers signatures newest first (as the node returns them) with an empty tx each.
func (h *harness) addSigs(t *testing.T, program solana.PublicKey, ids []int, slotOf func(int) uint64) {
	t.Helper()
	for _, id := range ids {
		sig := testSig(id)
		h.rpc.sigs[program] = append(h.rpc.sigs[program], &rpc.TransactionSignature{Signature: sig, Slot: slotOf(id)})
		if _, ok := h.rpc.txs[sig]; !ok {
			h.rpc.txs[sig] = rpcTx(t, slotOf(id), i64(1), []string{testUser}, nil, nil, nil)
		}
	}
}

func desc(from, to int) []int {
	var out []int
	for i := from; i >= to; i-- {
		out = append(out, i)
	}
	return out
}

func TestSyncPagesOldestFirstAndSavesCursor(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Mcs = c.Mcs[:1] })
	h.rpc.slot = 1000
	h.addSigs(t, poolProgram, desc(249, 0), func(id int) uint64 { return uint64(100 + id) })
	var order []solana.Signature
	h.chain.rpc = &orderRPC{fakeRPC: h.rpc, order: &order}

	h.chain.syncOnce()

	if len(h.rpc.sigCalls) != 3 || *h.rpc.sigCalls[0].Limit != 100 || !h.rpc.sigCalls[0].Before.IsZero() ||
		h.rpc.sigCalls[1].Before != testSig(150) || h.rpc.sigCalls[2].Before != testSig(50) ||
		h.rpc.sigCalls[0].Commitment != rpc.CommitmentFinalized {
		t.Fatalf("signature paging = %+v", h.rpc.sigCalls)
	}
	if len(order) != 250 || order[0] != testSig(0) || order[249] != testSig(249) {
		t.Fatalf("handled %d txs, first %v", len(order), order[0])
	}
	if got := string(h.bs.files[cursorFile(poolProgram)]); got != testSig(249).String() {
		t.Fatalf("cursor = %s", got)
	}
	if !reflect.DeepEqual(h.store.scans, []uint64{1000}) || !reflect.DeepEqual(h.store.latest, []uint64{1000}) {
		t.Fatalf("scans %v latest %v", h.store.scans, h.store.latest)
	}

	// next round asks only for signatures after the cursor
	h.rpc.sigCalls = nil
	h.rpc.slot = 1001
	h.chain.syncOnce()
	if len(h.rpc.sigCalls) != 1 || h.rpc.sigCalls[0].Until != testSig(249) {
		t.Fatalf("second round = %+v", h.rpc.sigCalls)
	}
	if !reflect.DeepEqual(h.store.scans, []uint64{1000, 1001}) {
		t.Fatalf("scans %v, want finalized slot when no new signatures", h.store.scans)
	}
}

type orderRPC struct {
	*fakeRPC
	order *[]solana.Signature
}

func (o *orderRPC) GetTransaction(ctx context.Context, sig solana.Signature, opts *rpc.GetTransactionOpts) (*rpc.GetTransactionResult, error) {
	*o.order = append(*o.order, sig)
	return o.fakeRPC.GetTransaction(ctx, sig, opts)
}

func TestSyncStopsAtFailingTxAndRetriesNextRound(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Mcs = c.Mcs[:1] })
	h.rpc.slot = 500
	h.addSigs(t, poolProgram, []int{3, 2, 1}, func(id int) uint64 { return uint64(100 * id) })
	h.rpc.txErr[testSig(2)] = 4 // fails every attempt of the first round

	h.chain.syncOnce()

	if got := string(h.bs.files[cursorFile(poolProgram)]); got != testSig(1).String() {
		t.Fatalf("cursor = %s, want the last handled signature", got)
	}
	if !reflect.DeepEqual(h.slept, []time.Duration{10 * time.Second, 30 * time.Second, 60 * time.Second}) {
		t.Fatalf("getTransaction retry delays = %v", h.slept)
	}
	if !reflect.DeepEqual(h.store.scans, []uint64{199}) {
		t.Fatalf("scans = %v, want slot before the failing tx", h.store.scans)
	}

	h.slept = nil
	h.chain.syncOnce()
	if got := string(h.bs.files[cursorFile(poolProgram)]); got != testSig(3).String() {
		t.Fatalf("cursor = %s after recovery", got)
	}
	if !reflect.DeepEqual(h.store.scans, []uint64{199, 500}) {
		t.Fatalf("scans = %v", h.store.scans)
	}
}

func TestSyncHandlesSharedTxOnceAndScanIsMinOverPrograms(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Mcs = c.Mcs[:2] })
	h.rpc.slot = 900
	h.addSigs(t, poolProgram, []int{2, 1}, func(id int) uint64 { return uint64(10 * id) })
	h.addSigs(t, receiverProgram, []int{4, 2}, func(id int) uint64 { return uint64(10 * id) })
	h.rpc.txErr[testSig(4)] = 4

	h.chain.syncOnce()

	if h.rpc.txCalls != 2+4 {
		t.Fatalf("getTransaction calls = %d, shared tx must be fetched once", h.rpc.txCalls)
	}
	if got := string(h.bs.files[cursorFile(receiverProgram)]); got != testSig(2).String() {
		t.Fatalf("receiver cursor = %s", got)
	}
	if !reflect.DeepEqual(h.store.scans, []uint64{39}) {
		t.Fatalf("scans = %v, want min over programs", h.store.scans)
	}
}

func TestScanProgressNeedsEveryProgramAndIsMonotonic(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Mcs = c.Mcs[:2] })
	h.chain.progress[poolProgram] = 50
	h.chain.saveScanProgress()
	if len(h.store.scans) != 0 {
		t.Fatalf("scan saved before every program reported: %v", h.store.scans)
	}
	h.chain.progress[receiverProgram] = 40
	h.chain.saveScanProgress()
	h.chain.saveScanProgress()
	if !reflect.DeepEqual(h.store.scans, []uint64{40}) {
		t.Fatalf("scans = %v", h.store.scans)
	}

	b := newHarness(t, func(c *Config) { c.Mcs = c.Mcs[:1] })
	b.chain.isBackUp = true
	b.chain.progress[poolProgram] = 10
	b.chain.saveScanProgress()
	b.chain.saveLatestBlock(10, time.Now())
	if len(b.store.scans) != 0 || len(b.store.latest) != 0 {
		t.Fatal("backup instance must not save scan or latest heights")
	}
}

func TestLoadCursorPrefersLocalOverStartBlock(t *testing.T) {
	h := newHarness(t, func(c *Config) {
		c.Mcs = c.Mcs[:2]
		c.StartBlock = map[string]string{defaultChainPoolProgram: "cfg-pool", defaultReceiverProgram: "cfg-rcvr"}
	})
	h.bs.files[cursorFile(poolProgram)] = []byte("local-pool\n")
	h.chain.loadCursors()
	if h.chain.cursors[poolProgram] != "local-pool" || h.chain.cursors[receiverProgram] != "cfg-rcvr" {
		t.Fatalf("cursors = %v", h.chain.cursors)
	}
}

func TestHandleTransactionRetriesOnlyRetryableErrors(t *testing.T) {
	h := newHarness(t, nil)
	tx := rpcTx(t, 1, i64(1), []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{
		{1, crossOutPayload(crossOutInput{fromChain: 1, toChain: 56, mint: testUSDC, bridgeAmount: 1, toToken: make([]byte, 32),
			receiver: make([]byte, 33), minOut: new(big.Int), sourceToken: make([]byte, 32)})},
	})

	err := h.chain.handleTransactionWithRetry("SIG", tx) // butter stub fails with "fetch failed"
	if err == nil || len(h.reqs) != 3 || !reflect.DeepEqual(h.slept, []time.Duration{time.Second, time.Second}) {
		t.Fatalf("err %v, butter calls %d, slept %v", err, len(h.reqs), h.slept)
	}
	if h.rpc.infoCalls != 1 {
		t.Fatalf("mint decimals fetched %d times, want cached", h.rpc.infoCalls)
	}

	h.reqs, h.slept = nil, nil
	delete(h.rpc.decimals, solana.MustPublicKeyFromBase58(testUSDC))
	h.chain.decimals = map[solana.PublicKey]int{}
	err = h.chain.handleTransactionWithRetry("SIG", tx)
	if err == nil || len(h.slept) != 0 {
		t.Fatalf("non retryable error retried: %v %v", err, h.slept)
	}
	if !strings.Contains(failureMessage(testChainId, defaultChainPoolProgram, "SIG", "handleTransaction", err), "retryable=false") {
		t.Fatal("failure message")
	}
}

func TestHandleTransactionSkipsUnsupportedMintFailedTxAndUnregistered(t *testing.T) {
	h := newHarness(t, nil)
	h.butter = map[string]interface{}{"data": "0x", "relay": false, "receiver": "r"}
	out := func(mint string) []byte {
		return crossOutPayload(crossOutInput{fromChain: 1, toChain: 56, mint: mint, bridgeAmount: 1, toToken: make([]byte, 32),
			receiver: make([]byte, 33), minOut: new(big.Int), sourceToken: make([]byte, 32)})
	}
	tx := rpcTx(t, 1, i64(1), []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{{1, out(testOther.String())}})
	if err := h.chain.handleTransaction("SIG", tx); err != nil || len(h.store.rows) != 0 || len(h.reqs) != 0 {
		t.Fatalf("unsupported mint: err %v rows %d", err, len(h.store.rows))
	}

	failed := rpcTx(t, 1, i64(1), []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{{1, out(testUSDC)}})
	failed.Meta.Err = map[string]interface{}{"InstructionError": []interface{}{0, "Custom"}}
	if err := h.chain.handleTransaction("SIG", failed); err != nil || len(h.store.rows) != 0 {
		t.Fatalf("failed tx: err %v rows %d", err, len(h.store.rows))
	}

	h.chain.events = nil
	ok := rpcTx(t, 1, i64(1), []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{{1, out(testUSDC)}})
	if err := h.chain.handleTransaction("SIG", ok); err != nil || len(h.store.rows) != 0 || len(h.reqs) != 0 {
		t.Fatalf("unregistered event: err %v rows %d butter %d", err, len(h.store.rows), len(h.reqs))
	}
}

func TestInsertOneRowPerRegisteredEvent(t *testing.T) {
	h := newHarness(t, nil)
	h.store.events = append(h.store.events, &dao.Event{Id: 200, ProjectId: 9, Address: defaultReceiverProgram, Topic: TopicCrossIn})
	if err := h.chain.getMatch(); err != nil {
		t.Fatal(err)
	}
	tx := rpcTx(t, 7, i64(1), []string{testUser, defaultReceiverProgram}, nil, nil, []innerIx{
		{1, crossInPayload(56, testUSDC, 1, testWSOL, 1, 1, 1)},
	})
	if err := h.chain.handleTransaction("SIG", tx); err != nil {
		t.Fatal(err)
	}
	rows := h.rowsFor(TopicCrossIn)
	if len(rows) != 2 || rows[0].EventId != 124 || rows[1].EventId != 200 || rows[1].ProjectId != 9 || rows[0].LogData != rows[1].LogData {
		t.Fatalf("rows = %+v", rows)
	}

	h.store.rows, h.store.failMos = nil, 1
	if err := h.chain.handleTransaction("SIG", tx); err == nil {
		t.Fatal("insert error must fail the tx so the cursor does not move")
	}
}
