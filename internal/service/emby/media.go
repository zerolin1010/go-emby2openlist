package emby

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/https"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/jsons"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/strs"

	"github.com/gin-gonic/gin"
)

// MediaSourceIdSegment 自定义 MediaSourceId 的分隔符
const MediaSourceIdSegment = "[[_]]"

// getEmbyFileLocalPath 获取 Emby 指定媒体的 Path 参数
//
// uri 中必须有 query 参数 MediaSourceId,
// 如果没有携带该参数, 可能会请求到多个媒体, 默认返回第一个媒体的本地路径
func getEmbyFileLocalPath(itemInfo ItemInfo) (string, error) {
	var header http.Header
	switch itemInfo.ApiKeyType {
	case Header:
		// 带上请求头的 api key
		header = http.Header{itemInfo.ApiKeyName: []string{itemInfo.ApiKey}}
	case Query:
		// 如果是 query 格式的 api key, 则往请求头中补充信息
		header = http.Header{HeaderFullAuthName: []string{"Token=" + itemInfo.ApiKey}}
	}

	innerRequest := func(method string) (*http.Response, error) {
		resp, err := https.Request(method, config.C.Emby.Host+itemInfo.PlaybackInfoUri).Header(header).Do()
		if err != nil {
			return nil, fmt.Errorf("请求 Emby 接口异常, error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("请求 Emby 接口异常, status: %s", resp.Status)
		}
		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "application/json") {
			resp.Body.Close()
			return nil, fmt.Errorf("请求 Emby 接口异常, 非 json 响应, contentType: %s", contentType)
		}
		return resp, nil
	}

	// 优先尝试 POST 请求, 响应速度快
	resp, err := innerRequest(http.MethodPost)
	if err != nil {
		resp, err = innerRequest(http.MethodGet)
		if err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()

	type MediaSourcesHolder struct {
		MediaSources []struct {
			Path string
			Id   string
		}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 Emby 响应异常, error: %v", err)
	}
	var holder MediaSourcesHolder
	if err = json.Unmarshal(bodyBytes, &holder); err != nil {
		return "", fmt.Errorf("解析 Emby 响应异常, error: %v, 原始响应: %s", err, string(bodyBytes))
	}

	if len(holder.MediaSources) == 0 {
		return "", fmt.Errorf("获取不到 MediaSources, 原始响应: %v", string(bodyBytes))
	}

	var path string
	var defaultPath string

	reqId := itemInfo.MsInfo.OriginId
	// 获取指定 MediaSourceId 的 Path
	for _, value := range holder.MediaSources {
		if strs.AnyEmpty(defaultPath) {
			// 默认选择第一个路径
			defaultPath = value.Path
		}
		if itemInfo.MsInfo.Empty {
			// 如果没有传递 MediaSourceId, 就使用默认的 Path
			break
		}
		if value.Id == reqId {
			path = value.Path
			break
		}
	}

	if strs.AllNotEmpty(path) {
		return path, nil
	}
	if strs.AllNotEmpty(defaultPath) {
		return defaultPath, nil
	}
	return "", fmt.Errorf("获取不到 Path 参数, 原始响应: %v", string(bodyBytes))
}

// findVideoPreviewInfos 查找 source 的所有转码资源
// 注意：已禁用 OpenList 转码功能
func findVideoPreviewInfos(source *jsons.Item, clientApiKey string, resChan chan []*jsons.Item) {
	if resChan == nil {
		return
	}
	defer close(resChan)

	// 已禁用转码功能，直接返回 nil
	resChan <- nil
}

// addSubtitles2MediaStreams 已禁用：添加转码字幕功能
func addSubtitles2MediaStreams(source *jsons.Item, subtitleList interface{}, openlistPath, templateId, clientApiKey string) {
	// 已禁用转码字幕功能
	return
}

// tryGetVideoStreamInfo 尝试获取 MediaSource 中的视频流信息
func tryGetVideoStreamInfo(source *jsons.Item) (*jsons.Item, bool) {
	if source == nil || source.Type() != jsons.JsonTypeObj {
		return nil, false
	}

	mediaStreams, ok := source.Attr("MediaStreams").Done()
	if !ok || mediaStreams.Type() != jsons.JsonTypeArr {
		return nil, false
	}

	var res *jsons.Item
	mediaStreams.RangeArr(func(_ int, value *jsons.Item) error {
		if value.Attr("Type").Val() == "Video" {
			res = value
			return jsons.ErrBreakRange
		}
		return nil
	})

	if res == nil {
		return nil, false
	}
	return res, true
}

// detectVirtualVideoDisplayTitle 检测虚拟视频媒体的显示名称
func detectVirtualVideoDisplayTitle(source *jsons.Item) {
	if source == nil || source.Type() != jsons.JsonTypeObj {
		return
	}

	vs, ok := tryGetVideoStreamInfo(source)
	if !ok {
		return
	}

	displayTitle, _ := vs.Attr("DisplayTitle").String()
	if displayTitle == "" {
		vs.Put("DisplayTitle", jsons.FromValue("Virtual Media"))
	}
}

// detectSubtitleStreamsDeliveryUrl 强制将外部挂载字幕的访问方式调整为直链访问
func detectSubtitleStreamsDeliveryUrl(source *jsons.Item, apiKey string) {
	if source == nil || source.Type() != jsons.JsonTypeObj {
		return
	}

	// 获取当前播放源的 id 和 itemId
	id, ok := source.Attr("Id").String()
	if !ok {
		return
	}
	itemId, ok := source.Attr("ItemId").String()
	if !ok {
		return
	}

	// 获取媒体流
	mediaStreams, ok := source.Attr("MediaStreams").Done()
	if !ok || mediaStreams.Type() != jsons.JsonTypeArr {
		return
	}

	mediaStreams.RangeArr(func(_ int, value *jsons.Item) error {
		if value.Attr("Type").Val() != "Subtitle" {
			return nil
		}

		// 仅处理外挂字幕
		isExternal, _ := value.Attr("IsExternal").Bool()
		if !isExternal {
			return nil
		}

		// DeliveryMethod 为 External 时, Emby 默认会提供 DeliveryUrl 字段, 无需手动修改
		deliveryMethod, _ := value.Attr("DeliveryMethod").String()
		if deliveryMethod == "External" {
			return nil
		}
		value.Put("DeliveryMethod", jsons.FromValue("External"))

		subIndex, _ := value.Attr("Index").Int()
		u, _ := url.Parse(fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/0/Stream.vtt?api_key=%s", itemId, id, subIndex, apiKey))
		value.Put("DeliveryUrl", jsons.FromValue(u.String()))
		return nil
	})

}

// simplifyMediaName 简化 MediaSource 中的视频名称, 如 '1080p HEVC'
func simplifyMediaName(source *jsons.Item) {
	if source == nil || source.Type() != jsons.JsonTypeObj {
		return
	}

	vs, ok := tryGetVideoStreamInfo(source)
	if !ok {
		return
	}

	displayTitle, _ := vs.Attr("DisplayTitle").String()
	if displayTitle != "" {
		source.Put("Name", jsons.FromValue(displayTitle))
		return
	}

	originName, _ := source.Attr("Name").String()
	reg := regexp.MustCompile(`(?i)S\d+E\d+?`)
	if reg.MatchString(originName) {
		loc := reg.FindStringIndex(originName)
		if len(loc) > 0 {
			newName := originName[loc[0]:]
			source.Put("Name", jsons.FromValue(newName))
		}
	}
}

// resolveItemInfo 解析 emby 资源 item 信息
func resolveItemInfo(c *gin.Context, routeType RouteType) (ItemInfo, error) {
	if c == nil {
		return ItemInfo{}, errors.New("参数 c 不能为空")
	}

	// 匹配 item id
	uri := c.Request.URL.Path
	itemInfo := ItemInfo{RouteType: routeType}
	switch routeType {
	case RouteItems:
		itemInfo.Id = filepath.Base(uri)
	case RoutePlaybackInfo, RouteStream, RouteSyncDownload, RouteTranscode, RouteOriginal:
		itemInfo.Id = filepath.Base(filepath.Dir(uri))
	default:
		return ItemInfo{}, fmt.Errorf("不支持的 RouteType: %s", routeType)
	}

	// 获取客户端请求的 api_key
	itemInfo.ApiKeyType, itemInfo.ApiKeyName, itemInfo.ApiKey = getApiKey(c)

	// 解析请求的媒体信息
	msInfo, err := resolveMediaSourceId(getRequestMediaSourceId(c))
	if err != nil {
		return ItemInfo{}, fmt.Errorf("解析 MediaSource 失败, uri: %s, err: %v", uri, err)
	}
	itemInfo.MsInfo = msInfo

	u, err := url.Parse(fmt.Sprintf("/Items/%s/PlaybackInfo", itemInfo.Id))
	if err != nil {
		return ItemInfo{}, fmt.Errorf("构建 PlaybackInfo uri 失败, err: %v", err)
	}
	q := u.Query()
	// 默认只携带 query 形式的 api key
	if itemInfo.ApiKeyType == Query {
		q.Set(itemInfo.ApiKeyName, itemInfo.ApiKey)
	}
	q.Set("reqformat", "json")
	q.Set("IsPlayback", "false")
	q.Set("AutoOpenLiveStream", "false")
	if !msInfo.Empty {
		q.Set("MediaSourceId", msInfo.OriginId)
	}
	u.RawQuery = q.Encode()
	itemInfo.PlaybackInfoUri = u.String()

	return itemInfo, nil
}

// getRequestMediaSourceId 尝试从请求参数或请求体中获取 MediaSourceId 信息
//
// 优先返回请求参数中的值, 如果两者都获取不到, 就返回空字符串
func getRequestMediaSourceId(c *gin.Context) string {
	if c == nil {
		return ""
	}

	// 1 从请求参数中获取
	q := c.Query("MediaSourceId")
	if strs.AllNotEmpty(q) {
		return q
	}

	// 2 从请求体中获取
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var BodyHolder struct {
		MediaSourceId string
	}
	json.Unmarshal(bodyBytes, &BodyHolder)
	return BodyHolder.MediaSourceId
}

// resolveMediaSourceId 解析 MediaSourceId
func resolveMediaSourceId(id string) (MsInfo, error) {
	res := MsInfo{Empty: true, RawId: id}

	if id == "" {
		return res, nil
	}
	res.Empty = false

	if len(id) <= 32 {
		res.OriginId = id
		return res, nil
	}

	segments := strings.Split(id, MediaSourceIdSegment)

	if len(segments) == 2 {
		res.Transcode = true
		res.OriginId = segments[0]
		res.TemplateId = segments[1]
		return res, nil
	}

	if len(segments) == 4 {
		res.Transcode = true
		res.OriginId = segments[0]
		res.TemplateId = segments[1]
		res.Format = segments[2]
		res.OpenlistPath = segments[3]
		res.SourceNamePrefix = fmt.Sprintf("%s_%s", res.TemplateId, res.Format)
		return res, nil
	}

	return MsInfo{}, errors.New("MediaSourceId 格式错误: " + id)
}

// getAllPreviewTemplateIds 获取所有转码格式
//
// 转码功能已禁用，直接返回空数组
func getAllPreviewTemplateIds() []string {
	return []string{}
}
