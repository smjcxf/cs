package main

import (
	"flag"
	"fmt"
	"ysptp/live"
	"ysptp/m3u"

	"github.com/gin-gonic/gin"
)

var tvM3uObj m3u.Tvm3u
var ysptpObj live.Ysptp

// 设置路由和处理逻辑
func setupRouter() *gin.Engine {
	// 设置Gin为发布模式
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// // 配置HEAD请求的响应
	// r.HEAD("/", func(c *gin.Context) {
	// 	c.String(http.StatusOK, "请求成功！")
	// })

	// // 配置GET请求的响应
	// r.GET("/", func(c *gin.Context) {
	// 	c.String(http.StatusOK, "请求成功！")
	// })

	// 配置获取tv.m3u文件的路由
	r.GET("/", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/octet-stream")
		c.Writer.Header().Set("Content-Disposition", "attachment; filename=tv.m3u")
		tvM3uObj.GetTvM3u(c)
	})

	// 保留其他路径和对象的逻辑
	r.GET("/ysptp/:rid", func(c *gin.Context) {
		rid := c.Param("rid")

		ts := c.Query("ts")
		if ts == "" {
			ysptpObj.HandleMainRequest(c, rid)
		} else {
			ysptpObj.HandleTsRequest(c, ts, rid, c.Query("wsTime"), c.Query("wsSecret"))
		}

	})

	return r
}

func main() {
	host := flag.String("host", "0.0.0.0", "host")
	port := flag.String("p", "8932", "port")

	flag.Parse()
	live.Host = *host
	live.Port = *port

	live.GetUIDStatus()
	live.GetGUID()
	live.CheckPlayAuth()
	live.GetAppSecret()
	live.SetCache("check", "", "", "", "")

	r := setupRouter()

	fmt.Println("Listen on "+*host+":"+*port, "...")
	fmt.Println("现在可以使用浏览器访问 你的ip:" + *port + " 获取tv.m3u文件")
	r.Run(*host + ":" + *port)
}
