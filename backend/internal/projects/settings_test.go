package projects

import (
	"encoding/json"
	"strings"
	"testing"
)

// 前端会把 settings.platforms 当数组用（展开、遍历）。Go 的 nil slice 编码出来是
// `null`，前端 `[...null]` 会直接抛异常——表现是「进项目后设置读不出来，页面
// 从中间断掉」。所以这里盯住「永远不是 null」这个约定。
func TestNormalizeSettingsNeverEncodesNullPlatforms(t *testing.T) {
	for _, in := range []Settings{
		{},
		{Platforms: nil},
		{Platforms: []string{}},
		{Platforms: []string{"arm"}},
	} {
		data, err := json.Marshal(normalizeSettings(in))
		if err != nil {
			t.Fatalf("序列化失败: %v", err)
		}
		if strings.Contains(string(data), `"platforms":null`) {
			t.Errorf("platforms 被编码成 null: %s", data)
		}
	}
}

func TestNormalizeSettingsKeepsPlatforms(t *testing.T) {
	got := normalizeSettings(Settings{Platforms: []string{"arm", "x86"}})
	if len(got.Platforms) != 2 || got.Platforms[0] != "arm" {
		t.Errorf("platforms 被改动: %#v", got.Platforms)
	}
}
