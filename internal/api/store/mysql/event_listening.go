package mysql

import (
	"context"
	"errors"
	"strconv"

	"github.com/mapprotocol/filter/internal/pkg/dao"
)

func (e *Event) Listening(ctx context.Context, chainID uint64) ([]*dao.Event, error) {
	if chainID == 0 {
		return nil, errors.New("chain_id must be a positive integer")
	}
	ret := make([]*dao.Event, 0)
	// NULL and empty chain IDs are global definitions loaded by the listeners.
	err := e.db.WithContext(ctx).
		Where("(chain_id = ? OR chain_id IS NULL OR chain_id = '')", strconv.FormatUint(chainID, 10)).
		Where("deleted_at IS NULL").
		Order("id ASC").Find(&ret).Error
	return ret, err
}
