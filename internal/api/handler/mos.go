package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/service"
	"github.com/mapprotocol/filter/internal/api/stream"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// MaxIDResp 只用于文档：MaxID 实际返回 map[string]interface{}{"id": id}
type MaxIDResp struct {
	Id int64 `json:"id"`
}

type Mos struct {
	srv service.MosSrv
}

func NewMos(db *gorm.DB) *Mos {
	return &Mos{srv: service.NewMosSrv(db)}
}

// List 按游标拉取已匹配的事件日志
//
//	@Summary		事件日志列表
//	@Description	按 id 游标增量拉取：返回 id 大于参数 id 的记录，id 不传按 1 处理；limit 不传或大于 100 时按 10 处理
//	@Tags			mos
//	@Produce		json
//	@Param		project_id		query		int		true	"项目 id（必填，非 0）"
//	@Param		chain_id		query		int		false	"链 id"
//	@Param		topic			query		string	false	"事件 topic，多个用英文逗号分隔"
//	@Param		id				query		int		false	"游标：只返回 id 大于它的记录"
//	@Param		block_number	query		int		false	"区块高度"
//	@Param		tx_hash			query		string	false	"交易 hash"
//	@Param		limit			query		int		false	"条数，1~100，默认 10"
//	@Success		200				{object}	stream.CommonResp{data=stream.MosListResp}	"code=200 成功，其他 code 为失败"
//	@Failure		401				{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/mos/list [get]
func (m *Mos) List(c *gin.Context) {
	var req stream.MosListReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}
	if req.ProjectId == 0 {
		WriteResponse(c, errors.New("param project id is zero"), nil)
		return
	}

	ret, err := m.srv.List(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get Mos list failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}

// MaxID 查询满足条件的最大日志 id
//
//	@Summary	最大日志 id
//	@Tags		mos
//	@Produce	json
//	@Param		project_id		query		int		true	"项目 id（必填，非 0）"
//	@Param		chain_id		query		int		false	"链 id"
//	@Param		topic			query		string	false	"事件 topic，多个用英文逗号分隔"
//	@Param		id				query		int		false	"游标：只返回 id 大于它的记录"
//	@Param		block_number	query		int		false	"区块高度"
//	@Param		tx_hash			query		string	false	"交易 hash"
//	@Success	200	{object}	stream.CommonResp{data=handler.MaxIDResp}	"code=200 成功，其他 code 为失败"
//	@Failure	401	{object}	stream.CommonResp
//	@Security	ApiKeyAuth
//	@Security	BearerAuth
//	@Router		/v1/mos/max/id [get]
func (m *Mos) MaxID(c *gin.Context) {
	var req stream.MosListReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}
	if req.ProjectId == 0 {
		WriteResponse(c, errors.New("param project id is zero"), nil)
		return
	}

	ret, err := m.srv.MaxID(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get Mos max id failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}

// BlockList 按链拉取事件日志（不限项目）
//
//	@Summary	按链拉取事件日志
//	@Tags		mos
//	@Produce	json
//	@Param		chain_id		query		int		true	"链 id（必填，非 0）"
//	@Param		project_id		query		int		false	"项目 id"
//	@Param		topic			query		string	false	"事件 topic，多个用英文逗号分隔"
//	@Param		id				query		int		false	"游标：只返回 id 大于它的记录"
//	@Param		block_number	query		int		false	"区块高度"
//	@Param		tx_hash			query		string	false	"交易 hash"
//	@Param		limit			query		int		false	"条数，1~100，默认 10"
//	@Success	200				{object}	stream.CommonResp{data=stream.MosListResp}	"code=200 成功，其他 code 为失败"
//	@Failure	401				{object}	stream.CommonResp
//	@Security	ApiKeyAuth
//	@Security	BearerAuth
//	@Router		/v1/mos/block/list [get]
func (m *Mos) BlockList(c *gin.Context) {
	var req stream.MosListReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}
	if req.ChainId == 0 {
		WriteResponse(c, errors.New("param chain id is zero"), nil)
		return
	}

	ret, err := m.srv.BlockList(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get Mos list failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}
