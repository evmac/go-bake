package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/evmac/go-bake/internal/baked"
	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/daemonproto"
)

func TestBakedStartsAndCreatesSocket(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "bt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	bf := []byte(`target build { steps { exec ["true"] } }
`)
	if err := os.WriteFile(filepath.Join(dir, "Bakefile"), bf, 0644); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(dir, "baked")
	modRoot := findModuleRoot(t)
	build := exec.Command("go", "build", "-o", exe, "./cmd/baked")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baked: %v\n%s", err, out)
	}

	cmd := exec.Command(exe, "--no-watch")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start baked: %v", err)
	}
	defer cmd.Process.Kill()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	pidPath := filepath.Join(dir, ".bake", "baked.pid")
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		if i == 99 {
			t.Fatalf("socket did not appear; stderr: %s", stderr.String())
		}
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Errorf("pid file missing: %v", err)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

func testConfig(dir string) *config.File {
	return &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "build", Steps: []config.Step{{Argv: []string{"true"}}}},
			{Name: "test", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
		Suites: []*config.Suite{
			{Name: "dev", Targets: []string{"build", "test"}},
		},
	}
}

func testWorkspace(dir string) *workspace {
	cfg := testConfig(dir)
	q, _ := baked.NewQueue(dir)
	return &workspace{rootDir: dir, cfg: cfg, queue: q}
}

func defaultResolveWs(ws *workspace) func(string) *workspace {
	return func(_ string) *workspace { return ws }
}

// --- runDaemon tests ---

func TestRunDaemonNoBakefile(t *testing.T) {
	dir := t.TempDir()
	err := runDaemon(dir, true, "", nil)
	if err == nil {
		t.Fatal("expected error with no Bakefile")
	}
}

func TestRunDaemonNoWatch(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "bd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(dir, true, "", nil)
	}()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(30 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	p, _ := os.FindProcess(os.Getpid())
	p.Signal(os.Interrupt)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not exit")
	}
}

func TestRunDaemonWithWatch(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "bd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(dir, false, "", nil)
	}()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(30 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err == nil {
		req := daemonproto.Request{Run: "build"}
		b, _ := json.Marshal(req)
		b = append(b, '\n')
		conn.Write(b)
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		conn.Close()
	}

	p, _ := os.FindProcess(os.Getpid())
	p.Signal(os.Interrupt)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon with watch: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not exit")
	}
}

func TestRunDaemonWithTCP(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "bd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(dir, true, "localhost:0", nil)
	}()

	sockPath := filepath.Join(dir, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(30 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	// Test via Unix socket
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial unix: %v", err)
	}
	req := daemonproto.Request{Run: "build"}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	conn.Write(b)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	conn.Close()
	if resp.ExitCode != 0 {
		t.Errorf("tcp daemon build via unix: exit=%d err=%q", resp.ExitCode, resp.Error)
	}

	p, _ := os.FindProcess(os.Getpid())
	p.Signal(os.Interrupt)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon with TCP: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not exit")
	}
}

func TestRunDaemonWithExtraWorkspace(t *testing.T) {
	dir1, err := os.MkdirTemp(os.TempDir(), "bd1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)
	os.WriteFile(filepath.Join(dir1, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	dir2, err := os.MkdirTemp(os.TempDir(), "bd2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir2)
	os.WriteFile(filepath.Join(dir2, "Bakefile"), []byte("target test { steps { exec [\"true\"] } }\n"), 0644)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(dir1, true, "", []string{dir2})
	}()

	sockPath := filepath.Join(dir1, ".bake", "baked.sock")
	for i := 0; i < 100; i++ {
		time.Sleep(30 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	p, _ := os.FindProcess(os.Getpid())
	p.Signal(os.Interrupt)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon with extra workspace: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not exit")
	}
}

// --- makeCfgGetter tests ---

func TestMakeCfgGetterNoWatcher(t *testing.T) {
	cfg := testConfig(t.TempDir())
	getter := makeCfgGetter(nil, cfg)
	got := getter()
	if got != cfg {
		t.Errorf("expected fallback config")
	}
}

func TestMakeCfgGetterWithWatcherNoConfig(t *testing.T) {
	w, err := baked.NewWatcher(func() (*config.File, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	fallback := testConfig(t.TempDir())
	getter := makeCfgGetter(w, fallback)
	if got := getter(); got != fallback {
		t.Errorf("should return fallback when watcher has nil config")
	}
}

func TestMakeCfgGetterWithWatcherHasConfig(t *testing.T) {
	w, err := baked.NewWatcher(func() (*config.File, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	watcherCfg := testConfig(t.TempDir())
	w.SetConfig(watcherCfg)
	fallback := testConfig(t.TempDir())
	getter := makeCfgGetter(w, fallback)
	if got := getter(); got != watcherCfg {
		t.Errorf("should return watcher config when set")
	}
}

// --- serve tests ---

func TestServeAcceptsAndStops(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "sv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	sockPath := filepath.Join(dir, "test.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	workspaces := map[string]*workspace{dir: ws}
	sigCh := make(chan os.Signal, 1)

	done := make(chan struct{})
	go func() {
		serve(listener, nil, dir, getCfg, resolveWs, workspaces, sigCh)
		close(done)
	}()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	req := daemonproto.Request{Run: "build"}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	conn.Write(b)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("no response from serve")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	conn.Close()
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d err=%q", resp.ExitCode, resp.Error)
	}

	sigCh <- os.Interrupt
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeMultipleConnections(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "sv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	sockPath := filepath.Join(dir, "test.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	workspaces := map[string]*workspace{dir: ws}
	sigCh := make(chan os.Signal, 1)

	go serve(listener, nil, dir, getCfg, resolveWs, workspaces, sigCh)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("unix", sockPath)
			if err != nil {
				return
			}
			defer conn.Close()
			req := daemonproto.Request{Run: "build"}
			b, _ := json.Marshal(req)
			b = append(b, '\n')
			conn.Write(b)
			scanner := bufio.NewScanner(conn)
			scanner.Scan()
		}()
	}
	wg.Wait()
	sigCh <- os.Interrupt
}

func TestServeWithTCPListener(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "sv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)

	unixSock := filepath.Join(dir, "test.sock")
	unixLn, err := net.Listen("unix", unixSock)
	if err != nil {
		t.Fatal(err)
	}
	tcpLn, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := tcpLn.Addr().String()

	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	workspaces := map[string]*workspace{dir: ws}
	sigCh := make(chan os.Signal, 1)

	go serve(unixLn, tcpLn, dir, getCfg, resolveWs, workspaces, sigCh)

	// Connect via TCP
	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatalf("dial tcp: %v", err)
	}
	req := daemonproto.Request{Run: "build"}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	conn.Write(b)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("no response via TCP")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	conn.Close()
	if resp.ExitCode != 0 {
		t.Errorf("TCP: expected exit code 0, got %d err=%q", resp.ExitCode, resp.Error)
	}

	sigCh <- os.Interrupt
}

// --- handleConn tests ---

func TestHandleConnRunTarget(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	server, client := net.Pipe()
	defer client.Close()

	go handleConn(server, dir, getCfg, resolveWs, &mu)

	req := daemonproto.Request{Run: "build"}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	client.Write(b)

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response from handleConn")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (err=%q)", resp.ExitCode, resp.Error)
	}
}

func TestHandleConnBadJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	server, client := net.Pipe()
	defer client.Close()

	go handleConn(server, dir, getCfg, resolveWs, &mu)

	client.Write([]byte("not json\n"))

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response from handleConn")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 2 {
		t.Errorf("expected exit code 2 for bad JSON, got %d", resp.ExitCode)
	}
}

func TestHandleConnNilConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	nilWs := &workspace{rootDir: dir, cfg: nil}
	getCfg := func() *config.File { return nil }
	resolveWs := defaultResolveWs(nilWs)
	var mu sync.Mutex

	server, client := net.Pipe()
	defer client.Close()

	go handleConn(server, dir, getCfg, resolveWs, &mu)

	req := daemonproto.Request{Run: "build"}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	client.Write(b)

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response from handleConn")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 2 || resp.Error != "no config loaded" {
		t.Errorf("expected code=2, error='no config loaded', got code=%d err=%q", resp.ExitCode, resp.Error)
	}
}

func TestHandleConnNoData(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	server, client := net.Pipe()
	go handleConn(server, dir, getCfg, resolveWs, &mu)
	client.Close()
	time.Sleep(50 * time.Millisecond)
}

func TestHandleConnSerializesRequests(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server, client := net.Pipe()
			defer client.Close()
			go handleConn(server, dir, getCfg, resolveWs, &mu)
			req := daemonproto.Request{Run: "build"}
			b, _ := json.Marshal(req)
			b = append(b, '\n')
			client.Write(b)
			scanner := bufio.NewScanner(client)
			scanner.Scan()
		}()
	}
	wg.Wait()
}

func TestHandleConnStatusRequest(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	server, client := net.Pipe()
	defer client.Close()

	go handleConn(server, dir, getCfg, resolveWs, &mu)

	req := daemonproto.Request{Status: true}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	client.Write(b)

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 0 {
		t.Errorf("status: exit=%d err=%q", resp.ExitCode, resp.Error)
	}
	if resp.Output == "" {
		t.Error("expected non-empty status output")
	}
}

func TestHandleConnReloadRequest(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	var mu sync.Mutex

	server, client := net.Pipe()
	defer client.Close()

	go handleConn(server, dir, getCfg, resolveWs, &mu)

	req := daemonproto.Request{Reload: true}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	client.Write(b)

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 0 {
		t.Errorf("reload: exit=%d err=%q", resp.ExitCode, resp.Error)
	}
}

// --- runRequest tests ---

func TestRunRequestRunTarget(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Run: "build"})
	if code != 0 || errMsg != "" {
		t.Errorf("run build: code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestRunSuite(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Run: "dev"})
	if code != 0 || errMsg != "" {
		t.Errorf("run suite dev: code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestRunUnknown(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Run: "nope"})
	if code != 1 || errMsg == "" {
		t.Errorf("expected code 1 with error, got code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestUp(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "up", Workflow: []string{"build"}},
			{Name: "build", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
	}
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Up: true})
	if code != 0 || errMsg != "" {
		t.Errorf("up: code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestUpNoTarget(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Up: true})
	if code != 1 || errMsg == "" {
		t.Errorf("expected code 1 with error, got code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestDown(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Down: true})
	if code != 0 || errMsg != "" {
		t.Errorf("down (empty state): code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestDownSpecificDaemon(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{Down: true, Daemon: "redis"})
	if code != 1 || errMsg == "" {
		t.Errorf("expected code 1 for unknown daemon, got code=%d err=%q", code, errMsg)
	}
}

func TestRunRequestEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)
	code, errMsg := runRequest(context.Background(), dir, cfg, &daemonproto.Request{})
	if code != 2 || errMsg != "missing run, up, or down in request" {
		t.Errorf("empty request: code=%d err=%q", code, errMsg)
	}
}

// --- writeResp tests ---

func TestWriteResp(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go writeResp(server, 0, "", "")

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response received")
	}
	var resp daemonproto.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ExitCode != 0 || resp.Error != "" {
		t.Errorf("resp = %+v, want {0, ''}", resp)
	}
}

func TestWriteRespWithError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go writeResp(server, 1, "something broke", "")

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response received")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.ExitCode != 1 || resp.Error != "something broke" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestWriteRespWithOutput(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go writeResp(server, 0, "", "status info")

	scanner := bufio.NewScanner(client)
	if !scanner.Scan() {
		t.Fatal("no response received")
	}
	var resp daemonproto.Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.Output != "status info" {
		t.Errorf("expected output 'status info', got %q", resp.Output)
	}
}

// --- queue replay tests ---

func TestReplayQueueEmpty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	replayQueue(ws)
}

func TestReplayQueueWithPending(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := testWorkspace(dir)
	ws.queue.Enqueue(daemonproto.Request{Run: "build"}, dir)
	if ws.queue.Len() != 1 {
		t.Fatalf("expected 1 queued item, got %d", ws.queue.Len())
	}
	replayQueue(ws)
	if ws.queue.Len() != 0 {
		t.Errorf("expected 0 queued items after replay, got %d", ws.queue.Len())
	}
}

// --- workspace tests ---

func TestWorkspaceGetCfg(t *testing.T) {
	dir := t.TempDir()
	ws := testWorkspace(dir)
	cfg := ws.getCfg()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.RootDir != dir {
		t.Errorf("expected rootDir %q, got %q", dir, cfg.RootDir)
	}
}

func TestWorkspaceSetCfg(t *testing.T) {
	dir := t.TempDir()
	ws := testWorkspace(dir)
	newCfg := &config.File{RootDir: "/new"}
	ws.setCfg(newCfg)
	if got := ws.getCfg(); got != newCfg {
		t.Error("setCfg did not update config")
	}
}

// --- multiFlag tests ---

func TestMultiFlagSet(t *testing.T) {
	var mf multiFlag
	mf.Set("/a")
	mf.Set("/b")
	if len(mf) != 2 {
		t.Errorf("expected 2 entries, got %d", len(mf))
	}
	if mf.String() != "/a,/b" {
		t.Errorf("unexpected String() = %q", mf.String())
	}
}

// --- initWorkspace tests ---

func TestInitWorkspaceNoWatch(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "iw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	ws, err := initWorkspace(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if ws.watcher != nil {
		t.Error("expected nil watcher with noWatch=true")
	}
	if ws.cfg == nil {
		t.Error("expected non-nil config")
	}
	if ws.queue == nil {
		t.Error("expected non-nil queue")
	}
}

func TestInitWorkspaceWithWatch(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "iw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	ws, err := initWorkspace(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.watcher.Close()
	if ws.watcher == nil {
		t.Error("expected non-nil watcher")
	}
}

func TestInitWorkspaceNoBakefile(t *testing.T) {
	dir := t.TempDir()
	_, err := initWorkspace(dir, true)
	if err == nil {
		t.Error("expected error with no Bakefile")
	}
}

// --- serve SIGUSR1/SIGUSR2 tests ---

func TestServeSIGUSR1(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "sv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	sockPath := filepath.Join(dir, "test.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(dir, "Bakefile"), []byte("target build { steps { exec [\"true\"] } }\n"), 0644)
	ws, err := initWorkspace(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.watcher.Close()

	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	workspaces := map[string]*workspace{dir: ws}
	sigCh := make(chan os.Signal, 2)

	done := make(chan struct{})
	go func() {
		serve(listener, nil, dir, getCfg, resolveWs, workspaces, sigCh)
		close(done)
	}()

	// Send SIGUSR1 to reload
	sigCh <- syscall.SIGUSR1
	time.Sleep(100 * time.Millisecond)

	// Then stop
	sigCh <- os.Interrupt
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop after SIGUSR1 + SIGINT")
	}
}

func TestServeSIGUSR2(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "sv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	sockPath := filepath.Join(dir, "test.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	ws := testWorkspace(dir)
	getCfg := func() *config.File { return ws.getCfg() }
	resolveWs := defaultResolveWs(ws)
	workspaces := map[string]*workspace{dir: ws}
	sigCh := make(chan os.Signal, 2)

	done := make(chan struct{})
	go func() {
		serve(listener, nil, dir, getCfg, resolveWs, workspaces, sigCh)
		close(done)
	}()

	// Send SIGUSR2 for status dump
	sigCh <- syscall.SIGUSR2
	time.Sleep(100 * time.Millisecond)

	sigCh <- os.Interrupt
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop after SIGUSR2 + SIGINT")
	}
}

// --- replayQueue with nil config ---

func TestReplayQueueNilConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	ws := &workspace{rootDir: dir, cfg: nil}
	q, _ := baked.NewQueue(dir)
	ws.queue = q
	ws.queue.Enqueue(daemonproto.Request{Run: "build"}, dir)
	replayQueue(ws)
	// Should return early without processing
	if ws.queue.Len() != 1 {
		t.Errorf("expected queue not drained when config nil, got %d", ws.queue.Len())
	}
}

func TestReplayQueueNilQueue(t *testing.T) {
	dir := t.TempDir()
	ws := &workspace{rootDir: dir, cfg: testConfig(dir), queue: nil}
	replayQueue(ws) // should not panic
}
