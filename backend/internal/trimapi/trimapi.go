// Package trimapi 调用飞牛 fnOS 的开放 API（trim.file.*）。
//
// 这一层只负责「怎么调」——固定走 Unix Socket、固定用 POST /api/v1/trimapp，
// 业务语义（授权目录、ACL、语义路径）由上层组合。几处刻意的约束，都来自官方
// 文档（/api/calling/）：
//
//   - **只能服务端调**。后端 API 不暴露给浏览器，token 也不能下发到前端。
//     前端要什么，经 server 的 /api/trim/* 转一手。
//   - **token 每次从环境变量读**。系统拉起应用脚本时把 token 写进
//     TRIM_API_TOKEN，重新注册 / 重装 / 运行环境变化后都会换新值，持久化下来
//     只会拿到过期的那个。
//   - **socket 路径固定**。/var/run/trim_open_gateway_apiscope.socket 是系统提供的
//     网关 socket；这里允许用环境变量覆盖，是为了在没有 fnOS 的环境里跑通链路。
package trimapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"fpkstudio/internal/logging"
)

// DefaultSocket 是系统提供的开放 API 网关 socket。
const DefaultSocket = "/var/run/trim_open_gateway_apiscope.socket"

// EnvToken 是系统注入的访问令牌所在的环境变量。
const EnvToken = "TRIM_API_TOKEN"

// 后端能力的标识（请求体里的 req）。
const (
	ReqGetUserAccessibleFolders = "trim.file.getUserAccessibleFolders"
	ReqDelUserAccessibleFolder  = "trim.file.delUserAccessibleFolder"
	ReqCheckUserACL             = "trim.file.checkUserACL"
	ReqConvertPath              = "trim.file.convertPath"
)

// ErrUnavailable 表示当前进程没有跑在 fnOS 的网关环境里（缺 socket 或缺 token）。
var ErrUnavailable = errors.New("当前应用不在飞牛系统的网关环境中运行")

// APIError 是开放 API 返回的业务错误（code 非 0）。
type APIError struct {
	Code int
	Msg  string
	// HTTP 是响应状态码，0 表示请求没能发出去。
	HTTP int
}

func (e *APIError) Error() string {
	msg := e.Msg
	if msg == "" {
		msg = "未提供错误信息"
	}
	if e.HTTP != 0 && e.HTTP != http.StatusOK {
		return fmt.Sprintf("飞牛接口调用失败（HTTP %d，code %d）：%s", e.HTTP, e.Code, msg)
	}
	return fmt.Sprintf("飞牛接口调用失败（code %d）：%s", e.Code, msg)
}

// IsUnavailable 判断错误是否属于「不在这套环境下」。
func IsUnavailable(err error) bool { return errors.Is(err, ErrUnavailable) }

// Client 是开放 API 的客户端。
type Client struct {
	socket  string
	appName string
	log     *logging.Logger
	http    *http.Client
}

