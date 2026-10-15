package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestAlignedTemplate(t *testing.T) {
	// 官方 fnpack 生成的 manifest 是「对齐 + = 两侧留空格」的，不能按固定格式切。
	text := "appname               = demo\n" +
		"version               = 1.0.0\n" +
		"platform              = x86\n" +
		"desktop_uidir         = ui\n"

	kv := parseManifest(text)
	for key, want := range map[string]string{
		"appname":       "demo",
		"version":       "1.0.0",
		"platform":      "x86",
		"desktop_uidir": "ui",
	} {
		if got := kv[key]; got != want {
			t.Errorf("%s = %q，期望 %q", key, got, want)
		}
	}
}

func TestParseManifestTripleQuotedMultiline(t *testing.T) {
	// desc 常见写法，第二行开始不能被当成新的键。
	text := "appname=demo\n" +
		"desc=\"\"\"line1\n" +
		"line2\n" +
		"line3\"\"\"\n" +
		"platform=all\n"

	kv := parseManifest(text)
	if got, want := kv["desc"], "line1\nline2\nline3"; got != want {
		t.Errorf("desc = %q，期望 %q", got, want)
	}
	if got, want := kv["platform"], "all"; got != want {
		t.Errorf("platform = %q，期望 %q（多行值吞掉了后面的键）", got, want)
	}
	if len(kv) != 3 {
		t.Errorf("解析出 %d 个键，期望 3 个：%v", len(kv), kv)
	}
}

func TestParseManifestSingleLineQuoting(t *testing.T) {
	text := "desc=\"<p>hi</p>\"\n" +
		"a=`backtick`\n" +
		"b='single'\n"

	kv := parseManifest(text)
	cases := map[string]string{"desc": "<p>hi</p>", "a": "backtick", "b": "single"}
	for key, want := range cases {
		if got := kv[key]; got != want {
			t.Errorf("%s = %q，期望 %q", key, got, want)
		}
	}
}

func TestParseManifestIgnoresNoise(t *testing.T) {
	text := "# 注释行\n" +
		"\n" +
		"   \n" +
		"这行没有等号\n" +
		"appname=demo\n"

	kv := parseManifest(text)
	if len(kv) != 1 {
		t.Fatalf("解析出 %d 个键，期望 1 个：%v", len(kv), kv)
	}
	if kv["appname"] != "demo" {
		t.Errorf("appname = %q", kv["appname"])
	}
}

func TestParseManifestValueKeepsEquals(t *testing.T) {
	// URL 里的 = 不能被当成键值分隔符截断。
	text := "distributor_url=https://example.com/?a=1&b=2\n"
	if got, want := parseManifest(text)["distributor_url"], "https://example.com/?a=1&b=2"; got != want {
		t.Errorf("值被截断: %q", got)
	}
}

func TestManifestValueMissingKey(t *testing.T) {
	if got := ManifestValue("appname=demo\n", "version"); got != "" {
		t.Errorf("缺失的键应返回空串，得到 %q", got)
	}
}

func TestParseManifestHandlesCRLF(t *testing.T) {
	text := "appname=demo\r\nversion=1.0.0\r\n"
	kv := parseManifest(text)
	if kv["appname"] != "demo" || kv["version"] != "1.0.0" {
		t.Errorf("CRLF 未被处理: %v", kv)
	}
}

