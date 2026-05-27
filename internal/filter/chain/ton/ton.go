package ton

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/observability"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
)

type Chain struct {
	log                  log.Logger
	cfg                  *Config
	stop, eventStop, dog chan struct{}
	storages             []storage.Saver
	events               []*dao.Event
	eventId              int64
	state                *observability.ChainState
}

func New(cfg config.RawChainConfig, storages []storage.Saver) (*Chain, error) {
	eCfg, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Chain{
		log:       log.New("chain", cfg.Name),
		stop:      make(chan struct{}),
		eventStop: make(chan struct{}),
		dog:       make(chan struct{}),
		cfg:       eCfg,
		storages:  storages,
	}, nil
}

func (c *Chain) SetState(s *observability.ChainState) { c.state = s }

func (c *Chain) Start() error {
	err := c.getMatch()
	if err != nil {
		return err
	}
	go func() {
		err = c.sync()
		if err != nil {
			c.log.Error("Polling blocks failed", "err", err)
		}
	}()
	return nil
}

func (c *Chain) Stop() {
	close(c.stop)
	close(c.eventStop)
	close(c.dog)
	return
}
