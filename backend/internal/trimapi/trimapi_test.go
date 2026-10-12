package trimapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fpkstudio/internal/logging"
)

// fakeGateway 起一个只监听 unix socket 的假网关，行为和开放 API 一致：
// POST /api/v1/trimapp + Bearer token + {reqId, req, appName, data}。
func fakeGateway(t *testing.T, handle func(req apiRequest, token string) (int, apiResponse)) (socket string, c *Client) {
	t.Helper()

	// 不能用 t.TempDir()：它把测试名拼进路径，中文子测试名会让 socket 路径超出
	// sun_path 上限，报一个和长度毫无关系的 "bind: invalid argument"。
	dir, err := os.MkdirTemp("", "trimapi")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socket = filepath.Join(dir, "trim.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("监听 unix socket 失败: %v", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/trimapp" {
			t.Errorf("请求路径 = %q，期望 /api/v1/trimapp", r.URL.Path)
		}
		var req apiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析请求体失败: %v", err)
		}
		token := ""
		if auth := r.Header.Get("Authorization"); len(auth) > 7 {
			token = auth[7:]
		}
		status, resp := handle(req, token)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	t.Setenv(EnvToken, "test-token")
	return socket, New(socket, "fpkstudio", log)
}

// 请求体、鉴权头、返回数据的解析：任何一环错了，前端拿到的都是「接口不可用」，
// 而真实环境里根本没有这台假网关可对着调，所以这里必须盯住。
func TestCallSendsExpectedRequest(t *testing.T) {
	var got apiRequest
	socket, c := fakeGateway(t, func(req apiRequest, token string) (int, apiResponse) {
		if token != "test-token" {
			t.Errorf("token = %q，期望 test-token", token)
		}
		got = req
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`{"paths":["/vol1/1000/photo"]}`)}
	})

	paths, err := c.GetUserAccessibleFolders(context.Background(), 1000)
	if err != nil {
		t.Fatalf("查询授权目录失败: %v", err)
	}
	if got.AppName != "fpkstudio" {
		t.Errorf("appName = %q，期望 fpkstudio", got.AppName)
	}
	if got.Req != ReqGetUserAccessibleFolders {
		t.Errorf("req = %q，期望 %q", got.Req, ReqGetUserAccessibleFolders)
	}
	// uid 必须是数字，不能是字符串："1000" 会被系统当成非法参数。
	if data, ok := got.Data.(map[string]any); !ok || data["uid"] != float64(1000) {
		t.Errorf("data = %#v，期望 uid 为数字 1000", got.Data)
	}
	if len(paths) != 1 || paths[0] != "/vol1/1000/photo" {
		t.Errorf("paths = %#v，期望 [/vol1/1000/photo]", paths)
	}
	if c.Socket() != socket {
		t.Errorf("Socket() = %q，期望 %q", c.Socket(), socket)
	}
}

// 业务码非 0 必须变成错误，并且要能拿到 code——code 1000002 是「Scope 不足」，
// 上层要靠它给出可操作提示。
func TestCallReturnsAPIError(t *testing.T) {
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		return http.StatusForbidden, apiResponse{Code: 1000002, Msg: "管理员权限或 API Scope 不足"}
	})

	err := c.DelUserAccessibleFolder(context.Background(), 1000, "/vol1/1000/photo")
	if err == nil {
		t.Fatal("期望报错，实际成功")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("错误类型 = %T，期望 *APIError", err)
	}
	if apiErr.Code != 1000002 || apiErr.HTTP != http.StatusForbidden {
		t.Errorf("code/http = %d/%d，期望 1000002/403", apiErr.Code, apiErr.HTTP)
	}
}

// 即使 HTTP 200，只要 code 非 0 也算失败（文档里用 200 返回业务错误）。
func TestCallTreatsNonZeroCodeAsFailure(t *testing.T) {
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		return http.StatusOK, apiResponse{Code: 1003201, Msg: "管理员已关闭该应用的普通用户授权能力"}
	})
	if _, err := c.GetUserAccessibleFolders(context.Background(), 1000); err == nil {
		t.Fatal("code 非 0 时不应成功")
	}
}

