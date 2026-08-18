package modelruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Paths      Paths
	HTTPClient *http.Client
}

type Manager struct {
	paths Paths
	http  *http.Client
}

type State struct {
	PID            int       `json:"pid"`
	Port           int       `json:"port"`
	Endpoint       string    `json:"endpoint"`
	Model          string    `json:"model"`
	RuntimeVersion string    `json:"runtimeVersion"`
	ServerPath     string    `json:"serverPath"`
	StartedAt      time.Time `json:"startedAt"`
}

type Status struct {
	Installed bool
	Running   bool
	State     State
	ModelPath string
	LogPath   string
}

func New() (*Manager, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	return NewWithOptions(Options{Paths: paths}), nil
}

func NewWithOptions(options Options) *Manager {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	return &Manager{paths: options.Paths, http: client}
}

func (m *Manager) Paths() Paths {
	return m.paths
}

func (m *Manager) Status(ctx context.Context) Status {
	status := Status{
		Installed: m.installed(),
		ModelPath: m.paths.Model,
		LogPath:   m.paths.Log,
	}
	state, err := m.readState()
	if err != nil {
		return status
	}
	status.State = state
	status.Running = m.healthy(ctx, state.Endpoint)
	return status
}

func (m *Manager) EnsureRunning(ctx context.Context) (State, error) {
	status := m.Status(ctx)
	if status.Running {
		return status.State, nil
	}
	return m.Start(ctx, 0)
}

func (m *Manager) Start(ctx context.Context, preferredPort int) (State, error) {
	if status := m.Status(ctx); status.Running {
		return status.State, nil
	}
	if !m.installed() {
		return State{}, errors.New("the managed model is not installed; run `prosecheck model install bonsai-8b`")
	}
	if err := os.MkdirAll(m.paths.Root, 0o700); err != nil {
		return State{}, err
	}
	release, err := acquireLock(ctx, m.paths.Lock, 2*time.Minute)
	if err != nil {
		return State{}, fmt.Errorf("cannot start the model: %w", err)
	}
	defer release()
	if status := m.Status(ctx); status.Running {
		return status.State, nil
	}

	asset, err := currentAsset()
	if err != nil {
		return State{}, err
	}
	server, err := findServer(m.paths.Runtime, asset.ServerExe)
	if err != nil {
		return State{}, errors.New("the Prism runtime is incomplete; run `prosecheck model install bonsai-8b`")
	}
	port, err := availablePort(preferredPort)
	if err != nil {
		return State{}, err
	}
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port) + "/v1"
	logFile, err := os.OpenFile(m.paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return State{}, err
	}
	command := exec.Command(server,
		"-m", m.paths.Model,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--alias", ModelName,
		"--ctx-size", strconv.Itoa(defaultContext),
		"--gpu-layers", "99",
		"--no-ui",
	)
	command.Stdout = logFile
	command.Stderr = logFile
	configureCommand(command)
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return State{}, fmt.Errorf("cannot start the Prism runtime: %w", err)
	}
	pid := command.Process.Pid
	_ = logFile.Close()
	if err := command.Process.Release(); err != nil {
		_ = command.Process.Kill()
		return State{}, fmt.Errorf("cannot detach the Prism runtime: %w", err)
	}
	state := State{
		PID: pid, Port: port, Endpoint: endpoint, Model: ModelName,
		RuntimeVersion: RuntimeVersion, ServerPath: server, StartedAt: time.Now().UTC(),
	}
	if err := m.writeState(state); err != nil {
		return State{}, err
	}

	readyContext, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.healthy(readyContext, endpoint) {
			return state, nil
		}
		select {
		case <-readyContext.Done():
			tail, _ := m.TailLogs(8)
			if tail != "" {
				return State{}, fmt.Errorf("the model did not become ready: %w\n%s", readyContext.Err(), tail)
			}
			return State{}, fmt.Errorf("the model did not become ready: %w", readyContext.Err())
		case <-ticker.C:
		}
	}
}

func (m *Manager) Stop(ctx context.Context) error {
	state, err := m.readState()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !m.healthy(ctx, state.Endpoint) {
		return m.removeState()
	}
	if state.PID < 1 {
		return errors.New("the model state has no valid process ID; stop the server manually and start it again")
	}
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return err
	}
	if err := interruptProcess(process); err != nil {
		if killErr := process.Kill(); killErr != nil {
			return errors.Join(err, killErr)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !m.healthy(ctx, state.Endpoint) {
			return m.removeState()
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = process.Kill()
	return m.removeState()
}

func (m *Manager) TailLogs(lines int) (string, error) {
	file, err := os.Open(m.paths.Log)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	if lines < 1 {
		lines = 40
	}
	buffer := make([]string, lines)
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		buffer[count%lines] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	start := 0
	length := count
	if count > lines {
		start = count % lines
		length = lines
	}
	result := make([]string, 0, length)
	for index := 0; index < length; index++ {
		result = append(result, buffer[(start+index)%lines])
	}
	return strings.Join(result, "\n"), nil
}

func (m *Manager) installed() bool {
	asset, err := currentAsset()
	if err != nil || !modelPresent(m.paths.Model) {
		return false
	}
	_, err = findServer(m.paths.Runtime, asset.ServerExe)
	return err == nil
}

func modelPresent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() == ModelSize
}

func (m *Manager) healthy(ctx context.Context, endpoint string) bool {
	if endpoint == "" {
		return false
	}
	healthURL := strings.TrimSuffix(endpoint, "/v1") + "/health"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	response, err := m.http.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func (m *Manager) readState() (State, error) {
	data, err := os.ReadFile(m.paths.State)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("cannot read model state: %w", err)
	}
	return state, nil
}

func (m *Manager) writeState(state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.paths.State), 0o700); err != nil {
		return err
	}
	temporary := m.paths.State + ".partial"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, m.paths.State)
}

func (m *Manager) removeState() error {
	err := os.Remove(m.paths.State)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func availablePort(preferred int) (int, error) {
	if preferred == 0 {
		preferred = DefaultPort
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(preferred))
	if err == nil {
		_ = listener.Close()
		return preferred, nil
	}
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("cannot find a free local port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

func acquireLock(ctx context.Context, path string, staleAfter time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > staleAfter {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
