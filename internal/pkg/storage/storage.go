package storage

import (
	"errors"

	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
)

var (
	ErrorOfStorageType = errors.New("storage type is miss")
)

type Saver interface {
	Type() string
	Mos(uint64, []*dao.Mos) error
	LatestBlockNumber(chainId string, latest uint64) error
	GetEvent(int64) ([]*dao.Event, error)
}

func NewSaver(tp, url string) (Saver, error) {
	switch tp {
	case constant.Redis:
		return NewRds(url)
	case constant.Mysql:
		return NewMysql(url)
	default:
		return nil, ErrorOfStorageType
	}
}
