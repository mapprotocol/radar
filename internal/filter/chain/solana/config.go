package solana

import (
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"

	"github.com/mapprotocol/filter/internal/filter/config"
)

var defaultSupportedBridgeMints = []string{
	"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
	"4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU",
	"So11111111111111111111111111111111111111112",
}

const defaultNativeTokenOutAddress = "0x0000000000000000000000000000000000000000"

type Config struct {
	Name                  string
	Id                    string
	Endpoint              string
	Mcs                   []solana.PublicKey
	StartBlock            map[string]string // program id -> signature cursor used when nothing is stored locally
	Butter                string
	ButterApiKey          string
	ButterEntrance        string
	SupportedBridgeMints  map[string]struct{}
	NativeTokenOutAddress string
}

func parseConfig(cfg config.RawChainConfig) (*Config, error) {
	ret := &Config{
		Name:                  cfg.Name,
		Id:                    cfg.Id,
		Endpoint:              cfg.Endpoint,
		StartBlock:            make(map[string]string),
		Butter:                cfg.Opts.Butter,
		ButterApiKey:          cfg.Opts.ButterApiKey,
		ButterEntrance:        cfg.Opts.ButterEntrance,
		SupportedBridgeMints:  make(map[string]struct{}),
		NativeTokenOutAddress: defaultNativeTokenOutAddress,
	}
	if ret.ButterApiKey == "" {
		ret.ButterApiKey = os.Getenv("BUTTER_API_KEY")
	}

	seen := make(map[string]struct{})
	for _, s := range strings.Split(cfg.Opts.Mcs, ",") {
		s = strings.TrimSpace(s)
		if _, ok := seen[s]; ok || s == "" {
			continue
		}
		seen[s] = struct{}{}
		pk, err := solana.PublicKeyFromBase58(s)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid mcs program id %s", s)
		}
		ret.Mcs = append(ret.Mcs, pk)
	}
	if len(ret.Mcs) == 0 {
		return nil, errors.New("solana opts.mcs is empty")
	}

	for _, pk := range ret.Mcs {
		program := pk.String()
		if cfg.Opts.StartBlock != "" {
			ret.StartBlock[program] = cfg.Opts.StartBlock
		} else if sig, ok := cfg.Opts.StartBlocks[program]; ok {
			ret.StartBlock[program] = sig
		}
	}

	mints := cfg.Opts.SupportedBridgeMints
	if len(mints) == 0 {
		mints = defaultSupportedBridgeMints
	}
	for _, m := range mints {
		ret.SupportedBridgeMints[m] = struct{}{}
	}

	if cfg.Opts.NativeTokenOutAddress != "" {
		if !common.IsHexAddress(cfg.Opts.NativeTokenOutAddress) {
			return nil, errors.New("nativeTokenOutAddress is not an evm address")
		}
		ret.NativeTokenOutAddress = cfg.Opts.NativeTokenOutAddress
	}
	return ret, nil
}
