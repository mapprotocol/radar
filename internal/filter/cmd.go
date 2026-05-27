package filter

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/filter/chain"
	"github.com/mapprotocol/filter/internal/filter/config"
	"github.com/mapprotocol/filter/internal/filter/core"
	"github.com/mapprotocol/filter/internal/observability"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/storage"
	"github.com/mapprotocol/filter/pkg/utils"
	"github.com/urfave/cli/v2"
)

var Command = &cli.Command{
	Name:  "cli",
	Flags: []cli.Flag{constant.ConfigFileFlag, constant.KeyPathFlag, constant.LatestFlag, constant.BackUpFlag},
	Action: func(cli *cli.Context) error {
		log.Root().SetHandler(log.StdoutHandler)
		cfg, err := config.Local(cli.String(constant.ConfigFileFlag.Name))
		if err != nil {
			return err
		}

		if cfg.KeystorePath == "" {
			cfg.KeystorePath = cli.String(constant.KeyPathFlag.Name)
		}

		storages := make([]storage.Saver, 0, len(cfg.Storages))
		for _, s := range cfg.Storages {
			ele, err := storage.NewSaver(s.Type, s.Url)
			if err != nil {
				return err
			}
			storages = append(storages, ele)
		}

		latest := cli.Bool(constant.LatestFlag.Name)
		isBackUp := cli.Bool(constant.BackUpFlag.Name)
		utils.Init(cfg.Other.Env, cfg.Other.MonitorUrl)

		// Stand up observability (metrics + /status + pprof + alarms) before
		// any chain goroutines start so the first tick can already publish.
		obsAddr := ":9101"
		if cfg.Other.ObservabilityAddr != "" {
			obsAddr = cfg.Other.ObservabilityAddr
		}
		obs := observability.New("radar", observability.Config{
			Addr:    obsAddr,
			AlarmFn: utils.Alarm,
		})
		observability.SetDefault(obs)
		obs.StartHTTP()
		obs.StartBlockLagAlarms(observability.DefaultBlockLagRule())
		log.Info("Observability HTTP serving", "addr", obsAddr,
			"endpoints", "/metrics /status /healthz /debug/pprof/")
		defer obs.Stop()

		chains, err := chain.Init(cfg, storages, latest, isBackUp)
		if err != nil {
			return err
		}
		// Register each successfully-constructed chain with observability and
		// inject the state back. chain.Init returns chains in cfg.Chains order
		// so the indices line up. Each Chain type implements a SetState method.
		for i, ch := range chains {
			state := observability.RegisterChain(cfg.Chains[i].Name, "sync")
			if setter, ok := ch.(interface {
				SetState(*observability.ChainState)
			}); ok {
				setter.SetState(state)
			}
		}
		sysErr := make(chan error)
		c := core.New(sysErr)
		for _, ch := range chains {
			c.AddChain(ch)
		}
		c.Start()
		return nil
	},
}
