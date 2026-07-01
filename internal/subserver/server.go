package subserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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
	return s.requestLogger(mux)
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

	listener, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return err
	}
	s.logLoaded(listener.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// requestLogger 记录不包含 token 和 query 的 HTTP 访问日志。
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		s.logRequest(r, recorder, startedAt)
	})
}

// logRequest 输出一次 HTTP 请求摘要，避免记录原始 path 中的 token。
func (s *Server) logRequest(r *http.Request, recorder *responseRecorder, startedAt time.Time) {
	route, user := subscriptionLogRoute(r.URL.Path)
	status := recorder.Status()
	event := s.logger.Info()
	if status >= http.StatusInternalServerError {
		event = s.logger.Error()
	} else if status >= http.StatusBadRequest {
		event = s.logger.Warn()
	}
	if user != "" {
		event = event.Str("user", user)
	}
	event.
		Str("method", r.Method).
		Str("route", route).
		Int("status", status).
		Str("client_ip", requestClientIP(r)).
		Dur("latency", time.Since(startedAt)).
		Int("bytes", recorder.Bytes()).
		Msg("HTTP request completed")
}

// logLoaded 输出订阅服务启动摘要，便于 systemd 或 launchd 日志确认加载状态。
func (s *Server) logLoaded(listen string) {
	index, _ := s.state.Snapshot()
	sources := 0
	nodes := 0
	users := 0
	if index != nil {
		sources = len(index.Sources)
		nodes = len(index.Nodes)
		users = index.UserCount()
	}
	access := s.config.Access.Type
	if access == "" {
		access = "none"
	}
	s.logger.Info().
		Str("data_dir", s.baseDir).
		Str("input_dir", subscription.InputsDir(s.baseDir)).
		Str("listen", listen).
		Str("access", access).
		Int("inputs", sources).
		Int("sources", sources).
		Int("nodes", nodes).
		Int("users", users).
		Msg("Subscription server loaded")
}

// subscriptionLogRoute 返回脱敏后的路由名称和用户。
func subscriptionLogRoute(rawPath string) (string, string) {
	if rawPath == "/health" {
		return "health", ""
	}
	route, ok := parseSubscriptionRoute(rawPath)
	if !ok {
		return "unmatched", ""
	}
	return "subscription", route.user
}

// requestClientIP 返回请求来源 IP，优先使用反向代理转发头。
func requestClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// Status 返回最终 HTTP 状态码，未显式写入时按 200 处理。
func (r *responseRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// Bytes 返回响应正文写入字节数。
func (r *responseRecorder) Bytes() int {
	return r.bytes
}

// WriteHeader 记录 HTTP 状态码并转发给底层 ResponseWriter。
func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write 记录响应字节数并确保默认状态码存在。
func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

// isSubscriptionPrefix 将 proxystack-go 订阅路径前缀映射到内部格式。
func isSubscriptionPrefix(prefix string) (string, bool) {
	switch prefix {
	case "sub":
		return string(subscription.FormatClash), true
	case "premium_sub":
		return string(subscription.FormatPremiumClash), true
	case "surge_sub":
		return string(subscription.FormatSurge), true
	case "sing-box":
		return string(subscription.FormatSingBox), true
	default:
		return "", false
	}
}

// isEmptyPathToken 判断三段路由中的 path token 是否为空。
func isEmptyPathToken(token string) bool {
	return token == ""
}

// validateSubscriptionRouteParts 校验订阅路径段数量。
func validateSubscriptionRouteParts(parts []string) bool {
	return len(parts) == 2 || len(parts) == 3
}

// parseRouteUser 解码路由中的 user 字段。
func parseRouteUser(part string) (string, bool) {
	user, err := url.PathUnescape(part)
	if err != nil || user == "" {
		return "", false
	}
	return user, true
}

// parseRouteToken 解码路由中的 path token 字段。
func parseRouteToken(part string) (string, bool) {
	token, err := url.PathUnescape(part)
	if err != nil || isEmptyPathToken(token) {
		return "", false
	}
	return token, true
}

// parseRoutePrefix 解码并映射路由前缀。
func parseRoutePrefix(part string) (string, bool) {
	prefix, err := url.PathUnescape(part)
	if err != nil || prefix == "" {
		return "", false
	}
	return isSubscriptionPrefix(prefix)
}

// buildSubscriptionRoute 按路径段构造订阅路由。
func buildSubscriptionRoute(parts []string) (subscriptionRoute, bool) {
	format, ok := parseRoutePrefix(parts[0])
	if !ok {
		return subscriptionRoute{}, false
	}
	if len(parts) == 2 {
		user, ok := parseRouteUser(parts[1])
		if !ok {
			return subscriptionRoute{}, false
		}
		return subscriptionRoute{format: format, user: user}, true
	}
	token, ok := parseRouteToken(parts[1])
	if !ok {
		return subscriptionRoute{}, false
	}
	user, ok := parseRouteUser(parts[2])
	if !ok {
		return subscriptionRoute{}, false
	}
	return subscriptionRoute{format: format, pathToken: token, user: user}, true
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
		return nil, fmt.Errorf("read subscription input directory %s: %w", dir, err)
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
			return nil, fmt.Errorf("subscription input filename %s: %w", name, err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read subscription input file info %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read subscription input %s: %w", name, err)
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

// parseSubscriptionRoute 解析 /sub/user、/sub/token/user 等 proxystack-go 兼容订阅路径。
func parseSubscriptionRoute(rawPath string) (subscriptionRoute, bool) {
	trimmed := strings.Trim(rawPath, "/")
	parts := strings.Split(trimmed, "/")
	if !validateSubscriptionRouteParts(parts) {
		return subscriptionRoute{}, false
	}
	return buildSubscriptionRoute(parts)
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
