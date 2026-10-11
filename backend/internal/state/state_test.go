package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingFileIsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("文件不存在时不应报错: %v", err)
	}
	if !s.Dirs().Empty() {
		t.Errorf("Dirs() = %#v，期望空配置", s.Dirs())
	}
}

func TestSaveThenOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	if err := s.Save(Dirs{ProjectRoot: " /vol1/1000/fpk ", OutputDir: "/vol1/1000/out"}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 路径两边的空白必须去掉：前端从输入框里拿到的值常带空格，带着走会让
	// os.Stat 找不到目录。
	if got := s.Dirs(); got.ProjectRoot != "/vol1/1000/fpk" || got.OutputDir != "/vol1/1000/out" {
		t.Fatalf("Dirs() = %#v，期望去掉首尾空白", got)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("重新打开失败: %v", err)
	}
	if again.Dirs() != s.Dirs() {
		t.Errorf("重新读回 = %#v，期望 %#v", again.Dirs(), s.Dirs())
	}

	// 临时文件不能留下：它和 state.json 在同一个目录，留着会让「数据目录里
	// 多出一个看不懂的文件」。
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("保存后残留了 .tmp 文件")
	}

	// 权限要显式设过：飞牛卷上新建文件可能落成 0000，下次启动就读不回来了。
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件状态失败: %v", err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Errorf("文件权限 = %v，期望 0644", st.Mode().Perm())
	}
}

// 坏 JSON 必须报错，让调用方记日志并回落到默认目录——静默返回空配置会让人
// 以为「设置没生效」，报错至少能在日志里看到原因。
func TestOpenBrokenJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("坏 JSON 应当报错")
	}

	// 但 Store 仍然要可用：覆盖写一次就能恢复正常。
	s := Empty(path)
	if err := s.Save(Dirs{OutputDir: "/vol1/1000/out"}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("重新打开失败: %v", err)
	}
	if again.Dirs().OutputDir != "/vol1/1000/out" {
		t.Errorf("Dirs() = %#v，期望已恢复", again.Dirs())
	}
}