// 打包前把 manifest 的格式空格收掉（stripManifestSpaces）：值内部的空格、多行值的
// 正文、空行都不能动——判断标准就是「解析出来的键值完全不变」。
func TestStripManifestSpacesKeepsValues(t *testing.T) {
	src := "# 说明：下面这些对齐都是为了好看\n" +
		"appname        =    demo   \n" +
		"display_name   = My App\n" +
		"distributor_url= https://example.com/?a=1&b=2\n" +
		"desc = \"\"\"\n" +
		"  第一行有缩进   \n" +
		"\n" +
		"  第三行 = 这不是键值\n" +
		"\"\"\"\n" +
		"\tcool=yes\n" +
		"只有一行说明\n" +
		"version = 1.0.0 \n"

	got := stripManifestSpaces(src)
	want := "# 说明：下面这些对齐都是为了好看\n" +
		"appname=demo\n" +
		"display_name=My App\n" +
		"distributor_url=https://example.com/?a=1&b=2\n" +
		"desc=\"\"\"\n" +
		"  第一行有缩进   \n" +
		"\n" +
		"  第三行 = 这不是键值\n" +
		"\"\"\"\n" +
		"cool=yes\n" +
		"只有一行说明\n" +
		"version=1.0.0\n"

	if got != want {
		t.Errorf("格式化结果不对：\n got %q\nwant %q", got, want)
	}
	// 判据落到「解析出来的键值」上：格式化只该改书写格式。
	before, after := parseManifest(src), parseManifest(got)
	if len(before) != len(after) {
		t.Fatalf("键的数量变了：%v → %v", before, after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("键 %s 的值被改了：%q → %q", k, v, after[k])
		}
	}
}

// 已经没有多余空格时一个字节都不改（打包前那次重写不该凭空改文件）。
func TestStripManifestSpacesIdempotent(t *testing.T) {
	src := "appname=demo\ndesc=\"\"\"\n带 空格 的正文\n\"\"\"\n"
	if got := stripManifestSpaces(src); got != src {
		t.Errorf("无空格的文件被改了：%q", got)
	}
	// 不以换行结尾的文件也不能被补一个换行（splitLines 末尾那个空元素要留住）。
	noEOL := "appname=demo"
	if got := stripManifestSpaces(noEOL); got != noEOL {
		t.Errorf("结尾换行被补上了：%q", got)
	}
	// CRLF 归一成 LF（和 parseManifest 读文件时的做法一致）。
	if got := stripManifestSpaces("appname = demo\r\n"); got != "appname=demo\n" {
		t.Errorf("CRLF 没被归一：%q", got)
	}
}

// setManifestPlatform 是「按行改 manifest」的那一处，必须和 parseManifest 一样跳过
// `"""` 多行值：desc 正文里出现一行 `platform = x86` 时，改错行的表现是「产物里的
// platform 和用户选的不一致」，而且描述正文被改坏。
func TestSetManifestPlatformSkipsMultilineValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest")
	src := "appname = demo\ndesc = \"\"\"\nplatform = x86 这是描述里的一行\n\"\"\"\nplatform = all\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setManifestPlatform(path, "arm"); err != nil {
		t.Fatalf("setManifestPlatform 失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if !strings.Contains(got, "platform = x86 这是描述里的一行") {
		t.Errorf("描述正文被改坏了：\n%s", got)
	}
	if !strings.Contains(got, "platform=arm") {
		t.Errorf("真正的 platform 行没被改：\n%s", got)
	}
	if strings.Contains(got, "platform = all") {
		t.Errorf("原来的 platform 行还在：\n%s", got)
	}
	if v := parseManifest(got)["platform"]; v != "arm" {
		t.Errorf("解析回来的 platform = %q，期望 arm", v)
	}
	// 其余行一个都不能少。
	for _, want := range []string{"appname = demo", "desc = \"\"\""} {
		if !strings.Contains(got, want) {
			t.Errorf("少了 %q：\n%s", want, got)
		}
	}
}

// 没有 platform 键时补一行，且不能丢掉已有的最后一行（文件不以换行结尾的情况）。
func TestSetManifestPlatformAppendsWithoutDroppingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest")
	src := "appname = demo\nversion = 1.0.0"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setManifestPlatform(path, "x86"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if !strings.Contains(got, "version = 1.0.0") {
		t.Errorf("最后一行被吃掉了：%q", got)
	}
	if !strings.Contains(got, "platform=x86") {
		t.Errorf("没有补上 platform：%q", got)
	}
}
