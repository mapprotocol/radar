package solana

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"

	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/stream"
	"github.com/mapprotocol/filter/pkg/utils"
)

const (
	TopicCrossOut = "crossOut"
	TopicCrossIn  = "crossIn"

	btcChainId        = 1360095883558913
	btcTokenOutPadded = "0x0000000000000000000000000000000000425443"
	btcTokenOut       = "0x425443"
	defaultEntranceId = "9"
)

func (c *Chain) handleTransaction(txHash string, tx *rpc.GetTransactionResult) error {
	if tx.Meta != nil && tx.Meta.Err != nil {
		c.log.Info("Skip failed solana tx", "hash", txHash, "err", tx.Meta.Err)
		return nil
	}
	outs, ins, err := c.parser.parse(tx)
	if err != nil {
		return errors.Wrap(err, "parse transaction")
	}
	for _, ev := range outs {
		if err = c.crossOut(ev, txHash, tx); err != nil {
			return err
		}
	}
	for _, ev := range ins {
		if err = c.crossIn(ev, txHash, tx); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) crossOut(parsed parsedCrossOut, txHash string, tx *rpc.GetTransactionResult) error {
	ev := parsed.event
	mint := ev.BridgeMint.String()
	if _, ok := c.cfg.SupportedBridgeMints[mint]; !ok {
		msg := fmt.Sprintf("Solana unsupported CrossOut bridgeMint %s txHash %s", mint, txHash)
		c.log.Warn(msg)
		utils.Alarm(context.Background(), msg)
		return nil
	}
	events := c.matchEvents(TopicCrossOut, parsed.program)
	if len(events) == 0 {
		c.log.Info("Ignore crossOut, because no event registered", "program", parsed.program, "hash", txHash)
		return nil
	}
	c.log.Info("Find CrossOut emit_cpi tx", "hash", txHash, "slot", tx.Slot, "orderId", hex.EncodeToString(ev.OrderId[:]))

	logData, err := c.crossOutData(ev, txHash)
	if err != nil {
		return err
	}
	return c.insert(TopicCrossOut, parsed.program, txHash, tx, logData, events, ev.ToChain)
}

// crossOutData builds log_data exactly as compass-ts SolEventHandler.crossOut does.
func (c *Chain) crossOutData(ev *CrossOutEvent, txHash string) (string, error) {
	data := newOrderedMap()
	data.Set("orderId", hex.EncodeToString(ev.OrderId[:]))
	data.Set("user", ev.User.String())
	data.Set("from", byteArray(ev.User[:]))
	data.Set("fromChain", u64Hex(ev.FromChain))
	data.Set("fromChainId", u64Hex(ev.FromChain))
	data.Set("toChain", u64Hex(ev.ToChain))
	data.Set("bridgeMint", ev.BridgeMint.String())
	data.Set("bridgeAmount", strconv.FormatUint(ev.BridgeAmount, 10))
	data.Set("tokenAmount", u64Hex(ev.SourceAmount))
	data.Set("toToken", byteArray(ev.ToToken[:]))
	data.Set("receiver", byteArray(ev.Receiver[:]))
	data.Set("minAmountOut", numberHex(ev.MinAmountOut))
	data.Set("swapTokenOutMinAmountOut", numberHex(ev.MinAmountOut))
	data.Set("fromToken", byteArray(ev.SourceToken[:]))
	data.Set("amountOut", u64Hex(ev.SourceAmount))
	data.Set("refererId", u16Array(ev.RefererId))
	data.Set("feeRatio", u16Array(ev.FeeRatio))
	data.Set("originReceiver", byteArray(ev.Receiver[:]))

	toToken := common.BytesToAddress(ev.ToToken[12:]).Hex()
	if isZeroBytes(ev.ToToken[:]) {
		toToken = common.HexToAddress(c.cfg.NativeTokenOutAddress).Hex()
	}
	if toToken == btcTokenOutPadded {
		toToken = btcTokenOut
	}

	receiver := common.BytesToAddress(ev.Receiver[13:]).Hex()
	if ev.ToChain == btcChainId {
		btcBytes := ev.Receiver[:]
		if isZeroBytes(btcBytes[:12]) {
			btcBytes = btcBytes[12:]
		}
		addr, err := btcAddress(btcBytes)
		if err != nil {
			return "", errors.Wrap(err, "convert btc receiver")
		}
		receiver = addr
	}
	data.Set("receiver", receiver)

	decimals, err := c.mintDecimals(ev.BridgeMint)
	if err != nil {
		return "", err
	}
	entranceId := defaultEntranceId
	if len(ev.RefererId) > 0 {
		entranceId = strconv.Itoa(int(ev.RefererId[0]))
	}
	bridge, err := c.bridgeData(&stream.BridgeDataRequest{
		Entrance:        c.cfg.ButterEntrance,
		Affiliate:       mergeAffiliate(ev.RefererId, ev.FeeRatio),
		FromChainID:     strconv.FormatUint(ev.FromChain, 10),
		ToChainID:       strconv.FormatUint(ev.ToChain, 10),
		Amount:          formatUnits(new(big.Int).SetUint64(ev.BridgeAmount), decimals),
		TokenInAddress:  ev.BridgeMint.String(),
		TokenOutAddress: toToken,
		MinAmountOut:    ev.MinAmountOut.String(),
		Receiver:        receiver,
		Caller:          ev.User.String(),
		EntranceId:      entranceId,
	})
	if err != nil {
		utils.Alarm(context.Background(), fmt.Sprintf("Sol2Evm %s filter failed %s", txHash, err))
		return "", errors.Wrap(err, "butter request")
	}
	data.Set("relay", jsField(bridge, "relay"))
	data.Set("swapData", jsField(bridge, "data"))
	data.Set("receiver", jsField(bridge, "receiver"))
	return data.String()
}

func jsField(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok {
		return undefined{}
	}
	return v
}

func mergeAffiliate(ids, ratios []uint16) []string {
	n := len(ids)
	if len(ratios) < n {
		n = len(ratios)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("%d:%d", ids[i], ratios[i]))
	}
	return out
}

