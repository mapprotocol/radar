package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mapprotocol/filter/internal/pkg/constant"
)

const (
	DefaultConfigPath   = "./config.json"
	DefaultBlockConfirm = 6
)

type Config struct {
	Events       string           `json:"events"`
	Address      string           `json:"address"`
	Chains       []RawChainConfig `json:"chains"`
	Other        Construction     `json:"other,omitempty"`
	Storages     []Storage        `json:"storages"`
	KeystorePath string           `json:"keystore_path"`
}

// RawChainConfig is parsed directly from the config file and should be using to construct the core.ChainConfig
type RawChainConfig struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Id           string `json:"id"`       // ChainID
	Endpoint     string `json:"endpoint"` // url for rpc endpoint
	KeystorePath string `json:"keystore_path"`
	Opts         opt    `json:"opts"`
}

type opt struct {
	Mcs                string `json:"mcs,omitempty"`
	Redis              string `json:"redis,omitempty"`
	StartBlock         string `json:"startBlock,omitempty"`
	Event              string `json:"event,omitempty"`
	BlockConfirmations string `json:"blockConfirmations,omitempty"`
	Range              string `json:"range,omitempty"`
	Butter             string `json:"butter,omitempty"`
}

type Construction struct {
	MonitorUrl                  string `json:"monitor_url,omitempty"`
	Env                         string `json:"env,omitempty"`
	Butter                      string `json:"butter,omitempty"`
	ObservabilityAddr           string `json:"observability_addr,omitempty"`
	OpsHubObservabilityURL      string `json:"ops_hub_observability_url,omitempty"`
	OpsHubObservabilityAPIKey   string `json:"ops_hub_observability_api_key,omitempty"`
	OpsHubObservabilityInstance string `json:"ops_hub_observability_instance,omitempty"`
	OpsHubObservabilityInterval string `json:"ops_hub_observability_interval,omitempty"`
}

type Storage struct {
	Type string `json:"type"`
	Url  string `json:"url"`
}

func (c *Config) validate() error {
	for idx, chain := range c.Chains {
		if chain.Id == "" {
			return fmt.Errorf("required field chain.Id empty for chain %s", chain.Id)
		}
		if chain.Type == "" {
			c.Chains[idx].Type = constant.Ethereum
		}
		if chain.Name == "" {
			return fmt.Errorf("required field chain.Name empty for chain %s", chain.Id)
		}
	}
	return nil
}

func Local(cfgFile string) (*Config, error) {
	var fig Config
	path := DefaultConfigPath
	if cfgFile != "" {
		path = cfgFile
	}

	err := loadConfig(path, &fig)
	if err != nil {
		return &fig, err
	}

	err = fig.validate()
	if err != nil {
		return nil, err
	}
	return &fig, nil
}

func loadConfig(file string, config *Config) error {
	ext := filepath.Ext(file)
	fp, err := filepath.Abs(file)
	if err != nil {
		return err
	}

	f, err := os.Open(filepath.Clean(fp))
	if err != nil {
		return err
	}

	if ext == ".json" {
		if err = json.NewDecoder(f).Decode(&config); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unrecognized extention: %s", ext)
	}

	return nil
}
