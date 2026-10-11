package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                 "/",
		"/":                "/",
		"app/fpkstudio":    "/app/fpkstudio",
		"/app/fpkstudio":   "/app/fpkstudio",
		"/app/fpkstudio/":  "/app/fpkstudio",
		"//app//fpkstudio": "/app/fpkstudio",
		"/app/fpkstudio//": "/app/fpkstudio",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// shortTempDir 给数据目录用：t.TempDir() 会把测试名拼进路径，名字长一点的测试
// 会让 socket 撞上 sun_path 上限，报一个和测试内容毫无关系的错误。
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cfg")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// 项目目录现在也能由环境变量给出：没设置时回退到数据目录下的 projects/。
func TestProjectsDirFromEnv(t *testing.T) {
	dataDir := shortTempDir(t)

	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvProjectsDir, "")
	if c, err := Load("test"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	} else if c.ProjectsDir != filepath.Join(c.DataDir, "projects") {
		t.Errorf("ProjectsDir = %q，期望回退到 %s", c.ProjectsDir, filepath.Join(c.DataDir, "projects"))
	}

	outside := t.TempDir()
	t.Setenv(EnvProjectsDir, outside)
	if c, err := Load("test"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	} else if c.ProjectsDir != outside {
		t.Errorf("ProjectsDir = %q，期望 %q", c.ProjectsDir, outside)
	}
}

// 两个目录撞在一起时保项目目录：输出目录退回默认值，并标记「环境变量被忽略」。
func TestOutputDirFallsBackWhenSameAsProjects(t *testing.T) {
	dataDir := shortTempDir(t)
	same := shortTempDir(t)

	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvProjectsDir, same)
	t.Setenv(EnvOutputDir, same)

	c, err := Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.ProjectsDir != same {
		t.Errorf("ProjectsDir = %q，期望保项目目录 %q", c.ProjectsDir, same)
	}
	if want := filepath.Join(dataDir, "output"); c.OutputDir != want {
		t.Errorf("OutputDir = %q，期望回退到 %q", c.OutputDir, want)
	}
	if !c.OutputEnvIgnored {
		t.Error("OutputEnvIgnored 应当为真，界面要据此标注")
	}

	// 只设输出目录、项目目录用默认值时不该误判。
	t.Setenv(EnvProjectsDir, "")
	c, err = Load("test")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if c.OutputEnvIgnored {
		t.Error("两个目录不同时不应当标记忽略")
	}
}
