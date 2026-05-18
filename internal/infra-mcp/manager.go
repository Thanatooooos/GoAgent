package inframcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultStartupTimeout = 10 * time.Second
	defaultCallTimeout    = 15 * time.Second
)

var (
	ErrManagerClosed        = errors.New("mcp manager is closed")
	ErrServerNotConfigured  = errors.New("mcp server not configured")
	ErrServerDisabled       = errors.New("mcp server disabled")
	ErrUnsupportedTransport = errors.New("mcp transport is not supported")
	ErrServerCommandMissing = errors.New("mcp server command is required")
	ErrToolNotFound         = errors.New("mcp tool not found")
	ErrCallTimeout          = errors.New("mcp call timed out")
)

type ServerConfig struct {
	Enabled          bool
	Transport        string
	Command          string
	Args             []string
	Env              map[string]string
	StartupTimeoutMs int
	CallTimeoutMs    int
}

type ToolClient interface {
	ListTools(ctx context.Context, serverName string) ([]*mcp.Tool, error)
	CallTool(ctx context.Context, serverName string, toolName string, args map[string]any) (*mcp.CallToolResult, error)
	Close() error
}

type Manager struct {
	mu       sync.Mutex
	servers  map[string]ServerConfig
	sessions map[string]*mcp.ClientSession
	closed   bool
}

func NewManager(servers map[string]ServerConfig) *Manager {
	cloned := make(map[string]ServerConfig, len(servers))
	for name, cfg := range servers {
		cloned[strings.TrimSpace(name)] = cloneServerConfig(cfg)
	}
	return &Manager{
		servers:  cloned,
		sessions: make(map[string]*mcp.ClientSession),
	}
}

func (m *Manager) ListTools(ctx context.Context, serverName string) ([]*mcp.Tool, error) {
	session, cfg, err := m.getSession(ctx, serverName)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := withTimeout(ctx, cfg.callTimeout())
	defer cancel()

	var tools []*mcp.Tool
	var cursor string
	for {
		result, err := session.ListTools(callCtx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			m.handleSessionError(serverName, err)
			return nil, wrapCallError(err)
		}
		tools = append(tools, result.Tools...)
		cursor = strings.TrimSpace(result.NextCursor)
		if cursor == "" {
			break
		}
	}
	return tools, nil
}

func (m *Manager) CallTool(ctx context.Context, serverName string, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	session, cfg, err := m.getSession(ctx, serverName)
	if err != nil {
		return nil, err
	}

	tools, err := m.ListTools(ctx, serverName)
	if err != nil {
		return nil, err
	}
	if !hasTool(tools, toolName) {
		return nil, fmt.Errorf("%w: server=%s tool=%s", ErrToolNotFound, serverName, strings.TrimSpace(toolName))
	}

	callCtx, cancel := withTimeout(ctx, cfg.callTimeout())
	defer cancel()

	result, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      strings.TrimSpace(toolName),
		Arguments: cloneArguments(args),
	})
	if err != nil {
		m.handleSessionError(serverName, err)
		return nil, wrapCallError(err)
	}
	return result, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	sessions := make(map[string]*mcp.ClientSession, len(m.sessions))
	for name, session := range m.sessions {
		sessions[name] = session
	}
	m.sessions = make(map[string]*mcp.ClientSession)
	m.mu.Unlock()

	var errs []error
	for _, session := range sessions {
		if session != nil {
			errs = append(errs, session.Close())
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) getSession(ctx context.Context, serverName string) (*mcp.ClientSession, managedConfig, error) {
	name := strings.TrimSpace(serverName)
	if name == "" {
		return nil, managedConfig{}, fmt.Errorf("%w: empty server name", ErrServerNotConfigured)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, managedConfig{}, ErrManagerClosed
	}
	if session, ok := m.sessions[name]; ok && session != nil {
		cfg := toManagedConfig(m.servers[name])
		m.mu.Unlock()
		return session, cfg, nil
	}
	rawCfg, ok := m.servers[name]
	m.mu.Unlock()
	if !ok {
		return nil, managedConfig{}, fmt.Errorf("%w: %s", ErrServerNotConfigured, name)
	}

	cfg, err := validateServerConfig(rawCfg)
	if err != nil {
		return nil, managedConfig{}, fmt.Errorf("%w: server=%s", err, name)
	}

	startCtx, cancel := withTimeout(ctx, cfg.startupTimeout())
	defer cancel()

	session, err := connectSession(startCtx, cfg)
	if err != nil {
		return nil, managedConfig{}, fmt.Errorf("mcp server start failed: server=%s: %w", name, wrapCallError(err))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		_ = session.Close()
		return nil, managedConfig{}, ErrManagerClosed
	}
	if existing, ok := m.sessions[name]; ok && existing != nil {
		_ = session.Close()
		return existing, cfg, nil
	}
	m.sessions[name] = session
	return session, cfg, nil
}

func (m *Manager) handleSessionError(serverName string, err error) {
	if err == nil {
		return
	}
	if !errors.Is(err, mcp.ErrConnectionClosed) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, strings.TrimSpace(serverName))
}

