package constant

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"
)

const (
	Near     = "near"
	Ton      = "ton"
	Xrp      = "xrp"
	Solana   = "solana"
	Ethereum = "ethereum"
)

const (
	Redis = "redis"
	Mysql = "mysql"
)

const (
	LatestBlock = "latest"
)

var (
	RetryInterval = time.Second * 5
)

var (
	ConfigFileFlag = &cli.StringFlag{
		Name:  "config",
		Usage: "JSON configuration file",
	}
	KeyPathFlag = &cli.StringFlag{
		Name:  "keystorePath",
		Usage: "Path to keystore",
	}
	LatestFlag = &cli.BoolFlag{
		Name:  "latest",
		Usage: "use latest block height",
	}
	BackUpFlag = &cli.BoolFlag{
		Name:  "back",
		Usage: "is back up program",
	}
)

var (
	OnlineChaId = map[string]struct{}{}
)

const (
	KeyOfLatestBlock = "chain_%s_latest_block"
	KeyOfScanBlock   = "chain_%s_scan_block"
)

const (
	ReqInterval = int64(2)
)

var (
	ZeroAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")
)

const (
	TopicMessageOut = "0x2aaebc938a5ab70e98644b0c6a8472fe02b7edece7e6e6dc71959dc34e109dfc"
	TopicMessageIn  = "0x549a11bfdfd61bdd00339bd8eb4af92f372360742915f6e72f7fec6c8e702c39"
)
