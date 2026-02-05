package storage

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
)

var (
	KeyOfMapMessenger   = "messenger_%d_%d" // messenger_sourceChainId_toChainId
	KeyOfOtherMessenger = "messenger_%d"    // messenger_sourceChainId_toChainId
)

type Redis struct {
	dsn         string
	redisClient *redis.Client
}

func NewRds(dsn string) (*Redis, error) {
	m := &Redis{dsn: dsn}
	err := m.init()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *Redis) init() error {
	opt, err := redis.ParseURL(r.dsn)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(opt)
	if err != nil {
		return err
	}
	r.redisClient = rdb
	return nil
}

func (r *Redis) Type() string {
	return constant.Redis
}

func (r *Redis) Mos(toChainId uint64, event []*dao.Mos) error {
	// var key string
	// if event.ChainId == 22776 || event.ChainId == 212 || event.ChainId == 213 {
	// 	if _, ok := constant.OnlineChaId[strconv.FormatUint(toChainId, 10)]; !ok {
	// 		log.Info("Found a map log that is not the current task", "hash", event.TxHash, "toChainId", toChainId)
	// 		return nil
	// 	}
	// 	key = fmt.Sprintf(KeyOfMapMessenger, event.ChainId, toChainId)
	// } else {
	// 	key = fmt.Sprintf(KeyOfOtherMessenger, event.ChainId)
	// }
	// data, _ := json.Marshal(event)
	// _, err := r.redisClient.RPush(context.Background(), key, data).Result()
	// if err != nil {
	// 	return err
	// }
	return nil
}

func (r *Redis) LatestBlockNumber(chainId string, latest uint64) error {
	err := r.redisClient.Set(context.Background(), fmt.Sprintf(constant.KeyOfLatestBlock, chainId), latest, 0).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *Redis) ScanBlockNumber(chainId string, latest uint64) error {
	err := r.redisClient.Set(context.Background(), fmt.Sprintf(constant.KeyOfScanBlock, chainId), latest, 0).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *Redis) GetEvent(int64) ([]*dao.Event, error) {
	return nil, nil
}
