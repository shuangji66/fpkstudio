package fsx

import "testing"

func TestRelNormalization(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "."},
		{".", "."},
		{"/", "."},
		{"app/server", "app/server"},
		{"./app/server", "app/server"},
		{"/app/server", "app/server"},
		{"app//server/", "app/server"},
		{"app/./server", "app/server"},
		{"a\\b", "a/b"},
	}
	for _, c := range cases {
		got, err := Rel(c.in)
		if err != nil {
			t.Errorf("Rel(%q) 报错: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Rel(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestRelRejectsEscape(t *testing.T) {
	// 这些必须在进入 os.Root 之前就被挡住，不能靠字符串前缀比较。
	for _, in := range []string{
		"..",
		"../etc/passwd",
		"a/../../b",
		"app/../../../etc",
		"a/\x00/b",
	} {
		if got, err := Rel(in); err == nil {
			t.Errorf("Rel(%q) 应当报错，却返回 %q", in, got)
		}
	}
}

func TestAbsStaysInsideRoot(t *testing.T) {
	root := &Root{dir: "/srv/data"}
	if got, want := root.Abs("."), "/srv/data"; got != want {
		t.Errorf("Abs(.) = %q，期望 %q", got, want)
	}
	if got, want := root.Abs("app/server"), "/srv/data/app/server"; got != want {
		t.Errorf("Abs(app/server) = %q，期望 %q", got, want)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:    "0 B",
		999:  "999 B",
		1024: "1.0 KB",
		1536: "1.5 KB",
	}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q，期望 %q", in, got, want)
		}
	}
}
