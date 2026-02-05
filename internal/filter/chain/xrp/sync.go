package xrp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/filter/internal/pkg/butter"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/memo"
	"github.com/mapprotocol/filter/internal/pkg/stream"
	"github.com/mapprotocol/filter/pkg/utils"

	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/pkg/errors"
)

func (c *Chain) sync() error {
	var currentBlock = c.cfg.StartBlock
	local, err := c.bs.TryLoadLatestBlock()
	if err != nil {
		return err
	}
	c.log.Info("Sync start", "config", currentBlock, "local", local)
	if local.Cmp(currentBlock) == 1 {
		currentBlock = local
	}
	savedBN := uint64(0)

	for {
		select {
		case <-c.stop:
			c.log.Info("Stop syncing blocks")
			return nil
		default:
			latestBlock, err := c.conn.LatestBlock()
			if err != nil {
				c.log.Error("Unable to get latest block", "block", currentBlock, "err", err)
				time.Sleep(constant.RetryInterval)
				continue
			}

			if latestBlock != savedBN {
				savedBN = latestBlock
				for _, s := range c.storages {
					err = s.LatestBlockNumber(c.cfg.Id, latestBlock)
					if err != nil {
						c.log.Error("Save latest height failed", "storage", s.Type(), "err", err)
					}
				}
			}

			if currentBlock.Uint64() == 0 || currentBlock.Uint64() > latestBlock {
				currentBlock = big.NewInt(0).SetUint64(latestBlock)
			}

			diff := currentBlock.Int64() - int64(latestBlock)
			if diff > 0 {
				c.log.Info("Chain online blockNumber less than local latestBlock, waiting...", "chainBlcNum", latestBlock,
					"localBlock", currentBlock.Uint64(), "diff", currentBlock.Int64()-int64(latestBlock))
				utils.Alarm(context.Background(), fmt.Sprintf("chain latestBlock less than local, please admin handler, chain=%s, latest=%d, local=%d",
					c.cfg.Name, latestBlock, currentBlock.Uint64()))
				time.Sleep(time.Second * time.Duration(diff))
				currentBlock = big.NewInt(0).SetUint64(latestBlock)
				continue
			}

			endBlock := big.NewInt(currentBlock.Int64())
			if c.cfg.Range != nil && c.cfg.Range.Int64() != 0 {
				endBlock = endBlock.Add(currentBlock, c.cfg.Range)
			}

			// xrp self logic
			err = c.mosHandler(currentBlock, endBlock)
			if err != nil {
				c.log.Error("Failed to get events for block", "block", currentBlock, "err", err)
				utils.Alarm(context.Background(), fmt.Sprintf("filter failed, chain=%s, err is %s", c.cfg.Name, err.Error()))
				time.Sleep(constant.RetryInterval)
				continue
			}

			err = c.bs.StoreBlock(endBlock)
			if err != nil {
				c.log.Error("Failed to write latest block to blockStore", "block", currentBlock, "err", err)
			}

			c.currentProgress = endBlock.Int64()
			c.latest = int64(latestBlock) // watchDog
			currentBlock = big.NewInt(0).Add(endBlock, big.NewInt(1))
			if latestBlock-currentBlock.Uint64() <= c.cfg.BlockConfirmations.Uint64() {
				time.Sleep(constant.RetryInterval)
			}
		}
	}
}

func (c *Chain) mosHandler(startBlock, endBlock *big.Int) error {
	for _, ele := range c.cfg.Mcs {
		payload := map[string]interface{}{
			"method": "account_tx",
			"params": []map[string]interface{}{{
				"account":          ele,
				"ledger_index_min": startBlock,
				"ledger_index_max": endBlock,
				"limit":            100,
			}},
		}
		body, _ := json.Marshal(payload)
		resp, err := c.conn.HttpClient().Post(c.cfg.Endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			return errors.Wrap(err, "http post error")
		}

		var result stream.LedgerAccountTx
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			return errors.Wrap(err, "error decoding response")
		}
		_ = resp.Body.Close()

		if len(result.Result.Transactions) == 0 {
			if endBlock.Int64()%100 == 0 {
				c.log.Info("No transaction found in block", "startBlock", startBlock, "endBlock", endBlock)
			}
			continue
		}

		for tIdx, t := range result.Result.Transactions {
			if t.Tx.TransactionType != "Payment" {
				c.log.Info("Ignore log, because txType not match", "address", ele, "blockNumber", t.Tx.LedgerIndex,
					"txHash", t.Tx.Hash, "txType", t.Tx.TransactionType)
				continue
			}
			if len(t.Tx.Memos) == 0 {
				c.log.Info("Ignore log, because memos is zero", "address", ele, "blockNumber", t.Tx.LedgerIndex, "txHash", t.Tx.Hash)
				continue
			}
			// if t.Tx.Destination != ele {
			// 	c.log.Info("Ignore log, because destination not match", "address", ele,
			// 		"blockNumber", t.Tx.LedgerIndex, "txHash", t.Tx.Hash, "destination", t.Tx.Destination)
			// 	continue
			// }
			idx := -1
			eventData := &MessageOutEvent{}
			for _, m := range t.Tx.Memos {
				idx, eventData, err = c.match(m.Memo.MemoData, t.Tx.Hash, t.Tx.LedgerIndex)
				if err != nil {
					return err
				}
				if idx != -1 {
					break
				}
			}
			if idx == -1 {
				c.log.Info("Ignore log, because topic not match", "address", ele, "blockNumber", t.Tx.LedgerIndex, "txHash", t.Tx.Hash)
				continue
			}

			//c.log.Info("Ignore log, because topic or address not match", "address", ele, "blockNumber", t.Tx.LedgerIndex)
			event := c.events[idx]
			tmp := t
			err = c.insert(&tmp, event, tIdx, eventData)
			if err != nil {
				c.log.Error("Insert failed", "hash", t.Tx.Hash, "logIndex", tIdx, "err", err)
				continue
			}
		}
	}

	return nil
}

