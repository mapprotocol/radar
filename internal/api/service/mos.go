package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mapprotocol/filter/internal/api/store"
	"github.com/mapprotocol/filter/internal/api/store/mysql"
	"github.com/mapprotocol/filter/internal/api/stream"
	"gorm.io/gorm"
)

type MosSrv interface {
	List(context.Context, *stream.MosListReq) (*stream.MosListResp, error)
	MaxID(context.Context, *stream.MosListReq) (map[string]interface{}, error)
	BlockList(context.Context, *stream.MosListReq) (*stream.MosListResp, error)
}

type Mos struct {
	store      store.Moser
	event      store.Evener
	updateTime int64
	lock       *sync.RWMutex
	eventCache map[string][]int64
}

func NewMosSrv(db *gorm.DB) MosSrv {
	return &Mos{
		store:      mysql.NewMos(db),
		event:      mysql.NewEvent(db),
		eventCache: make(map[string][]int64),
		lock:       &sync.RWMutex{},
	}
}

func (m *Mos) List(ctx context.Context, req *stream.MosListReq) (*stream.MosListResp, error) {
	if req.Limit == 0 || req.Limit > 100 {
		req.Limit = 10
	}

	if req.Id == 0 {
		req.Id = 1
	}

	splits := strings.Split(req.Topic, ",")
	eventIds := make([]int64, 0, len(splits))
	for _, sp := range splits {
		m.lock.RLock()
		id, ok := m.eventCache[sp]
		m.lock.RUnlock()
		if ok && time.Now().Unix()-m.updateTime < 300 {
			eventIds = append(eventIds, id...)
			continue
		}
		events, _, err := m.event.List(ctx, &store.EventCond{Topic: sp, Limit: 100})
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(events))
		for _, ele := range events {
			ids = append(ids, ele.Id)
		}
		m.lock.Lock()
		m.eventCache[sp] = ids
		m.lock.Unlock()
		eventIds = append(eventIds, ids...)
		m.updateTime = time.Now().Unix()
	}

	list, total, err := m.store.List(ctx, &store.MosCond{
		Id:          req.Id,
		ChainId:     req.ChainId,
		ProjectId:   req.ProjectId,
		EventIds:    eventIds,
		BlockNumber: req.BlockNumber,
		TxHash:      req.TxHash,
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*stream.GetMosResp, 0, req.Limit)
	for _, ele := range list {
		ret = append(ret, &stream.GetMosResp{
			Id:              ele.Id,
			ProjectId:       ele.ProjectId,
			ChainId:         ele.ChainId,
			EventId:         ele.EventId,
			TxHash:          ele.TxHash,
			ContractAddress: ele.ContractAddress,
			Topic:           ele.Topic,
			BlockNumber:     ele.BlockNumber,
			BlockHash:       ele.BlockHash,
			LogIndex:        ele.LogIndex,
			LogData:         ele.LogData,
			TxIndex:         ele.TxIndex,
			TxTimestamp:     ele.TxTimestamp,
		})
	}
	return &stream.MosListResp{
		Total: total,
		List:  ret,
	}, nil
}

func (m *Mos) MaxID(ctx context.Context, req *stream.MosListReq) (map[string]interface{}, error) {
	splits := strings.Split(req.Topic, ",")
	eventIds := make([]int64, 0, len(splits))
	for _, sp := range splits {
		m.lock.RLock()
		id, ok := m.eventCache[sp]
		m.lock.RUnlock()
		if ok && time.Now().Unix()-m.updateTime < 300 {
			eventIds = append(eventIds, id...)
			continue
		}
		events, _, err := m.event.List(ctx, &store.EventCond{Topic: sp, Limit: 100})
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(events))
		for _, ele := range events {
			ids = append(ids, ele.Id)
		}
		m.lock.Lock()
		m.eventCache[sp] = ids
		m.lock.Unlock()
		eventIds = append(eventIds, ids...)
		m.updateTime = time.Now().Unix()
	}

	id, err := m.store.MaxID(ctx, &store.MosCond{
		Id:          req.Id,
		ChainId:     req.ChainId,
		ProjectId:   req.ProjectId,
		EventIds:    eventIds,
		BlockNumber: req.BlockNumber,
		TxHash:      req.TxHash,
		Limit:       1,
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": id}, nil
}

func (m *Mos) BlockList(ctx context.Context, req *stream.MosListReq) (*stream.MosListResp, error) {
	if req.Limit == 0 || req.Limit > 100 {
		req.Limit = 10
	}

	splits := strings.Split(req.Topic, ",")
	eventIds := make([]int64, 0, len(splits))
	for _, sp := range splits {
		m.lock.RLock()
		id, ok := m.eventCache[sp]
		m.lock.RUnlock()
		if ok && time.Now().Unix()-m.updateTime < 300 {
			eventIds = append(eventIds, id...)
			continue
		}
		events, _, err := m.event.List(ctx, &store.EventCond{Topic: sp, Limit: 100})
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(events))
		for _, ele := range events {
			ids = append(ids, ele.Id)
		}
		m.lock.Lock()
		m.eventCache[sp] = ids
		m.lock.Unlock()
		eventIds = append(eventIds, ids...)
		m.updateTime = time.Now().Unix()
	}

	list, total, err := m.store.BlockList(ctx, &store.MosCond{
		ChainId:     req.ChainId,
		ProjectId:   req.ProjectId,
		EventIds:    eventIds,
		BlockNumber: req.BlockNumber,
		TxHash:      req.TxHash,
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*stream.GetMosResp, 0, req.Limit)
	for _, ele := range list {
		ret = append(ret, &stream.GetMosResp{
			Id:              ele.Id,
			ProjectId:       ele.ProjectId,
			ChainId:         ele.ChainId,
			EventId:         ele.EventId,
			TxHash:          ele.TxHash,
			ContractAddress: ele.ContractAddress,
			Topic:           ele.Topic,
			BlockNumber:     ele.BlockNumber,
			LogIndex:        ele.LogIndex,
			LogData:         ele.LogData,
			TxIndex:         ele.TxIndex,
			TxTimestamp:     ele.TxTimestamp,
		})
	}
	return &stream.MosListResp{
		Total: total,
		List:  ret,
	}, nil
}
