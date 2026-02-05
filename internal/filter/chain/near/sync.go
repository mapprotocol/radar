package near

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	rds "github.com/go-redis/redis/v8"
	"github.com/mapprotocol/filter/internal/pkg/constant"
)

var (
	ListKey = "near_messsage_log"
)

func (c *Chain) sync() error {
	cid, _ := strconv.ParseInt(c.cfg.Id, 10, 64)
	for {
		select {
		case <-c.stop:
			return errors.New("polling terminated")
		default:
			ctx := context.Background()
			cmd := c.rdb.RPop(ctx, ListKey)
			result, err := cmd.Result()
			if err != nil && !errors.Is(err, rds.Nil) {
				c.log.Error("Unable to get latest block", "err", err)
				time.Sleep(constant.RetryInterval)
				continue
			}
			if result == "" {
				time.Sleep(constant.RetryInterval)
				continue
			}

			data := StreamerMessage{}
			err = json.Unmarshal([]byte(result), &data)
			if err != nil {
				c.log.Error("json marshal failed", "err", err, "data", result)
				time.Sleep(constant.RetryInterval)
				continue
			}
			idx := 0
			for _, shard := range data.Shards {
				for _, outcome := range shard.ReceiptExecutionOutcomes {
					idx++
					if c.Idx(outcome.ExecutionOutcome.Outcome.ExecutorID) == -1 {
						continue
					}
					if len(outcome.ExecutionOutcome.Outcome.Logs) == 0 {
						continue
					}
					match := false
					for _, ls := range outcome.ExecutionOutcome.Outcome.Logs {
						match = c.match(ls)
						if match {
							break
						}
					}
					if !match {
						c.log.Info("Mos Not Match", "log", outcome.ExecutionOutcome.Outcome.Logs)
						continue
					}

					txHash, err := c.rdb.Get(context.Background(), outcome.ExecutionOutcome.ID.String()).Result()
					if err != nil {
						c.log.Error("Get TxHah Failed", "receiptId", outcome.ExecutionOutcome.ID.String(), "txHash", txHash, "cid", cid, "err", err)
						continue
					}
					// sData, _ := json.Marshal(outcome)
					// for _, s := range c.storages {
					// 	err = s.Mos(0, &dao.Mos{
					// 		ChainId:         cid,
					// 		TxHash:          txHash,
					// 		ContractAddress: outcome.ExecutionOutcome.Outcome.ExecutorID,
					// 		Topic:           "",
					// 		BlockNumber:     data.Block.Header.Height,
					// 		LogIndex:        uint(idx),
					// 		LogData:         string(sData),
					// 		TxTimestamp:     data.Block.Header.Timestamp,
					// 	})
					// 	if err != nil {
					// 		c.log.Error("insert failed", "blockNumber", data.Block.Header.Height, "hash", txHash, "logIndex", idx, "err", err)
					// 		continue
					// 	}
					// 	c.log.Info("insert success", "blockNumber", data.Block.Header.Height, "hash", txHash, "logIndex", idx)
					// }
				}
			}
		}
	}
}

func (c *Chain) Idx(contract string) int {
	ret := -1
	for idx, addr := range c.cfg.Mcs {
		if addr == contract {
			ret = idx
			break
		}
	}

	return ret
}

func (c *Chain) match(log string) bool {
	for _, e := range c.cfg.Events {
		if strings.HasPrefix(log, e) {
			return true
		}
	}

	return false
}
