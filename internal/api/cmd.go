package api

import (
	"net/http"

	"github.com/ethereum/go-ethereum/log"
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/config"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/urfave/cli/v2"
)

var Command = &cli.Command{
	Name:  "api",
	Flags: []cli.Flag{constant.ConfigFileFlag},
	Action: func(cli *cli.Context) error {
		log.Root().SetHandler(log.StdoutHandler)
		cfg, err := config.Local(cli.String(constant.ConfigFileFlag.Name))
		if err != nil {
			return err
		}

		g := gin.Default()
		initMiddleware(g)
		err = initController(g, cfg)
		if err != nil {
			log.Error("init failed", "err", err)
			return err
		}

		httpsrv := &http.Server{Addr: cfg.Listen, Handler: g}
		if err := httpsrv.ListenAndServe(); err != nil {
			return err
		}
		return nil
	},
}