func TestCheckUserACLAndConvertPath(t *testing.T) {
	_, c := fakeGateway(t, func(req apiRequest, _ string) (int, apiResponse) {
		data, _ := req.Data.(map[string]any)
		switch req.Req {
		case ReqCheckUserACL:
			// path 一律传数组：单路径也传数组，省得两套分支。
			if _, ok := data["path"].([]any); !ok {
				t.Errorf("checkUserACL 的 path = %#v，期望数组", data["path"])
			}
			return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(
				`[{"path":"/vol1/1000/fpk","readable":true,"writable":false,"deletable":false}]`)}
		case ReqConvertPath:
			if data["language"] != "zh-CN" {
				t.Errorf("language = %#v，期望 zh-CN", data["language"])
			}
			return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(
				`{"status":0,"result":[{"path":"/vol1/1000/fpk","semanticPath":"存储空间1/admin 的文件/fpk"}]}`)}
		}
		t.Errorf("意外的 req: %s", req.Req)
		return http.StatusNotFound, apiResponse{Code: 200005, Msg: "Not Found"}
	})

	acls, err := c.CheckUserACL(context.Background(), 1000, []string{"/vol1/1000/fpk"})
	if err != nil {
		t.Fatalf("检查权限失败: %v", err)
	}
	if len(acls) != 1 || !acls[0].Readable || acls[0].Writable {
		t.Errorf("acl = %#v，期望可读不可写", acls)
	}

	semantic, err := c.ConvertPath(context.Background(), []string{"/vol1/1000/fpk"}, "zh-CN")
	if err != nil {
		t.Fatalf("转换路径失败: %v", err)
	}
	if len(semantic) != 1 || semantic[0].SemanticPath == "" {
		t.Errorf("semantic = %#v，期望有 semanticPath", semantic)
	}
}

// convertPath 的 data 在不同系统版本上形态不同：文档写的是 {status,result}，
// 实测的系统直接回数组。三种都要认，否则用户看到的只是「界面退回显示 /vol1/...」。
func TestConvertPathAcceptsEveryShape(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"文档形态：{status,result}", `{"status":0,"result":[{"path":"/vol1/1000/fpk","semanticPath":"存储空间1/fpk"}]}`, "存储空间1/fpk"},
		{"实测形态：数组", `[{"path":"/vol1/1000/fpk","semanticPath":"存储空间1/fpk"}]`, "存储空间1/fpk"},
		{"数组元素是纯字符串", `["存储空间1/fpk"]`, "存储空间1/fpk"},
		{"空数组", `[]`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, client := fakeGateway(t, func(req apiRequest, _ string) (int, apiResponse) {
				return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(c.data)}
			})
			got, err := client.ConvertPath(context.Background(), []string{"/vol1/1000/fpk"}, "zh-CN")
			if err != nil {
				t.Fatalf("转换路径失败: %v", err)
			}
			if len(got) == 0 {
				if c.want != "" {
					t.Fatalf("结果为空，期望 %q", c.want)
				}
				return
			}
			if got[0].SemanticPath != c.want {
				t.Errorf("semanticPath = %q，期望 %q", got[0].SemanticPath, c.want)
			}
			// 纯字符串形态也要把请求过的路径补回来，前端才按得上号。
			if got[0].Path != "/vol1/1000/fpk" {
				t.Errorf("path = %q，期望回填请求路径", got[0].Path)
			}
		})
	}
}

// 文档形态里的 status 非 0 是失败，不能当成「没转换成功但没报错」。
func TestConvertPathStatusFailure(t *testing.T) {
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`{"status":7,"result":[]}`)}
	})
	if _, err := c.ConvertPath(context.Background(), []string{"/vol1/x"}, "zh-CN"); err == nil {
		t.Fatal("status 非 0 应当报错")
	}
}

// 两种形态都不是时给一句人话，别只说「格式不符合预期」。
func TestConvertPathUnknownShape(t *testing.T) {
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`"nope"`)}
	})
	_, err := c.ConvertPath(context.Background(), []string{"/vol1/x"}, "zh-CN")
	if err == nil {
		t.Fatal("预期之外的数据应当报错")
	}
	if !strings.Contains(err.Error(), "预期之外") {
		t.Errorf("错误信息 = %q，期望说明数据形态不对", err)
	}
}

