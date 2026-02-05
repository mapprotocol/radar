package storage

import (
	glog "log"
	"os"
	"strconv"
	"strings"

	"github.com/mapprotocol/filter/pkg/utils"

	"github.com/ethereum/go-ethereum/log"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Mysql struct {
	dsn          string
	db           *gorm.DB
	chainMapping *utils.RWMap
}

func NewMysql(dsn string) (*Mysql, error) {
	m := &Mysql{dsn: dsn, chainMapping: utils.NewRWMap()}
	err := m.init()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Mysql) init() error {
	mdb, err := gorm.Open(mysql.Open(m.dsn), &gorm.Config{
		Logger: logger.New(
			glog.New(os.Stdout, "\r\n", glog.LstdFlags),
			logger.Config{
				LogLevel: logger.Warn,
				Colorful: false,
			},
		),
	})

	if err != nil {
		return err
	}
	m.db = mdb
	return nil
}

func (m *Mysql) Type() string {
	return constant.Mysql
}

func (m *Mysql) Mos(toChainId uint64, event []*dao.Mos) error {
	err := m.db.Create(&event).Error
	if err != nil {
		if strings.Index(err.Error(), "Duplicate") != -1 {
			log.Info("log is inserted", "event", event)
			return nil
		}
		return err
	}
	return nil
}

func (m *Mysql) LatestBlockNumber(chainId string, latest uint64) error {
	key := "latest_" + chainId
	id, ok := m.chainMapping.Get(key)
	if ok {
		err := m.db.Model(&dao.Block{}).Where("id = ?", id).Update("number", strconv.FormatUint(latest, 10)).Error
		if err != nil {
			return err
		}
		return nil
	}
	blk := &dao.Block{}
	err := m.db.Model(&dao.Block{}).Where("chain_id = ?", chainId).First(blk).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = m.db.Create(&dao.Block{
				ChainId: chainId,
				Number:  strconv.FormatUint(latest, 10),
			}).Error
		}
		return err
	}
	m.chainMapping.Set(key, blk.Id)
	err = m.db.Model(&dao.Block{}).Where("id = ?", id).Update("number", strconv.FormatUint(latest, 10)).Error
	if err != nil {
		return err
	}
	return nil
}

func (m *Mysql) GetEvent(id int64) ([]*dao.Event, error) {
	ret := make([]*dao.Event, 0)
	err := m.db.Model(&dao.Event{}).Where("id > ?", id).Find(&ret).Error
	return ret, err
}
