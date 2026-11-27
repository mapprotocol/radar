package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/service"
	"github.com/mapprotocol/filter/internal/api/stream"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Project struct {
	srv service.ProjectSrv
}

func NewProject(db *gorm.DB) *Project {
	return &Project{srv: service.NewProSrv(db)}
}

// Add 创建项目
//
//	@Summary		创建项目
//	@Description	name 为必填；重名等业务失败同样返回 HTTP 200，需看 body 里的 code
//	@Tags			project
//	@Accept			json
//	@Produce		json
//	@Param			body	body		stream.AddProjectReq	true	"项目信息"
//	@Success		200		{object}	stream.CommonResp		"code=200 成功，其他 code 为失败"
//	@Failure		401		{object}	stream.CommonResp		"缺少或错误的 API Key，且不在 IP 白名单"
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/project [post]
func (p *Project) Add(c *gin.Context) {
	var req stream.AddProjectReq

	if err := c.ShouldBindJSON(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	if req.Name == "" {
		WriteResponse(c, errors.New("param  name is empty"), nil)
		return
	}

	err := p.srv.Add(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "add project failed"), nil)
		return
	}
	WriteResponse(c, nil, nil)
}

// Get 按名称查询项目
//
//	@Summary		查询项目
//	@Description	注意：参数从 JSON body 读取（GET 带 body），Swagger UI 和浏览器无法发送，请用 curl -X GET -d
//	@Tags			project
//	@Accept			json
//	@Produce		json
//	@Param			body	body		stream.GetProjectReq							true	"name 必填"
//	@Success		200		{object}	stream.CommonResp{data=stream.GetProjectResp}	"code=200 成功，其他 code 为失败"
//	@Failure		401		{object}	stream.CommonResp
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Router			/v1/project [get]
func (p *Project) Get(c *gin.Context) {
	var req stream.GetProjectReq

	if err := c.ShouldBindJSON(&req); err != nil {
		WriteResponse(c, err, nil)
		return
	}

	if req.Name == "" {
		WriteResponse(c, errors.New("param  name is empty"), nil)
		return
	}

	ret, err := p.srv.Get(c, &req)
	if err != nil {
		WriteResponse(c, errors.Wrap(err, "add project failed"), nil)
		return
	}
	WriteResponse(c, nil, ret)
}
