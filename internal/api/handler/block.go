package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/service"
	"github.com/mapprotocol/filter/internal/api/stream"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Block struct {
	srv service.BlockSrv
}

func NewBlock(db *gorm.DB) *Block {
	return &Block{srv: service.NewBlockSrv(db)}
}

// Get 查询链上最新高度
//
//	@Summary		链上最新高度
//	@Description	filter 主实例约每 3 秒写一次，data 为十进制字符串
//	@Tags			block
//	@Produce		json
//	@Param			chain_id	query		int									true	"链 id（必填，非 0）"
//	@Success		200			{object}	stream.CommonResp{data=string}	"code=200 成功，其他 code 为失败"
//	@Failure		401			{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/block [get]
func (p *Block) Get(c *gin.Context) {
	var req stream.GetBlockReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	if req.ChainId == 0 {
		WriteResponse(c, errors.New("param chain id is empty"), nil)
		return
	}

	ret, err := p.srv.Get(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get block failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}

// GetCurrentScan 查询当前扫描进度
//
//	@Summary		扫描进度
//	@Description	filter 主实例约每 3 秒写一次已扫描到的高度，data 为十进制字符串
//	@Tags			block
//	@Produce		json
//	@Param			chain_id	query		int									true	"链 id（必填，非 0）"
//	@Success		200			{object}	stream.CommonResp{data=string}	"code=200 成功，其他 code 为失败"
//	@Failure		401			{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/block/scan [get]
func (p *Block) GetCurrentScan(c *gin.Context) {
	var req stream.GetBlockReq
	if err := c.ShouldBind(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	if req.ChainId == 0 {
		WriteResponse(c, errors.New("param chain id is empty"), nil)
		return
	}

	ret, err := p.srv.GetCurrentScan(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "get block failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}
