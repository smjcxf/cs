package live

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Ysptp struct {
}

var cache sync.Map

type CacheItem struct {
	playUrl      string
	Expiration   int64
	appRandomStr string
	appSign      string
	urlPath      string
}

func (y *Ysptp) HandleMainRequest(c *gin.Context, vid string) {
	// if !UIDInit {
	// 	GetUIDStatus()
	// 	GetGUID()
	// 	CheckPlayAuth()
	// 	GetAppSecret()
	// }
	_, _, _, _, flag := GetCache("check")
	if !flag {
		CheckPlayAuth()
		GetAppSecret()
		SetCache("check", "", "", "", "")
	}

	// 从 HTTP 请求的查询参数中获取 "uid"，如果未提供则使用默认值 "1234123122"
	//uid := c.DefaultQuery("uid", "1234123122")
	uid := UID

	// 检查全局变量 cctvList 中是否包含指定的 ID
	// 如果找不到对应的 ID，返回 404 错误并终止函数
	if _, ok := CCTVList[vid]; !ok {
		c.String(http.StatusNotFound, "vid not found!") // 返回 404 状态码和错误信息
		return
	}

	// 调用自定义函数 getURL，根据 ID、基础 URL、用户 UID 和 path 获取视频数据
	data, urlPath := getURL(vid, CCTVList[vid], uid)

	// 构建当前请求的主机和路径信息，作为 URL 前缀
	// 示例: 如果请求地址为 http://example.com/path，则 golang = "http://example.com/path"
	golang := "http://" + c.Request.Host + c.Request.URL.Path

	// 使用正则表达式匹配视频数据中的 TS 文件链接
	// `(?i)` 表示忽略大小写，匹配 .ts 文件的路径
	re := regexp.MustCompile(`((?i).*?\.ts)`)

	// 将匹配到的 TS 文件路径替换为新的路径，格式为:
	// 当前请求地址 + "?ts=" + 附加参数 + TS 文件路径
	data = re.ReplaceAllString(data, golang+"?ts="+urlPath+"$1")

	// 设置 HTTP 响应头，用于指定文件下载的名称
	c.Header("Content-Disposition", "attachment;filename="+vid)

	// 返回 HTTP 响应状态码 200 和处理后的视频数据
	c.String(http.StatusOK, data)
}

// 处理 TS 请求，返回 TS 视频流数据
func (y *Ysptp) HandleTsRequest(c *gin.Context, ts, vid string, wsTime string, wsSecret string) {
	_, _, _, _, flag := GetCache("check")
	if !flag {
		CheckPlayAuth()
		GetAppSecret()
		SetCache("check", "", "", "", "")
	}

	// 构建请求数据
	data := ts + "&wsTime=" + wsTime + "&wsSecret=" + wsSecret
	//uid := c.DefaultQuery("uid", "121323241")
	uid := UID
	cacheKey := vid + uid
	_, appSign, appRandomStr, _, found := GetCache(cacheKey)
	if !found {
		log.Println("未知的ts", ts)
		c.String(http.StatusNotFound, "ts not found!")
	}

	// 设置响应头为视频流类型
	c.Header("Content-Type", "video/MP2T")
	// 返回视频数据
	c.String(http.StatusOK, getTs(data, uid, appSign, appRandomStr))
}

// 获取视频 URL，若缓存中存在则直接返回，否则发起请求获取
func getURL(vid, liveID, uid string) (string, string) {
	// 生成缓存键
	cacheKey := vid + uid
	// 查找缓存
	if playURL, appSign, appRandomStr, urlPath, found := GetCache(cacheKey); found {
		// 如果缓存中有，返回缓存中的数据
		//fmt.Println("命中缓存", cacheKey)
		return fetchData(playURL, uid, appSign, appRandomStr, urlPath), urlPath
	}

	baseM3u8Url := GetBaseM3uUrl(liveID)
	if baseM3u8Url == "" {
		log.Println("获取base m3u8地址失败")
		return "", ""
	}

	// POST 数据
	postData := map[string]string{
		"appcommon": `{"adid":"` + uid + `","av":"` + AppVersion + `","an":"央视视频电视投屏助手","ap":"cctv_app_tv"}`,
		"url":       baseM3u8Url,
	}
	// postData := map[string]string{
	// 	"appcommon": `{"adid":"123456","av":"1.3.4","an":"央视视频电视投屏助手","ap":"cctv_app_tv"}`,
	// 	"url":       "http://live-tpgq.cctv.cn/live/3e1b6788736d5a9507c7f9f627ff04f8.m3u8",
	// }

	appRandomStr := uuid.New().String()
	appSignStr := AppId + AppSecret + appRandomStr
	appSign := Md5Encrypt(appSignStr)

	// 创建 POST 请求
	req, _ := http.NewRequest("POST", UrlGetStream, strings.NewReader(EncodeFormData(postData)))
	req.Header.Set("User-Agent", UA)
	req.Header.Set("Referer", Referer)
	req.Header.Set("UID", uid)
	req.Header.Set("APPID", AppId)
	req.Header.Set("APPSIGN", appSign)
	req.Header.Set("APPRANDOMSTR", appRandomStr)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 执行请求并读取响应
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求失败：", err)
		return "", ""
	}
	defer resp.Body.Close()
	var body strings.Builder
	_, _ = io.Copy(&body, resp.Body)

	// fmt.Println("getstream结果：", body.String())

	// 解析 JSON 响应
	var result map[string]interface{}
	json.Unmarshal([]byte(body.String()), &result)
	playURL := result["url"].(string)
	urlPath := ExtractUrlPath(playURL)

	// 将结果缓存起来
	SetCache(cacheKey, playURL, appRandomStr, appSign, urlPath)

	// 返回获取的数据
	return fetchData(playURL, uid, appSign, appRandomStr, urlPath), urlPath
}

