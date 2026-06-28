package subserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/sunliang711/sbox-manager/internal/domain"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

// Options 描述订阅 HTTP 服务启动参数。
type Options struct {
	BaseDir string
	Config  domain.SubConfig
	Logger  zerolog.Logger
	Now     func() time.Time
}

// Server 封装订阅 HTTP handler、索引状态和 watcher。
type Server struct {
	baseDir  string
	config   domain.SubConfig
	state    *State
	renderer *subscription.Renderer
	logger   zerolog.Logger
	now      func() time.Time
}

// State 保存当前可用索引和最近一次 reload 错误。
type State struct {
	mu        sync.RWMutex
	index     *subscription.Index
	lastError string
}

// New 加载初始 input 索引并创建订阅服务。
func New(options Options) (*Server, error) {
	index, err := subscription.LoadIndexFromDir(subscription.InputsDir(options.BaseDir))
	if err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Server{
		baseDir:  options.BaseDir,
		config:   options.Config,
		state:    NewState(index),
		renderer: subscription.NewRenderer(options.BaseDir, options.Config.TemplatesDir),
		logger:   options.Logger,
		now:      now,
	}, nil
}

// Run 创建服务并阻塞运行到 context 取消或 HTTP server 返回错误。
func Run(ctx context.Context, options Options) error {
	server, err := New(options)
	if err != nil {
		return err
	}
	return server.ListenAndServe(ctx)
}

// NewState 创建索引状态容器。
func NewState(index *subscription.Index) *State {
	return &State{index: index}
}

// Snapshot 返回当前索引和最近 reload 错误。
func (s *State) Snapshot() (*subscription.Index, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index, s.lastError
}

// Replace 成功 reload 后替换索引并清空错误。
func (s *State) Replace(index *subscription.Index) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index = index
	s.lastError = ""
}

// MarkReloadError 记录 reload 错误但保留旧索引。
func (s *State) MarkReloadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err.Error()
}

// Handler 创建订阅 HTTP 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleSubscription)
	return mux
}

// ListenAndServe 启动 HTTP server 和轮询 watcher。
func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{
		Addr:    s.config.Listen,
		Handler: s.Handler(),
	}
	watcherCtx, cancelWatcher := context.WithCancel(ctx)
	defer cancelWatcher()
	go s.watchInputs(watcherCtx)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// handleHealth 输出健康状态、索引摘要和 reload 错误。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	index, lastError := s.state.Snapshot()
	response := map[string]interface{}{
		"status": "ok",
		"users":  0,
	}
	if index == nil {
		response["status"] = "error"
		response["index"] = nil
	} else {
		response["users"] = index.UserCount()
		response["index"] = map[string]interface{}{
			"sources":  index.Sources,
			"nodes":    len(index.Nodes),
			"built_at": index.BuiltAt.Format(time.RFC3339),
		}
	}
	if lastError != "" {
		response["status"] = "degraded"
		response["last_error"] = lastError
	}
	writeJSON(w, http.StatusOK, response)
}

