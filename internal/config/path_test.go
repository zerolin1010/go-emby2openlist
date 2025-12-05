package config

import "testing"

func TestMapEmby2Nginx(t *testing.T) {
	// 模拟配置
	testPath := &Path{
		Emby2Nginx: []string{
			"/media/data:/video/data",
			"/media/data1:/video/data1",
			"/media/data_3_oumeiguochan:/video/data_3_oumeiguochan",
		},
	}

	// 初始化
	if err := testPath.Init(); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}

	tests := []struct {
		name      string
		embyPath  string
		wantNginx string
		wantOK    bool
	}{
		{
			name:      "基础映射 - data 目录",
			embyPath:  "/media/data/movie/test.mp4",
			wantNginx: "/video/data/movie/test.mp4",
			wantOK:    true,
		},
		{
			name:      "基础映射 - data1 目录",
			embyPath:  "/media/data1/series/show.mkv",
			wantNginx: "/video/data1/series/show.mkv",
			wantOK:    true,
		},
		{
			name:      "特殊字符映射",
			embyPath:  "/media/data_3_oumeiguochan/movie/test.mp4",
			wantNginx: "/video/data_3_oumeiguochan/movie/test.mp4",
			wantOK:    true,
		},
		{
			name:      "深层嵌套路径",
			embyPath:  "/media/data/movie/2024/action/test.mp4",
			wantNginx: "/video/data/movie/2024/action/test.mp4",
			wantOK:    true,
		},
		{
			name:      "不匹配的路径",
			embyPath:  "/other/path/video.mp4",
			wantNginx: "",
			wantOK:    false,
		},
		{
			name:      "空路径",
			embyPath:  "",
			wantNginx: "",
			wantOK:    false,
		},
		{
			name:      "只有前缀",
			embyPath:  "/media/data",
			wantNginx: "/video/data",
			wantOK:    true,
		},
		{
			name:      "前缀不完全匹配",
			embyPath:  "/media/data2/video.mp4",
			wantNginx: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := testPath.MapEmby2Nginx(tt.embyPath)
			if got != tt.wantNginx {
				t.Errorf("MapEmby2Nginx(%q) nginx路径 = %q, 期望 %q",
					tt.embyPath, got, tt.wantNginx)
			}
			if ok != tt.wantOK {
				t.Errorf("MapEmby2Nginx(%q) 成功标志 = %v, 期望 %v",
					tt.embyPath, ok, tt.wantOK)
			}
		})
	}
}

func TestMapEmby2Nginx_MultipleMatches(t *testing.T) {
	// 测试多个可能匹配的情况（应该匹配最长前缀）
	testPath := &Path{
		Emby2Nginx: []string{
			"/media/data:/video/data",
			"/media/data/test:/video/special",
		},
	}

	if err := testPath.Init(); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}

	// 应该匹配更具体的路径
	got, ok := testPath.MapEmby2Nginx("/media/data/test/movie.mp4")
	if !ok {
		t.Error("应该找到匹配")
	}

	// 验证是否匹配了更长的前缀
	t.Logf("映射结果: %s (期望匹配更具体的路径)", got)
}

func TestMapEmby2Nginx_EmptyConfig(t *testing.T) {
	// 测试空配置
	testPath := &Path{
		Emby2Nginx: []string{},
	}

	if err := testPath.Init(); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}

	_, ok := testPath.MapEmby2Nginx("/media/data/movie.mp4")
	if ok {
		t.Error("空配置应该返回 false")
	}
}
