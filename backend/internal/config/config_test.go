package config

import "testing"

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
