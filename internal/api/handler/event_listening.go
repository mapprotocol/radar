package handler

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/stream"
)

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
