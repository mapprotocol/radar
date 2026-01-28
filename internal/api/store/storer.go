package store

import (
	"context"

	"github.com/mapprotocol/filter/internal/pkg/dao"
)

type Projector interface {
	Create(ctx context.Context, ele *dao.Project) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, c *ProjectCond) (*dao.Project, error)
	List(ctx context.Context, c *ProjectCond) ([]*dao.Project, error)
}

type ProjectCond struct {
	Id   int64
	Name string
}

type Evener interface {
	Create(ctx context.Context, ele *dao.Event) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, c *EventCond) (*dao.Event, error)
	List(ctx context.Context, c *EventCond) ([]*dao.Event, int64, error)
}

type EventCond struct {
	Id, ProjectId int64
	Format, Topic string
	Page, Limit   int64
}

type Moser interface {
	Create(ctx context.Context, ele *dao.Mos) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, c *MosCond) (*dao.Mos, error)
	List(ctx context.Context, c *MosCond) ([]*dao.Mos, int64, error)
	BlockList(ctx context.Context, c *MosCond) ([]*dao.Mos, int64, error)
}

type MosCond struct {
	Id, ChainId, ProjectId, EventId int64
	BlockNumber                     uint64
	TxHash                          string
	Limit                           int
	EventIds                        []int64
}

type Blocker interface {
	Get(ctx context.Context, c *BlockCond) (*dao.Block, error)
	GetCurrentScan(ctx context.Context, c *BlockCond) (*dao.ScanBlock, error)
}

type BlockCond struct {
	ChainId int64
}
