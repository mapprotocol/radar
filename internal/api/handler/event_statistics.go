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
