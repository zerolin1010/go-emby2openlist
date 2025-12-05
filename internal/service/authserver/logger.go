package authserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
)

// AccessLogger 访问日志记录器
type AccessLogger struct {
	logFile   *os.File
	logPath   string
	stats     *Stats
	mu        sync.Mutex
	bufferCh  chan AccessLog
	closeCh   chan struct{}
	enableLog bool
}

// NewAccessLogger 创建访问日志记录器
func NewAccessLogger(logPath string, enableLog bool) (*AccessLogger, error) {
	logger := &AccessLogger{
		logPath:   logPath,
		enableLog: enableLog,
		stats: &Stats{
			FailReasons: make(map[string]int64),
			TopUsers:    make([]UserStats, 0),
		},
		bufferCh: make(chan AccessLog, 1000),
		closeCh:  make(chan struct{}),
	}

	if enableLog {
		// 确保日志目录存在
		dir := filepath.Dir(logPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败: %v", err)
		}

		// 打开日志文件（追加模式）
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件失败: %v", err)
		}
		logger.logFile = file
	}

	// 启动日志写入协程
	go logger.writeLoop()

	// 启动统计清理协程
	go logger.cleanupLoop()

	logs.Success("访问日志记录器已启动，日志文件: %s", logPath)
	return logger, nil
}

// Log 记录访问日志（异步）
func (l *AccessLogger) Log(log AccessLog) {
	select {
	case l.bufferCh <- log:
	default:
		logs.Warn("日志缓冲区已满，丢弃日志")
	}
}

// writeLoop 日志写入循环
func (l *AccessLogger) writeLoop() {
	for {
		select {
		case log := <-l.bufferCh:
			l.writeLog(log)
			l.updateStats(log)
		case <-l.closeCh:
			// 处理剩余日志
			for len(l.bufferCh) > 0 {
				log := <-l.bufferCh
				l.writeLog(log)
				l.updateStats(log)
			}
			return
		}
	}
}

// writeLog 写入单条日志
func (l *AccessLogger) writeLog(log AccessLog) {
	if !l.enableLog || l.logFile == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// JSON 格式
	data, err := json.Marshal(log)
	if err != nil {
		logs.Error("序列化日志失败: %v", err)
		return
	}

	// 写入文件
	if _, err := l.logFile.Write(append(data, '\n')); err != nil {
		logs.Error("写入日志失败: %v", err)
	}
}

// updateStats 更新统计信息
func (l *AccessLogger) updateStats(log AccessLog) {
	l.stats.mu.Lock()
	defer l.stats.mu.Unlock()

	l.stats.TotalRequests++

	if log.AuthResult == "success" {
		l.stats.SuccessRequests++
	} else {
		l.stats.FailedRequests++
		if log.ErrorReason != "" {
			l.stats.FailReasons[log.ErrorReason]++
		}
	}

	// 更新每小时统计
	l.stats.LastHourStats.Requests++
	if log.AuthResult == "success" {
		l.stats.LastHourStats.Success++
	} else {
		l.stats.LastHourStats.Failed++
	}

	// 更新用户统计（简化版，只记录最近的用户）
	if log.ApiKey != "" {
		found := false
		for i, user := range l.stats.TopUsers {
			if user.ApiKey == log.ApiKey {
				l.stats.TopUsers[i].Requests++
				l.stats.TopUsers[i].LastSeen = log.Timestamp
				found = true
				break
			}
		}
		if !found && len(l.stats.TopUsers) < 100 {
			l.stats.TopUsers = append(l.stats.TopUsers, UserStats{
				ApiKey:   log.ApiKey,
				Requests: 1,
				LastSeen: log.Timestamp,
			})
		}
	}
}

// cleanupLoop 定期清理统计数据
func (l *AccessLogger) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.resetHourlyStats()
		case <-l.closeCh:
			return
		}
	}
}

// resetHourlyStats 重置每小时统计
func (l *AccessLogger) resetHourlyStats() {
	l.stats.mu.Lock()
	defer l.stats.mu.Unlock()

	l.stats.LastHourStats = HourlyStats{}
}

// GetStats 获取统计信息
func (l *AccessLogger) GetStats() *Stats {
	l.stats.mu.RLock()
	defer l.stats.mu.RUnlock()

	// 返回副本
	stats := &Stats{
		TotalRequests:   l.stats.TotalRequests,
		SuccessRequests: l.stats.SuccessRequests,
		FailedRequests:  l.stats.FailedRequests,
		FailReasons:     make(map[string]int64),
		LastHourStats:   l.stats.LastHourStats,
		TopUsers:        make([]UserStats, len(l.stats.TopUsers)),
	}

	for k, v := range l.stats.FailReasons {
		stats.FailReasons[k] = v
	}

	copy(stats.TopUsers, l.stats.TopUsers)

	return stats
}

// Close 关闭日志记录器
func (l *AccessLogger) Close() error {
	close(l.closeCh)

	// 等待日志写入完成
	time.Sleep(100 * time.Millisecond)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logFile != nil {
		if err := l.logFile.Close(); err != nil {
			return fmt.Errorf("关闭日志文件失败: %v", err)
		}
	}

	logs.Success("访问日志记录器已关闭")
	return nil
}

// Rotate 日志轮转
func (l *AccessLogger) Rotate() error {
	if !l.enableLog || l.logFile == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭当前文件
	if err := l.logFile.Close(); err != nil {
		return fmt.Errorf("关闭日志文件失败: %v", err)
	}

	// 重命名旧文件
	timestamp := time.Now().Format("20060102-150405")
	oldPath := l.logPath
	newPath := fmt.Sprintf("%s.%s", l.logPath, timestamp)

	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("重命名日志文件失败: %v", err)
	}

	// 打开新文件
	file, err := os.OpenFile(oldPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开新日志文件失败: %v", err)
	}

	l.logFile = file
	logs.Success("日志文件已轮转: %s -> %s", oldPath, newPath)
	return nil
}
