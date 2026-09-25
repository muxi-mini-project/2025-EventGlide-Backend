package safe

import "go.uber.org/zap"

// Go 启动一个带 panic 兜底的后台 goroutine，用于一次性的异步任务。
// name 用于日志定位；panic 时记录堆栈后结束该 goroutine，避免拖垮整个进程。
func Go(l *zap.Logger, name string, fn func()) {
	go func() {
		defer Recover(l, name)
		fn()
	}()
}

// Run 执行 fn 并兜底 panic，供长驻循环在每轮调用，使单轮 panic 不终止整个循环。
func Run(l *zap.Logger, name string, fn func()) {
	defer Recover(l, name)
	fn()
}

// Recover 捕获当前 goroutine 的 panic 并记录堆栈，供 defer 调用。
func Recover(l *zap.Logger, name string) {
	r := recover()
	if r == nil || l == nil {
		return
	}
	l.Error("background goroutine panicked",
		zap.String("task", name),
		zap.Any("panic", r),
		zap.Stack("stack"),
	)
}
