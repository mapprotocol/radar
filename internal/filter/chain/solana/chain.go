package solana

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/pkg/errors"

	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/observability"
	"github.com/mapprotocol/filter/internal/pkg/butter"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
	"github.com/mapprotocol/filter/internal/pkg/stream"
	"github.com/mapprotocol/filter/pkg/blockstore"
)

// rpcClient is the subset of *rpc.Client the filter uses.
type rpcClient interface {
	GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
	GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey,
		opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
	GetTransaction(ctx context.Context, sig solana.Signature, opts *rpc.GetTransactionOpts) (*rpc.GetTransactionResult, error)
	GetAccountInfoWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error)
}

type Chain struct {
	log        log.Logger
	cfg        *Config
	stop       chan struct{}
	storages   []storage.Saver
	events     []*dao.Event
	eventId    int64
	bs         blockstore.BlockStorer
	rpc        rpcClient
	parser     *parser
	bridgeData func(*stream.BridgeDataRequest) (map[string]interface{}, error)
	sleep      func(time.Duration)
	decimals   map[solana.PublicKey]int
	state      *observability.ChainState
	isBackUp   bool

	cursors        map[solana.PublicKey]string
	progress       map[solana.PublicKey]uint64 // slot up to which every signature of the program is handled
	savedScan      uint64
	lastLatestSave time.Time
	savedLatest    uint64
}

func New(cfg config.RawChainConfig, storages []storage.Saver, isBackUp bool) (*Chain, error) {
	sCfg, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}
	bs, err := blockstore.New(blockstore.PathPostfix, sCfg.Id)
	if err != nil {
		return nil, err
	}
	httpClient := rpclog.NewHTTPClient(time.Minute, http.DefaultTransport)
	client := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(sCfg.Endpoint, &jsonrpc.RPCClientOpts{HTTPClient: httpClient}))

	return newChain(sCfg, storages, bs, client, isBackUp), nil
}

func newChain(cfg *Config, storages []storage.Saver, bs blockstore.BlockStorer, client rpcClient, isBackUp bool) *Chain {
	return &Chain{
		log:      log.New("chain", cfg.Name),
		cfg:      cfg,
		stop:     make(chan struct{}),
		storages: storages,
		bs:       bs,
		rpc:      client,
		parser:   newParser(cfg.Mcs),
		bridgeData: func(req *stream.BridgeDataRequest) (map[string]interface{}, error) {
			return butter.RequestBridgeDataWithKey(cfg.Butter, cfg.ButterApiKey, req)
		},
		sleep:    time.Sleep,
		decimals: make(map[solana.PublicKey]int),
		isBackUp: isBackUp,
		cursors:  make(map[solana.PublicKey]string),
		progress: make(map[solana.PublicKey]uint64),
	}
}

func (c *Chain) SetState(s *observability.ChainState) { c.state = s }

func (c *Chain) Start() error {
	if err := c.getMatch(); err != nil {
		return err
	}
	c.loadCursors()
	go func() {
		if err := c.sync(); err != nil {
			c.log.Error("Polling solana signatures failed", "err", err)
		}
	}()
	return nil
}

func (c *Chain) Stop() {
	close(c.stop)
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
			c.log.Info("Add new event", "project", e.ProjectId, "topic", e.Topic, "addr", e.Address, "eventId", e.Id)
		}
	}
	return nil
}