// 路径转换不带 language 时兜一个默认值：这个参数是必填的，缺了接口会直接报错。
func TestConvertPathDefaultsLanguage(t *testing.T) {
	_, c := fakeGateway(t, func(req apiRequest, _ string) (int, apiResponse) {
		data, _ := req.Data.(map[string]any)
		if data["language"] == "" {
			t.Errorf("language 不能为空")
		}
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`{"status":0,"result":[]}`)}
	})
	if _, err := c.ConvertPath(context.Background(), []string{"/vol1/1000"}, ""); err != nil {
		t.Fatalf("转换路径失败: %v", err)
	}
}

// 没有 token 时不能真发请求，而要报 ErrUnavailable——上层据此回 503 并给出
// 「请从飞牛应用中心启动」这类提示。
func TestCallWithoutTokenIsUnavailable(t *testing.T) {
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		t.Error("没有 token 时不应该发出请求")
		return http.StatusOK, apiResponse{}
	})
	t.Setenv(EnvToken, "")

	_, err := c.GetUserAccessibleFolders(context.Background(), 1000)
	if !IsUnavailable(err) {
		t.Fatalf("错误 = %v，期望 ErrUnavailable", err)
	}
}

func TestProbeReportsMissingSocket(t *testing.T) {
	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	defer log.Close()

	c := New(filepath.Join(os.TempDir(), "nope.sock"), "fpkstudio", log)
	t.Setenv(EnvToken, "test-token")
	if st := c.Probe(context.Background()); st.Available {
		t.Errorf("socket 不存在时不应可用：%#v", st)
	}
}

// 空 socket 要落回系统默认路径：部署时没人会显式指定它。
func TestNewDefaultsSocket(t *testing.T) {
	if got := New("", "", nil).Socket(); got != DefaultSocket {
		t.Errorf("socket = %q，期望 %q", got, DefaultSocket)
	}
	if got := New("", "", nil).AppName(); got != "fpkstudio" {
		t.Errorf("appName = %q，期望 fpkstudio", got)
	}
}

// DelUserAccessibleFolder：成功要真的调对 req、带上 uid 与 path；
// 系统回 suc=false 时也要算失败（否则界面会以为旧授权删掉了）。
func TestDelUserAccessibleFolder(t *testing.T) {
	var got apiRequest
	_, c := fakeGateway(t, func(req apiRequest, _ string) (int, apiResponse) {
		got = req
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`{"suc":true}`)}
	})

	if err := c.DelUserAccessibleFolder(context.Background(), 1000, "/vol1/1000/photo"); err != nil {
		t.Fatalf("删除授权目录失败: %v", err)
	}
	if got.Req != ReqDelUserAccessibleFolder {
		t.Errorf("req = %q，期望 %q", got.Req, ReqDelUserAccessibleFolder)
	}
	data, ok := got.Data.(map[string]any)
	if !ok || data["uid"] != float64(1000) || data["path"] != "/vol1/1000/photo" {
		t.Errorf("data = %#v", got.Data)
	}

	// suc=false：系统没有确认删除，必须报错
	_, c2 := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`{"suc":false}`)}
	})
	if err := c2.DelUserAccessibleFolder(context.Background(), 1000, "/vol1/1000/photo"); err == nil {
		t.Fatal("suc=false 应当报错")
	}
}

// CheckUserACL：空路径不发请求；返回的裸数组（实测形态）要能解出来。
func TestCheckUserACLEdgeCases(t *testing.T) {
	called := false
	_, c := fakeGateway(t, func(apiRequest, string) (int, apiResponse) {
		called = true
		return http.StatusOK, apiResponse{Code: 0, Data: json.RawMessage(`[{"path":"/a","readable":true}]`)}
	})

	if list, err := c.CheckUserACL(context.Background(), 1000, nil); err != nil || len(list) != 0 {
		t.Fatalf("空路径应当直接回空列表: %v %v", list, err)
	}
	if called {
		t.Error("空路径不该发请求")
	}

	list, err := c.CheckUserACL(context.Background(), 1000, []string{"/a"})
	if err != nil || len(list) != 1 || !list[0].Readable {
		t.Fatalf("解析裸数组失败: %+v %v", list, err)
	}
}