func (c *Chain) mintDecimals(mint solana.PublicKey) (int, error) {
	if d, ok := c.decimals[mint]; ok {
		return d, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	start := time.Now()
	info, err := c.rpc.GetAccountInfoWithOpts(ctx, mint, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentFinalized})
	c.state.ObserveRPC("getAccountInfo", time.Since(start).Seconds())
	if err != nil {
		return 0, errors.Wrap(err, "get bridge mint account")
	}
	if info == nil || info.Value == nil || info.Value.Data == nil {
		return 0, errors.New("Bridge Token mint account not found")
	}
	raw := info.Value.Data.GetBinary()
	if len(raw) <= 44 {
		return 0, fmt.Errorf("bridge mint account data too short: %d", len(raw))
	}
	c.decimals[mint] = int(raw[44])
	return int(raw[44]), nil
}

func (c *Chain) crossIn(parsed parsedCrossIn, txHash string, tx *rpc.GetTransactionResult) error {
	events := c.matchEvents(TopicCrossIn, parsed.program)
	if len(events) == 0 {
		c.log.Info("Ignore crossIn, because no event registered", "program", parsed.program, "hash", txHash)
		return nil
	}
	ev := parsed.event
	c.log.Info("Find CrossIn emit_cpi tx", "name", ev.Name, "hash", txHash, "slot", tx.Slot, "orderId", hex.EncodeToString(ev.OrderId[:]))
	logData, err := crossInData(ev)
	if err != nil {
		return err
	}
	return c.insert(TopicCrossIn, parsed.program, txHash, tx, logData, events, 0)
}

