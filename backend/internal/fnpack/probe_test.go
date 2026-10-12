package fnpack

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fpkstudio/internal/config"
	"fpkstudio/internal/logging"
)

// Probe 的缓存判据：成功一直缓存（不必每次起一个进程），失败只压一小会儿
// （用户装好 fnpack 之后不用重启应用）。
func TestProbeCachesSuccessButRetriesFailure(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fnpack")
	cfg := &config.Config{FnpackBin: bin}
	log, err := logging.New("", "error", 10)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	c := New(cfg, log)
	ctx := context.Background()

	// 第一次：文件还不存在 → 失败
	if _, _, msg := c.Probe(ctx); msg == "" {
		t.Fatal("fnpack 不存在时应当报不可用")
	}

	// 装好它（假脚本会打印一行带版本号的用法）
	script := "#!/bin/sh\necho 'Usage: fnpack version v9.9.9'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// 失败还在 TTL 内：沿用缓存，不该立刻重探（省进程）
	if _, _, msg := c.Probe(ctx); msg == "" {
		t.Fatal("失败结果应当被缓存一小会儿")
	}

	// TTL 过了：重新探测，这次应当成功
	old := probeFailureTTL
	probeFailureTTL = 0
	defer func() { probeFailureTTL = old }()

	_, version, msg := c.Probe(ctx)
	if msg != "" {
		t.Fatalf("装上之后应当探测成功，得到 %q", msg)
	}
	if version != "9.9.9" {
		t.Errorf("版本号 = %q，期望 9.9.9", version)
	}

	// 成功之后再探也不会重新起进程（把脚本换成会写标记的，标记不该出现）
	marker := filepath.Join(dir, "ran")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, v2, _ := c.Probe(ctx); v2 != "9.9.9" {
		t.Errorf("成功结果应当被缓存，得到 %q", v2)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("成功之后又起了一次 fnpack 进程")
	}
	_ = time.Now()
}
