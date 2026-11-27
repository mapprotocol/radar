package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/service"
	"github.com/mapprotocol/filter/internal/api/stream"
)

type EventStatistics struct {
	srv *service.EventStatistics
}

func NewEventStatistics(srv *service.EventStatistics) *EventStatistics {
	return &EventStatistics{srv: srv}
}

// Get 每日事件统计
//
//	@Summary		每日事件统计
//	@Description	按天、按链统计 MessageOut / MessageIn 数量；详见 API_EVENT_STATISTICS.md
//	@Tags			event
//	@Produce		json
//	@Success		200	{object}	stream.CommonResp{data=stream.EventStatisticsResp}
//	@Failure		401	{object}	stream.CommonResp
//	@Failure		503	{object}	stream.CommonResp	"统计数据尚未就绪"
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/event/statistics [get]
func (h *EventStatistics) Get(c *gin.Context) {
	ret, err := h.srv.Get(time.Now())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, stream.CommonResp{
			Code: http.StatusServiceUnavailable, Message: err.Error(),
		})
		return
	}
	WriteResponse(c, nil, ret)
}
