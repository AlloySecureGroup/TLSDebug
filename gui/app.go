package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const sessionFileVersion = 1

type Settings struct {
	ProxyPort   int    `json:"proxyPort"`
	MonitorPort int    `json:"monitorPort"`
	DataDir     string `json:"dataDir"`
	ProxyBinary string `json:"proxyBinary"`
}

type AppState struct {
	Running    bool     `json:"running"`
	Settings   Settings `json:"settings"`
	CAPath     string   `json:"caPath"`
	BinaryPath string   `json:"binaryPath"`
}

type TrafficEntry struct {
	ID              int                 `json:"id"`
	Timestamp       string              `json:"timestamp"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	Host            string              `json:"host"`
	Path            string              `json:"path"`
	StatusCode      int                 `json:"statusCode"`
	StatusText      string              `json:"statusText"`
	RequestHeaders  map[string][]string `json:"requestHeaders"`
	ResponseHeaders map[string][]string `json:"responseHeaders"`
	RequestBody     string              `json:"requestBody"`
	ResponseBody    string              `json:"responseBody"`
	ContentType     string              `json:"contentType"`
	Duration        int64               `json:"duration"`
	TLSVersion      string              `json:"tlsVersion"`
	ClientAddr      string              `json:"clientAddr"`
}

type sessionFile struct {
	Version    int            `json:"version"`
	ExportedAt time.Time      `json:"exportedAt"`
	Entries    []TrafficEntry `json:"entries"`
}

type App struct {
	ctx        context.Context
	mu         sync.RWMutex
	cmd        *exec.Cmd
	settings   Settings
	configPath string
	httpClient *http.Client
}

func NewApp() *App {
	return &App{
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configRoot, err := os.UserConfigDir()
	if err != nil {
		configRoot = "."
	}
	appDir := filepath.Join(configRoot, "TLSDebug")
	a.configPath = filepath.Join(appDir, "gui-settings.json")
	a.settings = Settings{
		ProxyPort:   8080,
		MonitorPort: 4040,
		DataDir:     filepath.Join(appDir, "sessions"),
	}

	_ = os.MkdirAll(appDir, 0700)
	_ = a.loadSettings()
	_ = os.MkdirAll(a.settings.DataDir, 0700)
}

func (a *App) shutdown(context.Context) {
	_ = a.stopProxy()
}

func (a *App) GetState() AppState {
	a.mu.RLock()
	settings := a.settings
	running := a.cmd != nil && a.cmd.Process != nil
	a.mu.RUnlock()

	binary, _ := a.resolveProxyBinary(settings)
	return AppState{
		Running:    running,
		Settings:   settings,
		CAPath:     filepath.Join(settings.DataDir, "proxy-ca.crt"),
		BinaryPath: binary,
	}
}

func (a *App) UpdateSettings(settings Settings) (AppState, error) {
	if settings.ProxyPort < 1 || settings.ProxyPort > 65535 {
		return AppState{}, errors.New("proxy port must be between 1 and 65535")
	}
	if settings.MonitorPort < 1 || settings.MonitorPort > 65535 {
		return AppState{}, errors.New("monitor port must be between 1 and 65535")
	}
	if settings.ProxyPort == settings.MonitorPort {
		return AppState{}, errors.New("proxy and monitor ports must be different")
	}
	if strings.TrimSpace(settings.DataDir) == "" {
		return AppState{}, errors.New("data directory is required")
	}

	settings.DataDir = filepath.Clean(settings.DataDir)
	settings.ProxyBinary = strings.TrimSpace(settings.ProxyBinary)
	if err := os.MkdirAll(settings.DataDir, 0700); err != nil {
		return AppState{}, fmt.Errorf("create data directory: %w", err)
	}

	a.mu.Lock()
	if a.cmd != nil {
		a.mu.Unlock()
		return AppState{}, errors.New("stop the proxy before changing settings")
	}
	a.settings = settings
	a.mu.Unlock()

	if err := a.saveSettings(); err != nil {
		return AppState{}, err
	}
	return a.GetState(), nil
}

func (a *App) StartProxy() (AppState, error) {
	a.mu.Lock()
	if a.cmd != nil {
		a.mu.Unlock()
		return a.GetState(), nil
	}
	settings := a.settings
	a.mu.Unlock()

	binary, err := a.resolveProxyBinary(settings)
	if err != nil {
		return AppState{}, err
	}
	if err := os.MkdirAll(settings.DataDir, 0700); err != nil {
		return AppState{}, fmt.Errorf("create data directory: %w", err)
	}

	args := []string{
		"-port", strconv.Itoa(settings.ProxyPort),
		"-listen-host", "127.0.0.1",
		"-monitor-port", strconv.Itoa(settings.MonitorPort),
		"-monitor-host", "127.0.0.1",
		"-certdir", settings.DataDir,
		"-skip-install",
	}
	configPath := filepath.Join(settings.DataDir, "proxy-config.ini")
	if _, statErr := os.Stat(configPath); statErr == nil {
		args = append(args, "-config", configPath)
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = settings.DataDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return AppState{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return AppState{}, err
	}
	if err := cmd.Start(); err != nil {
		return AppState{}, fmt.Errorf("start TLSDebug proxy: %w", err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()

	go a.streamOutput(stdout)
	go a.streamOutput(stderr)
	go func() {
		err := cmd.Wait()
		a.mu.Lock()
		if a.cmd == cmd {
			a.cmd = nil
		}
		a.mu.Unlock()
		if a.ctx != nil {
			message := "Proxy stopped"
			if err != nil {
				message = "Proxy stopped: " + err.Error()
			}
			wailsruntime.EventsEmit(a.ctx, "proxy:log", message)
			wailsruntime.EventsEmit(a.ctx, "proxy:state", false)
		}
	}()

	wailsruntime.EventsEmit(a.ctx, "proxy:state", true)
	return a.GetState(), nil
}

func (a *App) StopProxy() (AppState, error) {
	if err := a.stopProxy(); err != nil {
		return AppState{}, err
	}
	return a.GetState(), nil
}

func (a *App) stopProxy() error {
	a.mu.RLock()
	cmd := a.cmd
	a.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if goruntime.GOOS == "windows" {
		return cmd.Process.Kill()
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return cmd.Process.Kill()
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.mu.RLock()
		stillRunning := a.cmd == cmd
		a.mu.RUnlock()
		if stillRunning {
			_ = cmd.Process.Kill()
		}
	}()
	return nil
}

func (a *App) GetSessions() ([]TrafficEntry, error) {
	a.mu.RLock()
	monitorPort := a.settings.MonitorPort
	a.mu.RUnlock()

	response, err := a.httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/api/entries", monitorPort))
	if err != nil {
		return nil, fmt.Errorf("connect to proxy monitor: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy monitor returned %s", response.Status)
	}

	var entries []TrafficEntry
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode sessions: %w", err)
	}
	return entries, nil
}

func (a *App) ClearSessions() error {
	a.mu.RLock()
	monitorPort := a.settings.MonitorPort
	a.mu.RUnlock()

	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/clear", monitorPort), nil)
	if err != nil {
		return err
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("connect to proxy monitor: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy monitor returned %s", response.Status)
	}
	return nil
}

func (a *App) SaveSessions(entries []TrafficEntry) (string, error) {
	defaultName := "tlsdebug-" + time.Now().Format("20060102-150405") + ".tlsdebug.json"
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save TLSDebug Sessions",
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "TLSDebug Sessions", Pattern: "*.tlsdebug.json"},
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	})
	if err != nil || path == "" {
		return path, err
	}

	payload, err := json.MarshalIndent(sessionFile{
		Version:    sessionFileVersion,
		ExportedAt: time.Now().UTC(),
		Entries:    entries,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, payload, 0600); err != nil {
		return "", fmt.Errorf("save sessions: %w", err)
	}
	return path, nil
}

func (a *App) ImportSessions() ([]TrafficEntry, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Import TLSDebug Sessions",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "TLSDebug Sessions", Pattern: "*.tlsdebug.json;*.json"},
		},
	})
	if err != nil || path == "" {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sessions: %w", err)
	}

	var envelope sessionFile
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Entries != nil {
		return envelope.Entries, nil
	}
	var entries []TrafficEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, errors.New("file is not a supported TLSDebug session export")
	}
	return entries, nil
}

func (a *App) InstallRootCA() (string, error) {
	a.mu.RLock()
	certPath := filepath.Join(a.settings.DataDir, "proxy-ca.crt")
	a.mu.RUnlock()
	if _, err := os.Stat(certPath); err != nil {
		return "", errors.New("CA certificate not found; start the proxy once to generate it")
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("certutil", "-addstore", "-user", "Root", certPath)
	case "darwin":
		command := "security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain " + shellQuote(certPath)
		script := fmt.Sprintf("do shell script %q with administrator privileges", command)
		cmd = exec.Command("osascript", "-e", script)
	case "linux":
		message, err := installLinuxRootCA(certPath)
		if err != nil {
			return "", err
		}
		return message, nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goruntime.GOOS)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("install CA: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return "Root CA installed successfully. Restart browsers that were already open.", nil
}

func installLinuxRootCA(certPath string) (string, error) {
	command := "install -m 0644 " + shellQuote(certPath) +
		" /usr/local/share/ca-certificates/tlsdebug.crt && update-ca-certificates"
	output, err := exec.Command("pkexec", "sh", "-c", command).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("install system CA: %w: %s", err, strings.TrimSpace(string(output)))
	}

	certutil, err := exec.LookPath("certutil")
	if err != nil {
		return "Root CA installed in the Linux system store. Install libnss3-tools to also update Chrome's NSS store.", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory for Chrome trust store: %w", err)
	}
	nssDir := filepath.Join(home, ".pki", "nssdb")
	if err := os.MkdirAll(nssDir, 0700); err != nil {
		return "", fmt.Errorf("create Chrome NSS directory: %w", err)
	}
	database := "sql:" + nssDir
	if _, err := os.Stat(filepath.Join(nssDir, "cert9.db")); errors.Is(err, os.ErrNotExist) {
		if output, err := exec.Command(certutil, "-N", "-d", database, "--empty-password").CombinedOutput(); err != nil {
			return "", fmt.Errorf("create Chrome NSS database: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	_ = exec.Command(certutil, "-D", "-d", database, "-n", "TLSDebug CA").Run()
	output, err = exec.Command(
		certutil, "-A", "-d", database, "-t", "CT,C,C", "-n", "TLSDebug CA", "-i", certPath,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("install CA in Chrome NSS store: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return "Root CA installed in the Linux system and Chrome NSS stores. Restart browsers that were already open.", nil
}

func (a *App) loadSettings() error {
	data, err := os.ReadFile(a.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &a.settings)
}

func (a *App) saveSettings() error {
	a.mu.RLock()
	settings := a.settings
	a.mu.RUnlock()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.configPath, data, 0600); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

func (a *App) resolveProxyBinary(settings Settings) (string, error) {
	name := "tlsproxy"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}

	var candidates []string
	if settings.ProxyBinary != "" {
		candidates = append(candidates, settings.ProxyBinary)
	}
	if envPath := os.Getenv("TLSDEBUG_PROXY_BIN"); envPath != "" {
		candidates = append(candidates, envPath)
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, name),
			filepath.Join(executableDir, "bin", name),
		)
	}
	candidates = append(candidates,
		filepath.Join("bin", name),
		filepath.Join("..", name),
	)

	for _, candidate := range candidates {
		path, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", errors.New("TLSDebug proxy binary not found; set its path in Settings or use a build script")
}

func (a *App) streamOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "proxy:log", scanner.Text())
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
