package xrp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/mapprotocol/filter/internal/filter/chain/ethereum"
	"github.com/mapprotocol/filter/internal/pkg/stream"
	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
	"github.com/mapprotocol/filter/internal/pkg/constant"
)

type Conner interface {
	HttpClient() *http.Client
	ethereum.Conner
}

type Connection struct {
	endpoint         string
	conn             *http.Client
	reqTime          int64
	cacheBNum, nonce uint64
}

// NewConn returns an uninitialized connection, must call Connection.Connect() before using.
func NewConn(endpoint string) *Connection {
	return &Connection{
		endpoint: endpoint,
	}
}

func newHTTPClient() *http.Client {
	return rpclog.NewHTTPClient(time.Minute)
}

// Connect starts the ethereum WS connection
func (c *Connection) Connect() error {
	fmt.Println("Connecting to xrp.go chain...", "url", c.endpoint)
	c.conn = newHTTPClient()

	return nil
}

func (c *Connection) newTransactOpts(value, gasLimit, gasPrice *big.Int) (*bind.TransactOpts, uint64, error) {
	return nil, 1, nil
}

func (c *Connection) Client() *ethclient.Client {
	return nil
}

func (c *Connection) HttpClient() *http.Client {
	return c.conn
}

// LatestBlock returns the latest block from the current chain
func (c *Connection) LatestBlock() (uint64, error) {
	if time.Now().Unix()-c.reqTime < constant.ReqInterval {
		return c.cacheBNum, nil
	}

	payload := map[string]interface{}{
		"method": "ledger",
		"params": []map[string]interface{}{{
			"ledger_index": "validated",
		}},
	}
	body, _ := json.Marshal(payload)

	for i := 0; i < 3; i++ {
		resp, err := c.conn.Post(c.endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			return 0, errors.Wrap(err, "http post error")
		}

		var result stream.LedgerResp
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			return 0, errors.Wrap(err, "error decoding response")
		}

		if result.Result.Status != "success" {
			time.Sleep(time.Millisecond * 100)
			continue
		}
		if result.Result.LedgerIndex == 0 {
			time.Sleep(time.Millisecond * 100)
			continue
		}
		c.cacheBNum = uint64(result.Result.LedgerIndex)
		break
	}

	c.reqTime = time.Now().Unix()
	return c.cacheBNum, nil
}

// Close terminates the client connection and stops any running routines
func (c *Connection) Close() {

}
