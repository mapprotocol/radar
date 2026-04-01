package ethereum

import (
	"math/big"

	ethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
	"github.com/mapprotocol/filter/pkg/blockstore"
	"github.com/pkg/errors"
)

type Chain struct {
	conn                             Conner
	kp                               *ethkeystore.Key
	log                              log.Logger
	cfg                              *EthConfig
	stop, eventStop, dog             chan struct{}
	bs                               blockstore.BlockStorer
	storages                         []storage.Saver
	events                           []*dao.Event
	eventId, currentProgress, latest int64
	isBackUp                         bool
}

func New(cfg config.RawChainConfig, storages []storage.Saver, latest, isBackUp bool) (*Chain, error) {
	eCfg, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}

	conn := NewConn(eCfg.Endpoint, nil)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}
	bs, err := blockstore.New(blockstore.PathPostfix, eCfg.Id)
	if err != nil {
		return nil, err
	}

	if latest {
		latestBlock, err := conn.LatestBlock()
		if err != nil {
			return nil, err
		}
		eCfg.StartBlock = big.NewInt(int64(latestBlock))
	}

	ret := &Chain{
		conn:      conn,
		log:       log.New("chain", eCfg.Name),
		cfg:       eCfg,
		stop:      make(chan struct{}),
		eventStop: make(chan struct{}),
		dog:       make(chan struct{}),
		bs:        bs,
		storages:  storages,
		events:    make([]*dao.Event, 0),
		isBackUp:  isBackUp,
	}
	ret.log.SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler))

	return ret, nil
}

func (c *Chain) Start() error {
	err := c.getMatch(true)
	if err != nil {
		return errors.Wrap(err, "init getMatch failed")
	}
	go func() {
		err := c.sync()
		if err != nil {
			c.log.Error("Polling blocks failed", "err", err)
		}
	}()
	go func() {
		err := c.renewEvent()
		if err != nil {
			c.log.Error("Renew event failed", "err", err)
		}
	}()

	go func() {
		c.watchdog()
	}()
	c.log.Info("Starting filter ...")
	return nil
}

func (c *Chain) Stop() {
	close(c.stop)
	close(c.eventStop)
	close(c.dog)
}
