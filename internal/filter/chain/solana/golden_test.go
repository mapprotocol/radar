package solana

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/mapprotocol/filter/internal/pkg/stream"
)

// golden log_data produced by running compass-ts SolEventParser + SolEventHandler (node, mocked
// butter/mysql/getAccountInfo) on the same transactions the tests below construct.
var goldenLogData = map[string]string{
	"crossout_evm": `{"orderId":"0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20","user":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","from":[126,140,8,135,96,191,222,29,221,207,50,193,127,32,155,130,66,238,82,170,241,49,250,205,136,208,234,44,109,11,6,242],"fromChain":"04d50300000001","fromChainId":"04d50300000001","toChain":"38","bridgeMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","bridgeAmount":"1234567","tokenAmount":"03e7","toToken":[0,0,0,0,0,0,0,0,0,0,0,0,85,211,152,50,111,153,5,159,247,117,72,82,70,153,144,39,179,25,121,85],"receiver":"0x00000000000000000000000000000000000000AA","minAmountOut":"400000000000000005","swapTokenOutMinAmountOut":"400000000000000005","fromToken":[6,155,136,87,254,171,129,132,251,104,127,99,70,24,192,53,218,196,57,220,26,235,59,85,152,160,240,0,0,0,0,1],"amountOut":"03e7","refererId":[9,1201,7],"feeRatio":[30,0],"originReceiver":[0,0,0,0,0,0,0,0,0,0,0,0,0,171,88,1,167,211,152,53,27,139,225,28,67,158,5,197,179,37,154,236,155],"relay":true,"swapData":"0xdeadbeef<>&"}`,
	"crossout_btc": `{"orderId":"0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20","user":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","from":[126,140,8,135,96,191,222,29,221,207,50,193,127,32,155,130,66,238,82,170,241,49,250,205,136,208,234,44,109,11,6,242],"fromChain":"04d50300000001","fromChainId":"04d50300000001","toChain":"04d50000000001","bridgeMint":"So11111111111111111111111111111111111111112","bridgeAmount":"2000000000","tokenAmount":"00","toToken":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"minAmountOut":"00","swapTokenOutMinAmountOut":"00","fromToken":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"amountOut":"00","refererId":[],"feeRatio":[5],"originReceiver":[0,0,0,0,0,0,0,0,0,0,0,0,0,160,161,162,163,164,165,166,167,168,169,170,171,172,173,174,175,176,177,178,179],"relay":false,"swapData":"0x"}`,
	"crossin_v0":   `{"orderId":"404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f","user":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","from":[126,140,8,135,96,191,222,29,221,207,50,193,127,32,155,130,66,238,82,170,241,49,250,205,136,208,234,44,109,11,6,242],"fromToken":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"fromChainId":"38","bridgeMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","bridgeAmount":"4c4b40","tokenAmount":"4c4b40","destToken":"So11111111111111111111111111111111111111112","toToken":[6,155,136,87,254,171,129,132,251,104,127,99,70,24,192,53,218,196,57,220,26,235,59,85,152,160,240,0,0,0,0,1],"amountOut":"7b","signedMinAmountOut":"00","minAmountOut":"00","swapTokenOutMinAmountOut":"00","effectiveMinAmountOut":"4d","refererId":null,"feeRatio":null,"receiver":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"}`,
	"refund_alias": `{"orderId":"808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f","user":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","from":[126,140,8,135,96,191,222,29,221,207,50,193,127,32,155,130,66,238,82,170,241,49,250,205,136,208,234,44,109,11,6,242],"fromToken":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"fromChainId":"01","bridgeMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","tokenAmount":"00","destToken":"So11111111111111111111111111111111111111112","toToken":[6,155,136,87,254,171,129,132,251,104,127,99,70,24,192,53,218,196,57,220,26,235,59,85,152,160,240,0,0,0,0,1],"amount":"2a","minAmountOut":"00","swapTokenOutMinAmountOut":"00","refererId":null,"feeRatio":null,"receiver":"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM","isrefund":true}`,
}

// compass-ts convertHexToBtcAddress outputs (bitcoinjs-lib)
var goldenBtc = map[string]string{
	"001111111111111111111111111111111111111111":                         "bc1qzyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3h8ffkz",
	"002222222222222222222222222222222222222222222222222222222222222222": "bc1qyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3qrkjgc9",
	"013333333333333333333333333333333333333333":                         "15fioDrrk36NmHdRCtGTu2TMj6rPXzG3sn",
	"014444444444444444444444444444444444444444444444444444444444444444": "bc1pg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zq8ss0kf",
	"055555555555555555555555555555555555555555":                         "39UDauvY7xog2eQo8a7NFfnXh2NhXV6iH1",
}

