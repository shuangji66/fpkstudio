package state

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// 目录设置是「设置弹窗保存」和「启动时读一次」共用的，并发读写必须无竞争
// （`go test -race` 会盯着这里）。
func TestStoreConcurrentSaveAndRead(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = st.Save(Dirs{ProjectRoot: fmt.Sprintf("/vol1/apps/%d", i)})
				_ = st.Dirs()
			}
		}(i)
	}
	wg.Wait()

	if st.Dirs().ProjectRoot == "" {
		t.Error("并发保存之后不该是空设置")
	}
	// 落盘的那一份必须是完整的 JSON（Save 是 tmp + rename）
	again, err := Open(st.Path())
	if err != nil {
		t.Fatalf("重新打开失败（说明落盘的不是合法 JSON）: %v", err)
	}
	if again.Dirs().ProjectRoot == "" {
		t.Error("重新读回来的设置是空的")
	}
}
