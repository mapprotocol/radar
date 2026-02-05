package ethereum

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/pkg/errors"

	"github.com/mapprotocol/filter/pkg/utils"
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
			return errors.New("polling terminated")
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

			if latestBlock-currentBlock.Uint64() < c.cfg.BlockConfirmations.Uint64() {
				c.log.Debug("Block not ready, will retry", "currentBlock", currentBlock, "latest", latestBlock)
				time.Sleep(constant.RetryInterval)
				continue
			}
			endBlock := big.NewInt(currentBlock.Int64())
			if c.cfg.Range != nil && c.cfg.Range.Int64() != 0 {
				endBlock = endBlock.Add(currentBlock, c.cfg.Range)
			}
			err = c.mosHandler(currentBlock, endBlock)
			if err != nil && !errors.Is(err, types.ErrInvalidSig) {
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
			c.latest = int64(latestBlock)
			currentBlock = big.NewInt(0).Add(endBlock, big.NewInt(1))
			if latestBlock-currentBlock.Uint64() <= c.cfg.BlockConfirmations.Uint64() {
				time.Sleep(constant.RetryInterval)
			}
		}
	}
}

type oldStruct struct {
	*dao.Event
	End int64 `json:"end"`
}

func (c *Chain) rangeScan(event *dao.Event, end int64) {
	start, ok := big.NewInt(0).SetString(event.BlockNumber, 10)
	if !ok {
		return
	}
	if start.Int64() >= end {
		c.log.Info("Find a event of appoint blockNumber, but block gather than current block",
			"appoint", event.BlockNumber, "current", end)
		return
	}
	c.log.Info("Find a event of appoint blockNumber, begin start scan", "appoint", event.BlockNumber, "current", end)
	// todo store redis
	sold := &oldStruct{
		Event: event,
		End:   end,
	}
	data, _ := json.Marshal(sold)
	filename := fmt.Sprintf("%s-%s-old.json", event.ChainId, event.Topic)
	err := c.bs.CustomStore(filename, data)
	if err != nil {
		c.log.Error("Find a event of appoint blockNumber, but store local filed", "format", event.Format, "topic", event.Topic)
		return
	}
	topics := make([]common.Hash, 0, 1)
	topics = append(topics, common.HexToHash(event.Topic))
	for i := start.Int64(); i < end; i += 20 {
		// querying for logs
		logs, err := c.conn.Client().FilterLogs(context.Background(), ethereum.FilterQuery{
			FromBlock: big.NewInt(i),
			ToBlock:   big.NewInt(i + 20),
			Addresses: []common.Address{common.HexToAddress(event.Address)},
			Topics:    [][]common.Hash{topics},
		})
		if err != nil {
			continue
		}
		if len(logs) == 0 {
			continue
		}
		// for _, l := range logs {
		// ele := l
		// err = c.insert(&ele, event)
		// if err != nil {
		// 	c.log.Error("RangeScan insert failed", "hash", l.TxHash, "logIndex", l.Index, "err", err)
		// 	continue
		// }
		// }
		time.Sleep(time.Millisecond * 3)
	}
	c.log.Info("Range scan finish", "appoint", event.BlockNumber, "current", end)
	_ = c.bs.DelFile(filename)
}

type eleStruct struct {
	ll    types.Log
	event *dao.Event
}

func (c *Chain) mosHandler(latestBlock, endBlock *big.Int) error {
	query := c.BuildQuery(latestBlock, endBlock)
	logs, err := c.conn.Client().FilterLogs(context.Background(), query)
	if err != nil {
		return fmt.Errorf("unable to Filter Logs: %w", err)
	}
	if len(logs) == 0 {
		return nil
	}

	inserts := make([]*eleStruct, 0)
	for _, l := range logs {
		ele := l
		idxs := c.match(&ele)
		if len(idxs) == 0 {
			c.log.Debug("Ignore log, because topic or address not match", "blockNumber", l.BlockNumber, "logTopic", l.Topics, "address", l.Address)
			continue
		}
		for _, idx := range idxs {
			event := c.events[idx]
			tmpEvent := event
			inserts = append(inserts, &eleStruct{
				ll:    ele,
				event: tmpEvent,
			})

		}
	}
	if len(inserts) > 0 {
		err = c.insert(inserts)
		if err != nil {
			c.log.Error("Insert failed", "insert", inserts, "err", err)
			return err
		}
	}
	if c.isBackUp {
		return nil
	}

	return nil
}

func (c *Chain) insert(inserts []*eleStruct) error {
	var (
		topic     string
		toChainId uint64
		cid, _    = strconv.ParseInt(c.cfg.Id, 10, 64)
	)
	header, err := c.conn.Client().HeaderByNumber(context.Background(),
		big.NewInt(0).SetUint64(inserts[0].ll.BlockNumber))
	if err != nil {
		c.log.Error("Get header by block number failed", "blockNumber", inserts[0].ll.BlockNumber, "err", err)
		header = &types.Header{
			Time: uint64(time.Now().Unix() - c.cfg.BlockConfirmations.Int64()),
		}
	}
	moss := make([]*dao.Mos, 0, len(inserts))
	for _, ele := range inserts {
		for idx, t := range ele.ll.Topics {
			topic += t.Hex()
			if idx != len(ele.ll.Topics)-1 {
				topic += ","
			}
			if idx == len(ele.ll.Topics)-1 {
				tmp, ok := big.NewInt(0).SetString(strings.TrimPrefix(t.Hex(), "0x"), 16)
				if ok {
					toChainId = tmp.Uint64()
				}
			}
		}
		moss = append(moss, &dao.Mos{
			ChainId:         cid,
			ProjectId:       ele.event.ProjectId,
			EventId:         ele.event.Id,
			TxHash:          ele.ll.TxHash.String(),
			ContractAddress: ele.ll.Address.String(),
			Topic:           topic,
			BlockNumber:     ele.ll.BlockNumber,
			LogIndex:        ele.ll.Index,
			TxIndex:         ele.ll.TxIndex,
			BlockHash:       ele.ll.BlockHash.Hex(),
			LogData:         common.Bytes2Hex(ele.ll.Data),
			TxTimestamp:     header.Time,
		})
	}

	for _, s := range c.storages {
		err = s.Mos(toChainId, moss)
		if err != nil {
			c.log.Error("Insert failed", "mos", moss, "err", err)
			continue
		}
		c.log.Info("Insert success", "mos", moss)
	}
	return nil
}

func (c *Chain) BuildQuery(startBlock *big.Int, endBlock *big.Int) ethereum.FilterQuery {
	query := ethereum.FilterQuery{
		FromBlock: startBlock,
		ToBlock:   endBlock,
	}
	return query
}

func (c *Chain) match(l *types.Log) []int {
	ret := make([]int, 0)
	for idx, d := range c.events {
		if !strings.EqualFold(l.Address.String(), d.Address) {
			continue
		}
		if l.Topics[0].Hex() != d.Topic {
			continue
		}
		ret = append(ret, idx)
	}
	return ret
}
