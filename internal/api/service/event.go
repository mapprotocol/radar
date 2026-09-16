package service

import (
	"context"
	"github.com/mapprotocol/filter/internal/api/store"
	"github.com/mapprotocol/filter/internal/api/store/mysql"
	"github.com/mapprotocol/filter/internal/api/stream"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"gorm.io/gorm"
	"strconv"
	"time"
)

type EventSrv interface {
	Add(context.Context, *stream.AddEventReq) error
	Get(context.Context, *stream.GetEventReq) (*stream.GetEventResp, error)
	Del(context.Context, *stream.DelEventReq) error
	List(context.Context, *stream.EventListReq) (*stream.EventListResp, error)
	Listening(context.Context, uint64) (*stream.ListeningEventsResp, error)
}

type Event struct {
	store store.Evener
}

func NewMysqlEventSrv(db *gorm.DB) EventSrv {
	return &Event{store: mysql.NewEvent(db)}
}

func (p *Event) Add(ctx context.Context, req *stream.AddEventReq) error {
	es := constant.EventSig(req.Format)
	bn := req.BlockNumber
	if bn == "" {
		bn = constant.LatestBlock
	}
	cId := ""
	if req.ChainId != 0 {
		cId = strconv.FormatInt(req.ChainId, 10)
	}
	err := p.store.Create(ctx, &dao.Event{
		ProjectId:   req.ProjectId,
		ChainId:     cId,
		Address:     req.Address,
		Format:      req.Format,
		Topic:       es.GetTopic().String(),
		BlockNumber: bn,
		CreatedAt:   time.Now(),
	})
	return err
}

func (p *Event) Get(ctx context.Context, req *stream.GetEventReq) (*stream.GetEventResp, error) {
	ele, err := p.store.Get(ctx, &store.EventCond{
		Id:        req.Id,
		ProjectId: req.ProjectId,
		Format:    req.Format,
		Topic:     req.Topic,
	})
	if err != nil {
		return nil, err
	}
	ret := &stream.GetEventResp{
		Id:        ele.Id,
		ProjectId: ele.ProjectId,
		Format:    ele.Format,
		Topic:     ele.Topic,
		Created:   ele.CreatedAt.Unix(),
	}
	return ret, nil
}

func (p *Event) Del(ctx context.Context, req *stream.DelEventReq) error {
	return p.store.Delete(ctx, req.Id)
}

func (p *Event) List(ctx context.Context, req *stream.EventListReq) (*stream.EventListResp, error) {
	list, total, err := p.store.List(ctx, &store.EventCond{
		Id:        req.Id,
		ProjectId: req.ProjectId,
		Format:    req.Format,
		Topic:     req.Topic,
		Page:      req.Offset,
		Limit:     req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*stream.GetEventResp, 0, req.Limit)
	for _, ele := range list {
		ret = append(ret, &stream.GetEventResp{
			Id:        ele.Id,
			ProjectId: ele.ProjectId,
			Format:    ele.Format,
			Topic:     ele.Topic,
			Created:   ele.CreatedAt.Unix(),
		})
	}
	return &stream.EventListResp{
		Total: total,
		Page:  req.Offset,
		Limit: req.Limit,
		List:  ret,
	}, nil
}