// New 构造客户端。socket 为空时用 DefaultSocket。
func New(socket, appName string, log *logging.Logger) *Client {
	if strings.TrimSpace(socket) == "" {
		socket = DefaultSocket
	}
	if strings.TrimSpace(appName) == "" {
		appName = "fpkstudio"
	}
	return &Client{
		socket:  socket,
		appName: appName,
		log:     log,
		http: &http.Client{
			// socket 是本机的，正常都是毫秒级；给足超时避免网关挂住时把
			// 前端的设置弹窗一直吊着。
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

// Socket 返回当前使用的 socket 路径。
func (c *Client) Socket() string { return c.socket }

// AppName 返回请求里带的应用名。
func (c *Client) AppName() string { return c.appName }

// Status 描述当前环境的可用状态，用于前端的「是否在飞牛里」提示。
//
// Socket **不带 json tag**：这套网关 socket 只在服务端用，不往浏览器下发（AGENTS 的
// 约定）。前端需要的是「可不可用 + 一句原因」，路径对它没有用处。
type Status struct {
	Available bool   `json:"available"`
	Socket    string `json:"-"`
	App       string `json:"app"`
	Message   string `json:"message"`
}

// Probe 探一次可用性：socket 连得上、token 又读得到，才算可用。
func (c *Client) Probe(ctx context.Context) Status {
	st := Status{Socket: c.socket, App: c.appName}

	if _, err := os.Stat(c.socket); err != nil {
		st.Message = "找不到飞牛开放 API 的 Socket（" + c.socket + "），当前不在飞牛应用里运行"
		return st
	}
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", c.socket)
	if err != nil {
		st.Message = "连不上飞牛开放 API 的 Socket：" + err.Error()
		return st
	}
	_ = conn.Close()

	if strings.TrimSpace(os.Getenv(EnvToken)) == "" {
		st.Message = "没有拿到 " + EnvToken + "，请从飞牛应用中心启动本应用"
		return st
	}
	st.Available = true
	return st
}

type apiRequest struct {
	ReqID   string `json:"reqId"`
	Req     string `json:"req"`
	AppName string `json:"appName"`
	Data    any    `json:"data"`
}

type apiResponse struct {
	ReqID string          `json:"reqId"`
	Code  int             `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
}

// Call 调一次开放 API，把 data 解到 out（out 可以为 nil）。
func (c *Client) Call(ctx context.Context, req string, data any, out any) error {
	raw, status, err := c.CallRaw(ctx, req, data)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &APIError{Code: 0, HTTP: status, Msg: "响应数据格式不符合预期: " + err.Error()}
	}
	return nil
}

// CallRaw 调一次开放 API，把 data 原样交给调用方——返回结构在不同系统版本上可能
// 不一样（convertPath 就踩到了），先拿到原文再自己解，比钉死一个结构稳。
func (c *Client) CallRaw(ctx context.Context, req string, data any) (json.RawMessage, int, error) {
	token := strings.TrimSpace(os.Getenv(EnvToken))
	if token == "" {
		return nil, 0, fmt.Errorf("%w：没有读到环境变量 %s", ErrUnavailable, EnvToken)
	}
	if data == nil {
		data = map[string]any{}
	}

	body, err := json.Marshal(apiRequest{
		ReqID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Req:     req,
		AppName: c.appName,
		Data:    data,
	})
	if err != nil {
		return nil, 0, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/api/v1/trimapp", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("%w（%s）：%v", ErrUnavailable, c.socket, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("读取飞牛接口响应失败: %w", err)
	}

	var resp apiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, res.StatusCode, &APIError{HTTP: res.StatusCode, Msg: "响应不是合法 JSON: " + snippet(raw)}
	}
	if resp.Code != 0 || res.StatusCode != http.StatusOK {
		return nil, res.StatusCode, &APIError{Code: resp.Code, Msg: resp.Msg, HTTP: res.StatusCode}
	}
	return resp.Data, res.StatusCode, nil
}

// FolderList 是 getUserAccessibleFolders 的结果。
type FolderList struct {
	Paths []string `json:"paths"`
}

// GetUserAccessibleFolders 查询当前用户授权给本应用的目录。
//
// 只包含目录授权，文件授权（pickUserFile 选的是文件时）不会出现在这里。
func (c *Client) GetUserAccessibleFolders(ctx context.Context, uid int) ([]string, error) {
	var out FolderList
	if err := c.Call(ctx, ReqGetUserAccessibleFolders, map[string]any{"uid": uid}, &out); err != nil {
		return nil, err
	}
	if out.Paths == nil {
		out.Paths = []string{}
	}
	return out.Paths, nil
}

// DelUserAccessibleFolder 删除一条目录授权。
func (c *Client) DelUserAccessibleFolder(ctx context.Context, uid int, path string) error {
	var out struct {
		Suc bool `json:"suc"`
	}
	err := c.Call(ctx, ReqDelUserAccessibleFolder, map[string]any{"uid": uid, "path": path}, &out)
	if err != nil {
		return err
	}
	if !out.Suc {
		return &APIError{Msg: "系统没有确认删除授权目录"}
	}
	return nil
}

// ACL 是一个路径对当前用户的权限。
type ACL struct {
	Path      string `json:"path"`
	Readable  bool   `json:"readable"`
	Writable  bool   `json:"writable"`
	Deletable bool   `json:"deletable"`
}

// CheckUserACL 检查当前用户对若干路径的权限。
//
// 授权只让应用用户拿到 ACL，不代表当前登录用户也能读写；返回文件列表或读写
// 文件之前用它兜一道。
func (c *Client) CheckUserACL(ctx context.Context, uid int, paths []string) ([]ACL, error) {
	if len(paths) == 0 {
		return []ACL{}, nil
	}
	var out []ACL
	if err := c.Call(ctx, ReqCheckUserACL, map[string]any{"uid": uid, "path": paths}, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ACL{}
	}
	return out, nil
}

// SemanticPath 是路径转换的结果。
type SemanticPath struct {
	Path         string `json:"path"`
	SemanticPath string `json:"semanticPath"`
}

// ConvertPath 把 /vol1/... 这类内部路径转成给用户看的语义路径。
//
// language 必须传（文档明确要求，且必须按当前界面语言传），否则用户会看到和界面
// 语言不一致的路径文案。
//
// 注意 data.status：文档里 0 才是转换成功。code 为 0 而 status 非 0 时，result 里
// 拿不到可用的语义路径——不报出来，调用方只能看到「一条都没转换成功」。
func (c *Client) ConvertPath(ctx context.Context, paths []string, language string) ([]SemanticPath, error) {
	if len(paths) == 0 {
		return []SemanticPath{}, nil
	}
	if strings.TrimSpace(language) == "" {
		language = "zh-CN"
	}
	raw, _, err := c.CallRaw(ctx, ReqConvertPath, map[string]any{"path": paths, "language": language})
	if err != nil {
		return nil, err
	}
	return decodeSemanticPaths(raw, paths)
}

// decodeSemanticPaths 解 convertPath 的 data。
//
// 官方文档写的是 `{"status":0,"result":[{path,semanticPath}...]}`，但实测的系统
// 直接回**数组**（`[{path,semanticPath}...]`）——钉死成文档里的结构，用户看到的是
// 「响应数据格式不符合预期: cannot unmarshal array into Go value of type struct」，
// 界面则退回显示 /vol1/...。三种形态都认，以后系统再改也不会又变成「什么都不显示」：
//
//  1. {"status":0,"result":[{path,semanticPath}...]}   文档形态
//  2. [{path,semanticPath}...]                          实测形态
//  3. ["存储空间1/..."]                                 只回展示路径，按请求顺序对应
func decodeSemanticPaths(raw json.RawMessage, requested []string) ([]SemanticPath, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []SemanticPath{}, nil
	}

	switch trimmed[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, &APIError{Msg: "路径转换返回格式无法解析: " + err.Error()}
		}
		out := make([]SemanticPath, 0, len(items))
		for i, item := range items {
			item = bytes.TrimSpace(item)
			if len(item) > 0 && item[0] == '"' {
				var semantic string
				if err := json.Unmarshal(item, &semantic); err != nil {
					return nil, &APIError{Msg: "路径转换返回格式无法解析: " + err.Error()}
				}
				path := ""
				if i < len(requested) {
					path = requested[i]
				}
				out = append(out, SemanticPath{Path: path, SemanticPath: semantic})
				continue
			}
			var one SemanticPath
			if err := json.Unmarshal(item, &one); err != nil {
				return nil, &APIError{Msg: "路径转换返回格式无法解析: " + err.Error()}
			}
			out = append(out, one)
		}
		return out, nil

	case '{':
		var obj struct {
			Status int            `json:"status"`
			Result []SemanticPath `json:"result"`
		}
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, &APIError{Msg: "路径转换返回格式无法解析: " + err.Error()}
		}
		if obj.Status != 0 {
			return nil, &APIError{Msg: fmt.Sprintf("路径转换失败（status %d）", obj.Status)}
		}
		if obj.Result == nil {
			obj.Result = []SemanticPath{}
		}
		return obj.Result, nil
	}

	return nil, &APIError{Msg: "路径转换返回了预期之外的数据: " + snippet(trimmed)}
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