// 从指定的播放 URL 获取数据
func fetchData(playURL, uid string, appSign string, appRandomStr, urlPath string) string {

	// 创建一个 HTTP 客户端，用于发起请求
	client := &http.Client{}

	// 无限循环，直到函数返回数据为止
	for {
		// 构造一个 HTTP GET 请求
		// 使用传入的 playURL 作为请求地址，nil 表示不需要请求体
		req, _ := http.NewRequest("GET", playURL, nil)

		// 设置请求头字段，模拟请求来源
		req.Header.Set("User-Agent", UA)
		req.Header.Set("Referer", Referer)
		req.Header.Set("UID", uid)
		req.Header.Set("APPID", AppId)
		req.Header.Set("APPSIGN", appSign)
		req.Header.Set("APPRANDOMSTR", appRandomStr)
		req.Header.Set("Icy-MetaData", "1")
		req.Header.Set("accept", "*/*")
		req.Header.Set("Connection", "keep-alive")

		// 执行请求并获取响应
		resp, err := client.Do(req)
		if err != nil {
			log.Println("请求失败：", err)
			return ""
		}
		// 确保响应体在函数返回之前被正确关闭以释放资源
		defer resp.Body.Close()

		// 使用 strings.Builder 构建响应数据
		var body strings.Builder
		// 将响应体的内容复制到 body 中（忽略错误处理）
		_, _ = io.Copy(&body, resp.Body)

		// 将响应数据转换为字符串
		data := body.String()

		// 使用正则表达式匹配返回数据中的 m3u8 播放链接
		re := regexp.MustCompile(`(.*\.m3u8\?.*)`) // 匹配带有 `.m3u8` 文件及其查询参数的字符串
		matches := re.FindStringSubmatch(data)     // 查找匹配的结果

		// 如果匹配到 m3u8 文件链接
		if len(matches) > 0 {
			// 将 playURL 更新为拼接了 path 和匹配到的链接的新 URL
			playURL = urlPath + matches[0]
		} else {
			// 如果没有匹配到 m3u8 文件链接，直接返回响应数据
			return data
		}
	}
}

// 获取 TS 视频流数据
func getTs(url string, uid string, appSign string, appRandomStr string) string {
	// 创建 GET 请求
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", UA)
	req.Header.Set("Referer", Referer)
	req.Header.Set("UID", uid)
	req.Header.Set("APPID", AppId)
	req.Header.Set("APPSIGN", appSign)
	req.Header.Set("APPRANDOMSTR", appRandomStr)
	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, deflate")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	// 执行请求并读取响应
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求失败：", err)
		return ""
	}
	defer resp.Body.Close()
	var body strings.Builder
	_, _ = io.Copy(&body, resp.Body)

	// 返回响应内容
	return body.String()
}

// 从缓存中获取数据
func GetCache(key string) (string, string, string, string, bool) {
	// 查找缓存
	if item, found := cache.Load(key); found {
		cacheItem := item.(CacheItem)
		// 检查缓存是否过期
		if time.Now().Unix() < cacheItem.Expiration {
			return cacheItem.playUrl, cacheItem.appSign, cacheItem.appRandomStr, cacheItem.urlPath, true
		}
	}
	// 如果没有找到或缓存已过期，返回空
	return "", "", "", "", false
}

// 设置缓存数据
func SetCache(key, playUrl, appRandomStr, appSign, urlPath string) {
	cache.Store(key, CacheItem{
		playUrl:      playUrl,
		Expiration:   time.Now().Unix() + 1700,
		appRandomStr: appRandomStr,
		appSign:      appSign,
		urlPath:      urlPath,
	})
}