func TestGoldenCrossOutEvm(t *testing.T) {
	h := newHarness(t, nil)
	h.butter = map[string]interface{}{"data": "0xdeadbeef<>&", "relay": true, "receiver": "0x00000000000000000000000000000000000000AA", "extra": 1}
	toToken := mustAddrBytes("0x55d398326f99059ff775485246999027b3197955", 12)
	receiver := mustAddrBytes("0xab5801a7d398351b8be11c439e05c5b3259aec9b", 13)
	minOut := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 70), big.NewInt(5))
	tx := rpcTx(t, 301234567, i64(1731800000), []string{testUser, defaultChainPoolProgram, testOther.String()}, nil, nil, []innerIx{
		{1, releasePayload()},
		// emitted by a program that is neither the chain pool, the receiver nor an mcs alias: ignored
		{2, crossOutPayload(crossOutInput{fromChain: 1, toChain: 2, mint: testUSDC, bridgeAmount: 1, toToken: toToken, receiver: receiver,
			minOut: big.NewInt(1), sourceToken: pk(testWSOL), sourceAmount: 1})},
		{1, crossOutPayload(crossOutInput{fromChain: 1360108768460801, toChain: 56, mint: testUSDC, bridgeAmount: 1234567, toToken: toToken,
			receiver: receiver, minOut: minOut, sourceToken: pk(testWSOL), sourceAmount: 999, referer: []uint16{9, 1201, 7}, fee: []uint16{30, 0}})},
	})

	if err := h.chain.handleTransaction("SIG_crossout_evm", tx); err != nil {
		t.Fatal(err)
	}
	rows := h.rowsFor(TopicCrossOut)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	assertJSON(t, "crossout_evm", r.LogData, goldenLogData["crossout_evm"])
	if r.ChainId != 1360108768460801 || r.EventId != 98 || r.ProjectId != 7 || r.TxHash != "SIG_crossout_evm" ||
		r.ContractAddress != defaultChainPoolProgram || r.BlockNumber != 301234567 || r.BlockHash != "" ||
		r.TxIndex != 1 || r.LogIndex != 1 || r.TxTimestamp != 1731800000 {
		t.Fatalf("row = %+v", r)
	}
	want := &stream.BridgeDataRequest{
		Entrance: "butter+", Affiliate: []string{"9:30", "1201:0"}, FromChainID: "1360108768460801", ToChainID: "56",
		Amount: "1.234567", TokenInAddress: testUSDC, TokenOutAddress: "0x55d398326f99059fF775485246999027B3197955",
		MinAmountOut: "1180591620717411303429", Receiver: "0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B", Caller: testUser, EntranceId: "9",
	}
	if len(h.reqs) != 1 || !reflect.DeepEqual(h.reqs[0], want) {
		t.Fatalf("butter request = %+v", h.reqs)
	}
}

func TestGoldenCrossOutBtc(t *testing.T) {
	h := newHarness(t, func(c *Config) {
		c.ButterEntrance = ""
		c.NativeTokenOutAddress = "0x0000000000000000000000000000000000425443"
	})
	h.butter = map[string]interface{}{"data": "0x", "relay": false}
	tx := rpcTx(t, 301234999, nil, []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{
		{1, crossOutPayload(crossOutInput{fromChain: 1360108768460801, toChain: btcChainId, mint: testWSOL, bridgeAmount: 2000000000,
			toToken: make([]byte, 32), receiver: cat(make([]byte, 12), []byte{0}, seq(20, 0xa0)), minOut: new(big.Int),
			sourceToken: make([]byte, 32), fee: []uint16{5}})},
	})

	if err := h.chain.handleTransaction("SIG_crossout_btc", tx); err != nil {
		t.Fatal(err)
	}
	rows := h.rowsFor(TopicCrossOut)
	if len(rows) != 1 || rows[0].TxTimestamp != 0 {
		t.Fatalf("rows = %+v", rows)
	}
	assertJSON(t, "crossout_btc", rows[0].LogData, goldenLogData["crossout_btc"])
	want := &stream.BridgeDataRequest{
		Affiliate: []string{}, FromChainID: "1360108768460801", ToChainID: "1360095883558913", Amount: "2.0",
		TokenInAddress: testWSOL, TokenOutAddress: "0x425443", MinAmountOut: "0",
		Receiver: "bc1q5zs69gay5kn2029f4246etdw47ctrv4nydx0t8", Caller: testUser, EntranceId: "9",
	}
	if len(h.reqs) != 1 || !reflect.DeepEqual(h.reqs[0], want) {
		t.Fatalf("butter request = %+v", h.reqs[0])
	}
}

