// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package ethereum

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
	"github.com/mapprotocol/filter/internal/pkg/constant"
)

type Conner interface {
	Client() *ethclient.Client
	LatestBlock() (uint64, error)
	Close()
}

type Connection struct {
	endpoint         string
	conn             *ethclient.Client
	reqTime          int64
	cacheBNum, nonce uint64
	kp               *keystore.Key
	opts             *bind.TransactOpts
}

// NewConn returns an uninitialized connection, must call Connection.Connect() before using.
func NewConn(endpoint string, kp *keystore.Key) *Connection {
	return &Connection{
		endpoint: endpoint,
		kp:       kp,
	}
}

func newHTTPClient() *http.Client {
	return rpclog.NewHTTPClient(time.Minute, http.DefaultTransport)
}

// Connect starts the ethereum WS connection
func (c *Connection) Connect() error {
	var (
		err       error
		rpcClient *rpc.Client
	)
	fmt.Println("Connecting to ethereum chain...", "url", c.endpoint)
	cli := newHTTPClient()
	withClient := rpc.WithHTTPClient(cli)
	rpcClient, err = rpc.DialOptions(context.Background(), c.endpoint, withClient)
	if err != nil {
		return err
	}
	c.conn = ethclient.NewClient(rpcClient)
	return nil
}

func (c *Connection) newTransactOpts(value, gasLimit, gasPrice *big.Int) (*bind.TransactOpts, uint64, error) {
	if c.kp == nil {
		return nil, 0, nil
	}
	privateKey := c.kp.PrivateKey
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	nonce, err := c.conn.PendingNonceAt(context.Background(), address)
	if err != nil {
		return nil, 0, err
	}

	id, err := c.conn.ChainID(context.Background())
	if err != nil {
		return nil, 0, err
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, id)
	if err != nil {
		return nil, 0, err
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = value
	auth.GasLimit = uint64(gasLimit.Int64())
	auth.GasPrice = gasPrice
	auth.Context = context.Background()

	return auth, nonce, nil
}

func (c *Connection) Client() *ethclient.Client {
	return c.conn
}

// LatestBlock returns the latest block from the current chain
func (c *Connection) LatestBlock() (uint64, error) {
	if time.Now().Unix()-c.reqTime < constant.ReqInterval {
		return c.cacheBNum, nil
	}
	num, err := c.conn.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}
	c.cacheBNum = num
	c.reqTime = time.Now().Unix()
	return num, nil
}

// Close terminates the client connection and stops any running routines
func (c *Connection) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