func (c *Chain) insert(tx *stream.LedgerTx, event *dao.Event, tIdx int, eventData *MessageOutEvent) error {
	// var (
	// 	topic  string
	// 	cid, _ = strconv.ParseInt(c.cfg.Id, 10, 64)
	// )
	// topic = strings.Join([]string{event.Topic, eventData.OrderID}, ",")
	// eventBytes, _ := json.Marshal(eventData)

	// for _, s := range c.storages {
	// 	err := s.Mos(22776, &dao.Mos{
	// 		ChainId:         cid,
	// 		ProjectId:       event.ProjectId,
	// 		EventId:         event.Id,
	// 		TxHash:          tx.Tx.Hash,
	// 		ContractAddress: tx.Tx.Account,
	// 		Topic:           topic,
	// 		BlockNumber:     uint64(tx.Tx.LedgerIndex),
	// 		LogIndex:        uint(tIdx),
	// 		TxIndex:         1,
	// 		BlockHash:       "",
	// 		LogData:         string(eventBytes),
	// 		TxTimestamp:     uint64(time.Now().Unix()),
	// 	})
	// 	if err != nil {
	// 		c.log.Error("Insert failed", "hash", tx.Tx.Hash, "logIndex", tIdx, "err", err)
	// 		continue
	// 	}
	// 	c.log.Info("Insert success", "blockNumber", tx.Tx.LedgerIndex, "hash", tx.Tx.Hash, "logIndex", tIdx)
	// }
	return nil
}

var (
	messageOutAbiJson = `[{"inputs":[{"internalType":"bytes32","name":"orderId","type":"bytes32"},{"internalType":"uint256","name":"chainAndGasLimit","type":"uint256"},{"internalType":"bytes","name":"data","type":"bytes"}],"name":"messageOut","outputs":[{"internalType":"bytes","name":"addr","type":"bytes"}],"stateMutability":"pure","type":"function"}]`
	messageOutAbi, _  = abi.JSON(strings.NewReader(messageOutAbiJson))
)

// type MessageOutEvent struct {
// 	OrderId          common.Hash `json:"orderId"`
// 	ChainAndGasLimit *big.Int    `json:"chainAndGasLimit"`
// 	Data             []byte      `json:"data"`
// }

type MessageOutEvent struct {
	Id                int64  `json:"id"`
	Topic             string `json:"topic"`
	BlockNumber       int64  `json:"block_number"`
	TxHash            string `json:"tx_hash"`
	Addr              string `json:"addr"`
	OrderID           string `json:"order_id"`  // orderId
	From              string `json:"from"`      // relay
	To                string `json:"to"`        //
	SrcChain          string `json:"src_chain"` // fromChain
	SrcToken          string `json:"src_token"` // token
	Sender            string `json:"sender"`    // initiator
	InAmount          string `json:"in_amount"` // amount
	InTxHash          string `json:"in_tx_hash"`
	BridgeFee         string `json:"bridge_fee"`
	DstChain          string `json:"dst_chain"`      // toChain
	DstToken          string `json:"dst_token"`      //
	Receiver          string `json:"receiver"`       //
	MOS               string `json:"mos"`            // map mos address
	Relay             bool   `json:"relay"`          //   (from butter)
	MessageType       uint8  `json:"message_type"`   // default 3
	GasLimit          string `json:"gas_limit"`      // default 0
	MinOutAmount      string `json:"min_out_amount"` //  minOutAmount
	SwapData          string `json:"swap_data"`      // (from butter)
	Entrance          string `json:"entrance"`
	InAmountNoDecimal string `json:"in_amount_no_decimal"` // amount
}