func TestGoldenCrossInV0(t *testing.T) {
	h := newHarness(t, nil)
	tx := rpcTx(t, 301235000, i64(1731800400), []string{testUser, testOther.String()},
		[]string{defaultReceiverProgram}, []string{testUSDC, testWSOL}, []innerIx{
			// index 2 is the first loaded (lookup table) address: the receiver program
			{2, crossInPayload(56, testUSDC, 5000000, testWSOL, 123, 0, 77)},
			{3, refundPayload(1, testUSDC, testWSOL, 1)},
		})

	if err := h.chain.handleTransaction("SIG_crossin_v0", tx); err != nil {
		t.Fatal(err)
	}
	rows := h.rowsFor(TopicCrossIn)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	assertJSON(t, "crossin_v0", r.LogData, goldenLogData["crossin_v0"])
	if r.EventId != 124 || r.ContractAddress != defaultReceiverProgram || r.BlockNumber != 301235000 || r.TxTimestamp != 1731800400 {
		t.Fatalf("row = %+v", r)
	}
	if len(h.reqs) != 0 || h.rpc.infoCalls != 0 {
		t.Fatalf("crossIn must not call butter or getAccountInfo")
	}
}

func TestGoldenRefundAlias(t *testing.T) {
	h := newHarness(t, nil)
	tx := rpcTx(t, 301235001, i64(1731800401), []string{testUser, testAlias.String()}, []string{}, []string{}, []innerIx{
		{1, refundPayload(1, testUSDC, testWSOL, 42)},
		{1, cat(eventCpiPrefix, []byte{1, 2, 3})},
	})

	if err := h.chain.handleTransaction("SIG_refund_alias", tx); err != nil {
		t.Fatal(err)
	}
	rows := h.rowsFor(TopicCrossIn)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	assertJSON(t, "refund_alias", rows[0].LogData, goldenLogData["refund_alias"])
	if rows[0].EventId != 125 || rows[0].ContractAddress != testAlias.String() {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestBtcAddressMatchesBitcoinjs(t *testing.T) {
	for in, want := range goldenBtc {
		got, err := btcAddress(common.FromHex(in))
		if err != nil || got != want {
			t.Fatalf("btcAddress(%s) = %s, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "02" + "11", "0511", "0011", "0111"} {
		if _, err := btcAddress(common.FromHex(in)); err == nil {
			t.Fatalf("btcAddress(%s) want error", in)
		}
	}
}

func TestFormatUnitsMatchesEthers(t *testing.T) {
	for _, c := range []struct {
		v    uint64
		d    int
		want string
	}{{0, 6, "0.0"}, {1000000, 6, "1.0"}, {1500000, 6, "1.5"}, {5, 0, "5"}, {123456789, 9, "0.123456789"}, {10, 1, "1.0"}, {1, 18, "0.000000000000000001"}} {
		if got := formatUnits(new(big.Int).SetUint64(c.v), c.d); got != c.want {
			t.Fatalf("formatUnits(%d, %d) = %s, want %s", c.v, c.d, got, c.want)
		}
	}
}

func TestEventDiscriminatorsMatchIdl(t *testing.T) {
	for name, want := range map[string][]byte{
		"CrossOutEvent": {247, 103, 102, 176, 13, 54, 106, 23},
		"CrossInEvent":  {231, 51, 205, 59, 225, 218, 111, 180},
		"RefundEvent":   {176, 159, 218, 59, 94, 213, 129, 218},
	} {
		if got := eventDiscriminator(name); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s discriminator = %v, want %v", name, got, want)
		}
	}
}

func TestDecodeRejectsTruncatedEvents(t *testing.T) {
	full := crossInPayload(56, testUSDC, 1, testWSOL, 1, 1, 1)
	if decodeCrossIn(eventBody(full[:len(full)-1])) != nil {
		t.Fatal("truncated CrossInEvent decoded")
	}
	// trailing bytes are ignored like buffer-layout
	if decodeCrossIn(eventBody(append(full, 0xff))) == nil {
		t.Fatal("CrossInEvent with trailing bytes rejected")
	}
	if decodeCrossOut(eventBody(cat(eventCpiPrefix, discCrossOut, make([]byte, 40)))) != nil {
		t.Fatal("truncated CrossOutEvent decoded")
	}
	// a receiver event under the chain pool program is not a chain pool event
	p := newParser(nil)
	tx := rpcTx(t, 1, nil, []string{testUser, defaultChainPoolProgram}, nil, nil, []innerIx{{1, full}})
	outs, ins, err := p.parse(tx)
	if err != nil || len(outs) != 0 || len(ins) != 0 {
		t.Fatalf("parse = %v %v %v", outs, ins, err)
	}
}
