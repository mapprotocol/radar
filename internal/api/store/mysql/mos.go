package mysql

import (
	"context"

	"github.com/mapprotocol/filter/internal/api/store"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Mos struct {
	db *gorm.DB
}

func NewMos(db *gorm.DB) *Mos {
	return &Mos{db: db}
}

func (m *Mos) Create(ctx context.Context, ele *dao.Mos) error {
	return m.db.WithContext(ctx).Create(ele).Error
}

func (m *Mos) Delete(ctx context.Context, id int64) error {
	return m.db.WithContext(ctx).Where("id = ?", id).Delete(&dao.Mos{}).Error
}

func (m *Mos) Get(ctx context.Context, c *store.MosCond) (*dao.Mos, error) {
	db := m.db.WithContext(ctx)
	if c.Id != 0 {
		db = db.Where("id = ?", c.Id)
	}
	if c.ChainId != 0 {
		db = db.Where("chain_id = ?", c.ChainId)
	}
	if c.ProjectId != 0 {
		db = db.Where("project_id = ?", c.ProjectId)
	}
	if c.BlockNumber != 0 {
		db = db.Where("block_number = ?", c.BlockNumber)
	}
	if c.EventId != 0 {
		db = db.Where("event_id = ?", c.EventId)
	}
	if c.TxHash != "" {
		db = db.Where("tx_hash = ?", c.TxHash)
	}
	ret := dao.Mos{}
	err := db.First(&ret).Error
	return &ret, err
}

func (m *Mos) List(ctx context.Context, c *store.MosCond) ([]*dao.Mos, int64, error) {
	db := m.db.WithContext(ctx)
	if c.Id != 0 {
		db = db.Where("id > ?", c.Id)
	}
	if c.BlockNumber != 0 {
		db = db.Where("block_number >= ?", c.BlockNumber)
	}
	if c.ChainId != 0 {
		db = db.Where("chain_id = ?", c.ChainId)
	}
	if c.ProjectId != 0 {
		db = db.Where("project_id = ?", c.ProjectId)
	}
	if c.EventId != 0 {
		db = db.Where("event_id = ?", c.EventId)
	}
	if c.TxHash != "" {
		db = db.Where("tx_hash = ?", c.TxHash)
	}
	//if len(c.EventIds) != 0 {
	db = db.Where("event_id IN ?", c.EventIds)
	//}
	total := int64(0)
	// err := db.Model(&dao.Mos{}).Count(&total).Error
	// if err != nil {
	// 	return nil, 0, err
	// }
	ret := make([]*dao.Mos, 0)
	err := db.Limit(c.Limit).Order(clause.OrderByColumn{
		Column: clause.Column{Table: clause.CurrentTable, Name: clause.PrimaryKey},
	}).Find(&ret).Error
	return ret, total, err
}

func (m *Mos) BlockList(ctx context.Context, c *store.MosCond) ([]*dao.Mos, int64, error) {
	db := m.db.WithContext(ctx)
	db = db.Where("block_number = ?", c.BlockNumber).Where("event_id IN ?", c.EventIds)
	if c.ChainId != 0 {
		db = db.Where("chain_id = ?", c.ChainId)
	}
	if c.ProjectId != 0 {
		db = db.Where("project_id = ?", c.ProjectId)
	}
	if c.EventId != 0 {
		db = db.Where("event_id = ?", c.EventId)
	}

	total := int64(0)
	// err := db.Model(&dao.Mos{}).Count(&total).Error
	// if err != nil {
	// 	return nil, 0, err
	// }
	ret := make([]*dao.Mos, 0)
	err := db.Limit(c.Limit).Find(&ret).Error
	return ret, total, err
}
