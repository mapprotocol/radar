package ethereum

import (
	"fmt"
	"time"

	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/pkg/errors"
)

func (c *Chain) renewEvent() error {
	for {
		select {
		case <-c.eventStop:
			return errors.New("renewEvent polling terminated")
		default:
			time.Sleep(time.Second * 30)
			err := c.getMatch(false)
			if err != nil {
				return errors.Wrap(err, "renewEvent failed")
			}
		}
	}
}

func (c *Chain) getMatch(init bool) error {
	for _, s := range c.storages {
		if s.Type() != constant.Mysql {
			continue
		}
		events, err := s.GetEvent(c.eventId)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s get events failed", s.Type()))
		}
		for _, e := range events {
			tmp := e
			c.eventId = tmp.Id
			if tmp.ChainId != "" && tmp.ChainId != c.cfg.Id {
				continue
			}
			c.events = append(c.events, tmp)
			c.log.Info("Add new event", "event", e.Format, "addr", e.Address,
				"project", e.ProjectId, "eventId", e.Id)
			if !init && tmp.BlockNumber != constant.LatestBlock {
				c.rangeScan(tmp, c.currentProgress)
			}
		}
	}
	return nil
}
