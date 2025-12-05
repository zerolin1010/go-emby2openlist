package userkey

import (
	"fmt"
	"net/http"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/https"
)

// Fetcher 用户 Key 获取器
type Fetcher struct {
	embyHost    string
	adminApiKey string
}

// NewFetcher 创建获取器
func NewFetcher(cfg *config.Emby) *Fetcher {
	return &Fetcher{
		embyHost:    cfg.Host,
		adminApiKey: cfg.AdminApiKey,
	}
}

// ValidateApiKey 验证 API Key 是否有效
func (f *Fetcher) ValidateApiKey(apiKey string) (bool, error) {
	url := fmt.Sprintf("%s/emby/System/Info?api_key=%s", f.embyHost, apiKey)

	resp, err := https.Get(url).Do()
	if err != nil {
		return false, fmt.Errorf("验证 API Key 失败: %v", err)
	}
	defer resp.Body.Close()

	// 401 表示无效
	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	return resp.StatusCode == http.StatusOK, nil
}
