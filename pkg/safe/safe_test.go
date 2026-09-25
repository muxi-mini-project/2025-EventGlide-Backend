package safe

import (
	"sync"
	"testing"

	"go.uber.org/zap"
)

// panic 必须被 recover 吞掉，不向上冒泡。
func TestRunRecoversPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked out of Run: %v", r)
		}
	}()

	Run(zap.NewNop(), "test", func() {
		panic("boom")
	})
}

// Go 里的 panic 同样不能终止进程（这里以能正常返回验证）。
func TestGoRecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	Go(zap.NewNop(), "test", func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait()
}

// 无 panic 时 Run 正常执行并返回。
func TestRunExecutesFn(t *testing.T) {
	ran := false
	Run(zap.NewNop(), "test", func() {
		ran = true
	})
	if !ran {
		t.Fatal("fn was not executed")
	}
}