type managedConfig struct {
	Enabled          bool
	Transport        string
	Command          string
	Args             []string
	Env              map[string]string
	StartupTimeoutMs int
	CallTimeoutMs    int
}

func toManagedConfig(cfg ServerConfig) managedConfig {
	return managedConfig{
		Enabled:          cfg.Enabled,
		Transport:        cfg.Transport,
		Command:          strings.TrimSpace(cfg.Command),
		Args:             append([]string(nil), cfg.Args...),
		Env:              cloneEnv(cfg.Env),
		StartupTimeoutMs: cfg.StartupTimeoutMs,
		CallTimeoutMs:    cfg.CallTimeoutMs,
	}
}

func validateServerConfig(cfg ServerConfig) (managedConfig, error) {
	managed := toManagedConfig(cfg)
	if !managed.Enabled {
		return managedConfig{}, ErrServerDisabled
	}
	transport := strings.ToLower(strings.TrimSpace(managed.Transport))
	if transport == "" {
		transport = "stdio"
	}
	if transport != "stdio" {
		return managedConfig{}, ErrUnsupportedTransport
	}
	managed.Transport = transport
	if managed.Command == "" {
		return managedConfig{}, ErrServerCommandMissing
	}
	return managed, nil
}

func connectSession(ctx context.Context, cfg managedConfig) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "goagent-mcp-client",
		Version: "v1.0.0",
	}, nil)

	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = mergeEnv(os.Environ(), cfg.Env)

	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		return nil, err
	}
	return session, nil
}

func (c managedConfig) startupTimeout() time.Duration {
	if c.StartupTimeoutMs > 0 {
		return time.Duration(c.StartupTimeoutMs) * time.Millisecond
	}
	return defaultStartupTimeout
}

func (c managedConfig) callTimeout() time.Duration {
	if c.CallTimeoutMs > 0 {
		return time.Duration(c.CallTimeoutMs) * time.Millisecond
	}
	return defaultCallTimeout
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func wrapCallError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrCallTimeout, err)
	}
	return err
}

func hasTool(tools []*mcp.Tool, toolName string) bool {
	want := strings.TrimSpace(toolName)
	for _, tool := range tools {
		if tool != nil && strings.TrimSpace(tool.Name) == want {
			return true
		}
	}
	return false
}

func mergeEnv(base []string, override map[string]string) []string {
	if len(override) == 0 {
		return append([]string(nil), base...)
	}
	merged := make(map[string]string, len(base)+len(override))
	for _, kv := range base {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			merged[parts[0]] = parts[1]
		}
	}
	for key, value := range override {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		merged[key] = value
	}
	result := make([]string, 0, len(merged))
	for key, value := range merged {
		result = append(result, key+"="+value)
	}
	return result
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	return ServerConfig{
		Enabled:          cfg.Enabled,
		Transport:        cfg.Transport,
		Command:          cfg.Command,
		Args:             append([]string(nil), cfg.Args...),
		Env:              cloneEnv(cfg.Env),
		StartupTimeoutMs: cfg.StartupTimeoutMs,
		CallTimeoutMs:    cfg.CallTimeoutMs,
	}
}

func cloneEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneArguments(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

var _ ToolClient = (*Manager)(nil)