// crossInData builds log_data exactly as compass-ts SolEventHandler.crossIn does.
func crossInData(ev *CrossInEvent) (string, error) {
	data := newOrderedMap()
	data.Set("orderId", hex.EncodeToString(ev.OrderId[:]))
	data.Set("user", ev.User.String())
	data.Set("from", byteArray(ev.User[:]))
	data.Set("fromToken", byteArray(make([]byte, 32)))
	if ev.FromChain != nil {
		data.Set("fromChainId", u64Hex(*ev.FromChain))
	} else {
		data.Set("fromChainId", "00")
	}
	data.Set("bridgeMint", ev.BridgeMint.String())
	if ev.BridgeAmount != nil {
		data.Set("bridgeAmount", u64Hex(*ev.BridgeAmount))
		data.Set("tokenAmount", u64Hex(*ev.BridgeAmount))
	} else {
		data.Set("tokenAmount", "00")
	}
	if ev.DestToken != nil {
		data.Set("destToken", ev.DestToken.String())
		data.Set("toToken", byteArray(ev.DestToken[:]))
	} else {
		data.Set("toToken", byteArray(ev.BridgeMint[:]))
	}
	if ev.Amount != nil {
		data.Set("amount", u64Hex(*ev.Amount))
	}
	if ev.AmountOut != nil {
		data.Set("amountOut", u64Hex(*ev.AmountOut))
	}
	if ev.SignedMinAmountOut != nil {
		data.Set("signedMinAmountOut", u64Hex(*ev.SignedMinAmountOut))
		data.Set("minAmountOut", u64Hex(*ev.SignedMinAmountOut))
		data.Set("swapTokenOutMinAmountOut", u64Hex(*ev.SignedMinAmountOut))
	} else {
		data.Set("minAmountOut", "00")
		data.Set("swapTokenOutMinAmountOut", "00")
	}
	if ev.EffectiveMinAmountOut != nil {
		data.Set("effectiveMinAmountOut", u64Hex(*ev.EffectiveMinAmountOut))
	}
	data.Set("refererId", nil)
	data.Set("feeRatio", nil)
	data.Set("receiver", ev.User.String())
	if ev.Name == "RefundEvent" {
		data.Set("isrefund", true)
	}
	return data.String()
}

func (c *Chain) matchEvents(topic string, program solana.PublicKey) []*dao.Event {
	ret := make([]*dao.Event, 0)
	for _, e := range c.events {
		if e.Topic == topic && e.Address == program.String() {
			ret = append(ret, e)
		}
	}
	return ret
}

func (c *Chain) insert(topic string, program solana.PublicKey, txHash string, tx *rpc.GetTransactionResult,
	logData string, events []*dao.Event, toChainId uint64) error {
	cid, _ := strconv.ParseInt(c.cfg.Id, 10, 64)
	var ts uint64
	if tx.BlockTime != nil && *tx.BlockTime > 0 {
		ts = uint64(*tx.BlockTime)
	}
	c.state.IncEventsMatched(len(events))
	for _, e := range events {
		// one row per insert so a duplicate (already inserted) row is skipped on its own
		for _, s := range c.storages {
			m := &dao.Mos{
				ChainId:         cid,
				ProjectId:       e.ProjectId,
				EventId:         e.Id,
				TxHash:          txHash,
				ContractAddress: program.String(),
				Topic:           topic,
				BlockNumber:     tx.Slot,
				BlockHash:       "",
				TxIndex:         1,
				LogIndex:        1,
				LogData:         logData,
				TxTimestamp:     ts,
			}
			dbStart := time.Now()
			err := s.Mos(toChainId, []*dao.Mos{m})
			c.state.ObserveDBInsert(s.Type(), time.Since(dbStart).Seconds())
			if err != nil {
				c.state.RecordError("db_mos", err.Error())
				return errors.Wrapf(err, "insert %s failed, hash %s", topic, txHash)
			}
			c.log.Info("Insert success", "topic", topic, "hash", txHash, "project", e.ProjectId, "event", e.Id)
		}
	}
	return nil
}
