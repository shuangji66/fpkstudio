package projects

import "testing"

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
