package handler

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/stream"
)

// Listening 查询某条链当前生效的全部监听事件
//
//	@Summary		链上生效的监听事件
//	@Description	返回指定链的事件定义以及不限链的全局定义，跨所有项目，按 id 升序，不分页；详见 API_EVENT_LISTENING.md
//	@Tags			event
//	@Produce		json
//	@Param			chain_id	query		int												false	"链 id，单个正整数，默认 1"
//	@Success		200			{object}	stream.CommonResp{data=stream.ListeningEventsResp}
//	@Failure		400			{object}	stream.CommonResp	"chain_id 非法"
//	@Failure		401			{object}	stream.CommonResp
//	@Failure		500			{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/event/listening [get]
func (p *Event) Listening(c *gin.Context) {
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, stream.CommonResp{
			Code: http.StatusBadRequest, Message: "invalid query parameters",
		})
		return
	}

	chainID := uint64(1)
	if values, ok := query["chain_id"]; ok {
		valid := len(values) == 1 && values[0] != ""
		if valid {
			for _, char := range values[0] {
				if char < '0' || char > '9' {
					valid = false
					break
				}
			}
			chainID, err = strconv.ParseUint(values[0], 10, 64)
		}
		if !valid || err != nil || chainID == 0 {
			c.JSON(http.StatusBadRequest, stream.CommonResp{
				Code: http.StatusBadRequest, Message: "chain_id must be a single positive integer",
			})
			return
		}
	}

	ret, err := p.srv.Listening(c.Request.Context(), chainID)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, stream.CommonResp{
			Code: http.StatusInternalServerError, Message: "get listening events failed",
		})
		return
	}
	WriteResponse(c, nil, ret)
}
