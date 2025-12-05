package logs

import (
	"fmt"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs/colors"
)

// Info 输出蓝色 Info 日志
func Info(format string, v ...any) {
	s := fmt.Sprintf("[INFO] "+format, v...)
	fmt.Println(now() + colors.ToBlue(s))
}

// Success 输出绿色 Success 日志
func Success(format string, v ...any) {
	s := fmt.Sprintf("[SUCCESS] "+format, v...)
	fmt.Println(now() + colors.ToGreen(s))
}

// Warn 输出黄色 Warn 日志
func Warn(format string, v ...any) {
	s := fmt.Sprintf("[WARN] "+format, v...)
	fmt.Println(now() + colors.ToYellow(s))
}

// Error 输出红色 Error 日志
func Error(format string, v ...any) {
	s := fmt.Sprintf("[ERROR] "+format, v...)
	fmt.Println(now() + colors.ToRed(s))
}

// Tip 输出灰色 Tip 日志
func Tip(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	fmt.Println(now() + colors.ToGray(s))
}

// Progress 输出紫色 Progress 日志
func Progress(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	fmt.Println(now() + colors.ToPurple(s))
}

// now 返回当前时间戳
func now() string {
	return time.Now().Format("2006-01-02 15:04:05") + " "
}