func (c *Chain) match(memoData, hash string, blockNumber int) (int, *MessageOutEvent, error) {
	ret := -1
	for idx, v := range c.events {
		if strings.HasPrefix(memoData, strings.ToUpper(strings.TrimPrefix(v.Topic, "0x"))) {
			ret = idx
			break
		}
	}
	if ret == -1 {
		return -1, nil, nil
	}
	var err error
	var event *MessageOutEvent
	switch c.events[ret].Topic {
	case constant.TopicMessageOut:
		event, err = c.handlerMessageOut(ret, blockNumber, memoData, hash)
		if err != nil {
			return -1, nil, err
		}
		return ret, event, nil
	case constant.TopicMessageIn:
		event, err = c.handlerMessageIn(ret, memoData, hash)
		if err != nil {
			return -1, nil, err
		}
		return ret, event, nil
	}
	return -1, nil, nil
}

func (c *Chain) handlerMessageOut(idx, blockNumber int, memoData, hash string) (*MessageOutEvent, error) {
	rmPrefix := strings.TrimPrefix(memoData,
		strings.ToUpper(strings.TrimPrefix(c.events[idx].Topic, "0x")))
	hexBytes := common.Hex2Bytes(rmPrefix)

	crossOutMemo := &memo.CrossOutMemo{}
	err := crossOutMemo.FromString(string(hexBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse CrossOutMemo")
	}

	srcToken := "0x585250"
	resp, err := butter.RequestBridgeData(c.cfg.Butter, hash,
		&stream.BridgeDataRequest{
			Entrance:        crossOutMemo.Affiliate,
			Affiliate:       nil,
			FromChainID:     c.cfg.Id,
			ToChainID:       crossOutMemo.ToChain,
			Amount:          crossOutMemo.Amount,
			TokenInAddress:  srcToken,
			TokenOutAddress: crossOutMemo.DstToken,
			MinAmountOut:    crossOutMemo.MinOutAmount,
			Receiver:        crossOutMemo.Receiver,
			Caller:          crossOutMemo.Sender,
		})
	if err != nil {
		return nil, errors.Wrap(err, "butter request error")
	}

	amountBigFloat, ok := new(big.Float).SetString(crossOutMemo.Amount)
	if !ok {
		return nil, errors.New("invalid amount string" + crossOutMemo.Amount)
	}
	exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	amount := new(big.Float).Mul(amountBigFloat, new(big.Float).SetInt(exp))
	amountBigInt, _ := amount.Int(nil)

	event := MessageOutEvent{
		Topic:             c.events[idx].Topic,
		SrcToken:          srcToken,
		MOS:               "0x787270",
		To:                resp.Receiver,
		Relay:             resp.Relay,
		SwapData:          resp.Data,
		BlockNumber:       int64(blockNumber),
		TxHash:            hash,
		OrderID:           crossOutMemo.OrderId,
		From:              crossOutMemo.Sender,
		SrcChain:          c.cfg.Id,
		MessageType:       3,
		GasLimit:          "0",
		Entrance:          crossOutMemo.Affiliate,
		MinOutAmount:      crossOutMemo.MinOutAmount,
		Sender:            crossOutMemo.Sender,
		InAmount:          amountBigInt.String(),
		InTxHash:          hash,
		DstChain:          crossOutMemo.ToChain,
		DstToken:          crossOutMemo.DstToken,
		Receiver:          crossOutMemo.Receiver,
		InAmountNoDecimal: crossOutMemo.Amount,
	}

	return &event, nil
}

func (c *Chain) handlerMessageIn(idx int, memoData, hash string) (*MessageOutEvent, error) {
	rmPrefix := strings.TrimPrefix(memoData,
		strings.ToUpper(strings.TrimPrefix(c.events[idx].Topic, "0x")))
	strBytes := common.Hex2Bytes(rmPrefix)
	c.log.Info("handlerMessageIn", "hash", hash, "strBytes", string(strBytes))
	parts := strings.Split(string(strBytes), "|")
	event := MessageOutEvent{}
	event.Topic = c.events[idx].Topic
	event.SrcChain = parts[1]
	event.OrderID = parts[2]

	return &event, nil
}

func (c *Chain) getMatch() error {
	for _, s := range c.storages {
		if s.Type() != constant.Mysql {
			continue
		}
		events, err := s.GetEvent(c.eventId)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s get events failed", s.Type()))
		}
		for _, e := range events {
			tmp := e
			c.eventId = tmp.Id
			if tmp.ChainId != "" && tmp.ChainId != c.cfg.Id {
				continue
			}
			c.events = append(c.events, tmp)
			c.log.Info("Add new event", "project", e.ProjectId, "topic", e.Topic)
		}
	}
	return nil
}
