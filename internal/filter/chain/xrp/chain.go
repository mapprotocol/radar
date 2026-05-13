package xrp

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/observability"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
	"github.com/mapprotocol/filter/pkg/blockstore"
)

type Chain struct {
	log                              log.Logger
	cfg                              *Config
	stop, eventStop, dog             chan struct{}
	storages                         []storage.Saver
	events                           []*dao.Event
	bs                               blockstore.BlockStorer
	conn                             Conner
	eventId, currentProgress, latest int64
	state                            *observability.ChainState
}

func New(cfg config.RawChainConfig, storages []storage.Saver) (*Chain, error) {
	eCfg, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}

	bs, err := blockstore.New(blockstore.PathPostfix, eCfg.Id)
	if err != nil {
		return nil, err
	}

	conn := NewConn(eCfg.Endpoint)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}

	return &Chain{
		log:       log.New("chain", cfg.Name),
		stop:      make(chan struct{}),
		eventStop: make(chan struct{}),
		dog:       make(chan struct{}),
		cfg:       eCfg,
		bs:        bs,
		conn:      conn,
		storages:  storages,
		state:     observability.RegisterChain(eCfg.Name, "sync"),
	}, nil
}

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

	go func() {
		c.watchdog()
	}()
	return nil
}

func (c *Chain) Stop() {
	close(c.stop)
	close(c.eventStop)
	close(c.dog)
	return
}
