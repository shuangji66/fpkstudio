package fnpack

import "testing"

func TestFailureReason(t *testing.T) {
	// 真实的 create 输出：失败时退出码是 0，只有这一行能说明问题。
	real := `fnpack create
Create application name ab error : appname ilegal,app name (ab) length must be between 3 and 32
`
	if got, want := FailureReason(real), "Create application name ab error : appname ilegal,app name (ab) length must be between 3 and 32"; got != want {
		t.Errorf("FailureReason() = %q，期望 %q", got, want)
	}

	ok := `fnpack create

Success! Created 'myapp' in the directory ./myapp
Here are some tips you may need:
1. manifest – Configure your application's metadata (e.g., appname, version, dependencies).
`
	if got := FailureReason(ok); got != "" {
		t.Errorf("成功输出里不该判出失败原因，得到 %q", got)
	}
}
