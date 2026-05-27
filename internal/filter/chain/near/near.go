package near

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/go-redis/redis/v8"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/observability"
	"github.com/mapprotocol/filter/internal/pkg/storage"
	"github.com/mapprotocol/filter/pkg/blockstore"
)

type Chain struct {
	log      log.Logger
	cfg      *Config
	stop     chan struct{}
	rdb      *redis.Client
	bs       blockstore.BlockStorer
	storages []storage.Saver
	state    *observability.ChainState
}

func New(cfg config.RawChainConfig, storages []storage.Saver) (*Chain, error) {
	eCfg, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}

	ret := &Chain{
		log:      log.New("chain", eCfg.Name),
		cfg:      eCfg,
		stop:     make(chan struct{}),
		storages: storages,
	}
	ret.log.SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler))
	opt, err := redis.ParseURL(cfg.Opts.Redis)
	if err != nil {
		panic(err)
	}
	ret.rdb = redis.NewClient(opt)

	return ret, nil
}

func (c *Chain) SetState(s *observability.ChainState) { c.state = s }

func (c *Chain) Start() error {
	go func() {
		err := c.sync()
		if err != nil {
			c.log.Error("Polling blocks failed", "err", err)
		}
	}()
	c.log.Info("Starting filter ...")
	return nil
}

func (c *Chain) Stop() {
	close(c.stop)
}
