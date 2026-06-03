package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/filter/internal/api/config"
	"github.com/mapprotocol/filter/internal/api/handler"
	"github.com/mapprotocol/filter/internal/api/store/mysql"
	"github.com/pkg/errors"
)

func initMiddleware(g *gin.Engine) {
	g.Use(cors.New(cors.Config{
		AllowOriginFunc:  func(origin string) bool { return true },
		AllowMethods:     []string{"OPTIONS", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-API-Key"},
		AllowCredentials: true,
		MaxAge:           2 * time.Minute,
	}))
}

func initController(g *gin.Engine, cfg *config.Config) error {
	db, err := mysql.Init(cfg.Dsn)
	if err != nil {
		return errors.Wrap(err, "init db failed")
	}
	v1 := g.Group("/v1", apiAuthMiddleware(cfg.APIAuthKeys, cfg.IPWhitelist))
	{
		pro := handler.NewProject(db)
		group := v1.Group("project")
		group.GET("", pro.Get)
		group.POST("", pro.Add)
	}
	{
		event := handler.NewEvent(db)
		group := v1.Group("event")
		group.GET("", event.Get)
		group.POST("", event.Add)
		group.DELETE("", event.Delete)
		group.GET("/list", event.List)
	}
	{
		mos := handler.NewMos(db)
		group := v1.Group("mos")
		group.GET("/list", mos.List)
		group.GET("/max/id", mos.MaxID)
		group.GET("/block/list", mos.BlockList)
	}
	{
		b := handler.NewBlock(db)
		group := v1.Group("block")
		group.GET("", b.Get)
		group.GET("/scan", b.GetCurrentScan)
	}
	return nil
}

func apiAuthMiddleware(authKeys []string, whitelist []string) gin.HandlerFunc {
	ipWhitelist := newIPWhitelist(whitelist)
	return func(c *gin.Context) {
		if ipWhitelist.allow(clientIP(c.Request)) {
			c.Next()
			return
		}
		if validAPIKey(apiKey(c.Request), authKeys) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "unauthorized",
		})
	}
}

func validAPIKey(got string, authKeys []string) bool {
	if got == "" {
		return false
	}
	for _, want := range authKeys {
		if constantTimeEqual(got, strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

type ipWhitelist struct {
	ips  map[string]struct{}
	nets []*net.IPNet
}

func newIPWhitelist(values []string) *ipWhitelist {
	ret := &ipWhitelist{ips: make(map[string]struct{})}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, cidr, err := net.ParseCIDR(value); err == nil {
			ret.nets = append(ret.nets, cidr)
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			ret.ips[ip.String()] = struct{}{}
		}
	}
	return ret
}

func (w *ipWhitelist) allow(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if _, ok := w.ips[parsed.String()]; ok {
		return true
	}
	for _, cidr := range w.nets {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if ip := strings.TrimSpace(strings.Split(forwarded, ",")[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func apiKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const bearer = "Bearer "
	if len(auth) >= len(bearer) && strings.EqualFold(auth[:len(bearer)], bearer) {
		return strings.TrimSpace(auth[len(bearer):])
	}
	return ""
}

func constantTimeEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
