package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/service"
	"github.com/mapprotocol/filter/internal/api/stream"
	"github.com/mapprotocol/filter/internal/pkg/constant"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Event struct {
	srv service.EventSrv
}

func NewEvent(db *gorm.DB) *Event {
	return &Event{srv: service.NewMysqlEventSrv(db)}
}

// Add 注册要监听的事件
//
//	@Summary		注册事件
//	@Description	format 为事件签名（如 MessageOut(bytes32,uint256,bytes)），topic 由服务端计算；
//	@Description	不传 chain_id 表示所有链都监听；指定 block_number 时必须同时指定 chain_id
//	@Tags			event
//	@Accept			json
//	@Produce		json
//	@Param			body	body		stream.AddEventReq	true	"project_id、format、address 必填"
//	@Success		200		{object}	stream.CommonResp	"code=200 成功，其他 code 为失败"
//	@Failure		401		{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/event [post]
func (p *Event) Add(c *gin.Context) {
	var req stream.AddEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	if req.Format == "" {
		WriteResponse(c, errors.New("param format is empty"), nil)
		return
	}
	if req.Address == "" || req.Address == constant.ZeroAddress.String() {
		WriteResponse(c, errors.New("param address is empty"), nil)
		return
	}
	if req.ProjectId == 0 {
		WriteResponse(c, errors.New("param project is zero"), nil)
		return
	}
	if req.BlockNumber != "" && req.ChainId == 0 {
		WriteResponse(c, errors.New("appoint blockNumber must appoint chain_id"), nil)
		return
	}

	err := p.srv.Add(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "add Event failed"), nil)
		return
	}
	WriteResponse(c, nil, nil)
}

// Get 查询单个事件
//
//	@Summary		查询事件
//	@Description	注意：参数从 JSON body 读取（GET 带 body），Swagger UI 和浏览器无法发送，请用 curl -X GET -d
//	@Tags			event
//	@Accept			json
//	@Produce		json
//	@Param			body	body		stream.GetEventReq							true	"按 id / project_id / format / topic 过滤"
//	@Success		200		{object}	stream.CommonResp{data=stream.GetEventResp}	"code=200 成功，其他 code 为失败"
//	@Failure		401		{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/event [get]
func (p *Event) Get(c *gin.Context) {
	var req stream.GetEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	ret, err := p.srv.Get(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get Event failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}

// Delete 删除事件
//
//	@Summary	删除事件
//	@Tags		event
//	@Accept		json
//	@Produce	json
//	@Param		body	body		stream.DelEventReq	true	"id 必填"
//	@Success	200		{object}	stream.CommonResp	"code=200 成功，其他 code 为失败"
//	@Failure	401		{object}	stream.CommonResp
//	@Security	ApiKeyAuth
//	@Security	BearerAuth
//	@Router		/v1/event [delete]
func (p *Event) Delete(c *gin.Context) {
	var req stream.DelEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}
	if req.Id == 0 {
		WriteResponse(c, errors.New("param event id is zero"), nil)
		return
	}

	err := p.srv.Del(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "del Event failed"), nil)
		return
	}
	WriteResponse(c, nil, nil)
}

// List 分页查询事件
//
//	@Summary	事件列表
//	@Tags		event
//	@Produce	json
//	@Param		id			query		int		true	"事件 id（必填，非 0）"
//	@Param		project_id	query		int		false	"项目 id"
//	@Param		format		query		string	false	"事件签名"
//	@Param		topic		query		string	false	"事件 topic"
//	@Param		offset		query		int		false	"偏移"
//	@Param		limit		query		int		false	"条数"
//	@Success	200			{object}	stream.CommonResp{data=stream.EventListResp}	"code=200 成功，其他 code 为失败"
//	@Failure	401			{object}	stream.CommonResp
//	@Security	ApiKeyAuth
//	@Security	BearerAuth
//	@Router		/v1/event/list [get]
func (p *Event) List(c *gin.Context) {
	var req stream.EventListReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}
	if req.Id == 0 {
		WriteResponse(c, errors.New("Param event id is zero"), nil)
		return
	}

	ret, err := p.srv.List(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "Get Event list failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}