// handleSubscription 处理四类订阅格式的用户路由。
func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	route, ok := parseSubscriptionRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	format, err := subscription.ParseFormat(route.format)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err := s.authorize(route.pathToken, r.URL.Query().Get("token")); err != nil {
		status := http.StatusForbidden
		code := "forbidden"
		if errors.Is(err, errTokenMissing) {
			status = http.StatusUnauthorized
			code = "unauthorized"
		}
		writeError(w, status, code, http.StatusText(status))
		return
	}

	index, _ := s.state.Snapshot()
	if index == nil {
		writeError(w, http.StatusServiceUnavailable, "index_unavailable", "index unavailable")
		return
	}
	nodes := index.NodesForUser(route.user)
	if len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	renderNodes := subscription.FilterNodesForFormat(format, nodes)
	if len(renderNodes) == 0 {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	data, err := s.renderer.Render(format, route.user, renderNodes, subscription.RenderOptions{
		Config:     s.config,
		RequestURL: requestURL(r),
		Sources:    index.Sources,
		Now:        s.now(),
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "template_error", "template error")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// authorize 按 access.type 校验 path token 或 query token。
func (s *Server) authorize(pathToken string, queryToken string) error {
	if s.config.Access.Type == "none" {
		return nil
	}
	token := queryToken
	if pathToken != "" {
		token = pathToken
	}
	if token == "" {
		return errTokenMissing
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Access.Token)) != 1 {
		return errTokenMismatch
	}
	return nil
}

// watchInputs 轮询 input 内容 hash，变化后完整 reload。
func (s *Server) watchInputs(ctx context.Context) {
	inputDir := subscription.InputsDir(s.baseDir)
	previous, err := snapshotInputDir(inputDir)
	if err != nil {
		s.state.MarkReloadError(err)
		s.logReloadError(err, inputDir)
	}
	ticker := time.NewTicker(s.config.WatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := snapshotInputDir(inputDir)
			if err != nil {
				s.state.MarkReloadError(err)
				s.logReloadError(err, inputDir)
				continue
			}
			_, lastError := s.state.Snapshot()
			if reflect.DeepEqual(previous, current) && lastError == "" {
				continue
			}
			if !sleepWithContext(ctx, s.config.WatchDebounce) {
				return
			}
			index, err := subscription.LoadIndexFromDir(inputDir)
			if err != nil {
				s.state.MarkReloadError(err)
				s.logReloadError(err, inputDir)
				continue
			}
			s.state.Replace(index)
			previous = current
			s.logger.Info().Str("input_dir", inputDir).Int("users", index.UserCount()).Int("nodes", len(index.Nodes)).Msg("subscription reload succeeded")
		}
	}
}

// logReloadError 记录 reload 错误但不包含 token、密码或订阅正文。
func (s *Server) logReloadError(err error, inputDir string) {
	s.logger.Error().Err(err).Str("input_dir", inputDir).Msg("subscription reload failed")
}

// snapshotInputDir 计算 input 文件内容 hash，忽略临时和非 input 文件。
func snapshotInputDir(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取订阅 input 目录 %s: %w", dir, err)
	}
	snapshot := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if subscription.ShouldIgnoreInputName(name) || !subscription.IsInputFileName(name) {
			continue
		}
		if err := domain.ValidateSubscriptionInputFilename(name); err != nil {
			return nil, fmt.Errorf("订阅 input 文件名 %s: %w", name, err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("读取订阅 input 文件信息 %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("读取订阅 input %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		snapshot[name] = hex.EncodeToString(sum[:])
	}
	return snapshot, nil
}

// sleepWithContext 等待 debounce 时间或 context 取消。
func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// parseSubscriptionRoute 解析 /format/user 和 /format/token/user。
func parseSubscriptionRoute(rawPath string) (subscriptionRoute, bool) {
	trimmed := strings.Trim(rawPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return subscriptionRoute{}, false
	}
	format, err := url.PathUnescape(parts[0])
	if err != nil || format == "" {
		return subscriptionRoute{}, false
	}
	if len(parts) == 2 {
		user, err := url.PathUnescape(parts[1])
		if err != nil || user == "" {
			return subscriptionRoute{}, false
		}
		return subscriptionRoute{format: format, user: user}, true
	}
	token, err := url.PathUnescape(parts[1])
	if err != nil {
		return subscriptionRoute{}, false
	}
	user, err := url.PathUnescape(parts[2])
	if err != nil || user == "" {
		return subscriptionRoute{}, false
	}
	return subscriptionRoute{format: format, pathToken: token, user: user}, true
}

// requestURL 返回当前请求 URL，供 Surge managed config 使用。
func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeError 输出统一错误响应。
func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

var (
	errTokenMissing  = errors.New("token missing")
	errTokenMismatch = errors.New("token mismatch")
)

type subscriptionRoute struct {
	format    string
	pathToken string
	user      string
}
