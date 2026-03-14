// Package main is the baked daemon: background process that watches Bakefile(s),
// updates shims, and can run targets/up/down via a Unix socket (and optionally TCP).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/evmac/go-bake/internal/baked"
	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/daemonproto"
	"github.com/evmac/go-bake/internal/shim"
)

const socketName = "baked.sock"
const pidName = "baked.pid"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	noWatch := flag.Bool("no-watch", false, "disable Bakefile watching (for tests)")
	tcpAddr := flag.String("tcp", "", "also listen on TCP address (e.g. localhost:9876)")
	var extraWorkspaces multiFlag
	flag.Var(&extraWorkspaces, "workspace", "additional workspace directory (repeatable)")
	flag.Parse()
	return runDaemon(".", *noWatch, *tcpAddr, extraWorkspaces)
}

// multiFlag collects repeated flag values.
type multiFlag []string

func (f *multiFlag) String() string { return strings.Join(*f, ",") }
func (f *multiFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

// workspace holds per-workspace state: config, watcher, queue.
type workspace struct {
	rootDir string
	cfg     *config.File
	watcher *baked.Watcher
	queue   *baked.Queue
	mu      sync.Mutex // protects cfg
}

func (ws *workspace) getCfg() *config.File {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.watcher != nil {
		if c := ws.watcher.Config(); c != nil {
			return c
		}
	}
	return ws.cfg
}

func (ws *workspace) setCfg(cfg *config.File) {
	ws.mu.Lock()
	ws.cfg = cfg
	ws.mu.Unlock()
}

// runDaemon is the core daemon logic.
func runDaemon(dir string, noWatch bool, tcpAddr string, extraWorkspaces []string) error {
	primary, err := initWorkspace(dir, noWatch)
	if err != nil {
		return err
	}

	workspaces := map[string]*workspace{primary.rootDir: primary}
	for _, d := range extraWorkspaces {
		ws, err := initWorkspace(d, noWatch)
		if err != nil {
			log.Printf("workspace %s: %v (skipped)", d, err)
			continue
		}
		workspaces[ws.rootDir] = ws
	}
	defer func() {
		for _, ws := range workspaces {
			if ws.watcher != nil {
				ws.watcher.Close()
			}
		}
	}()

	dotBake := filepath.Join(primary.rootDir, ".bake")
	if err := os.MkdirAll(dotBake, 0755); err != nil {
		return fmt.Errorf("create .bake: %w", err)
	}

	sockPath := filepath.Join(dotBake, socketName)
	_ = os.Remove(sockPath)

	unixListener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", sockPath, err)
	}
	defer unixListener.Close()
	defer os.Remove(sockPath)

	pidPath := filepath.Join(dotBake, pidName)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	// Optional TCP listener
	var tcpListener net.Listener
	if tcpAddr != "" {
		tl, err := net.Listen("tcp", tcpAddr)
		if err != nil {
			return fmt.Errorf("listen on TCP %s: %w", tcpAddr, err)
		}
		tcpListener = tl
		defer tcpListener.Close()
		log.Printf("listening on TCP %s", tcpAddr)
	}

	// Health checker (sleep/wake lifecycle)
	hc := baked.StartHealthChecker(primary.rootDir)
	defer hc.Stop()

	// Replay any pending queue items from previous runs
	for _, ws := range workspaces {
		replayQueue(ws)
	}

	// Signal handling: SIGINT/SIGTERM/SIGHUP to stop, SIGUSR1 to reload, SIGUSR2 to dump status
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(sigCh)

	getCfg := func() *config.File { return primary.getCfg() }
	resolveWs := func(wsPath string) *workspace {
		if wsPath == "" {
			return primary
		}
		abs, _ := filepath.Abs(wsPath)
		if ws, ok := workspaces[abs]; ok {
			return ws
		}
		return primary
	}

	serve(unixListener, tcpListener, primary.rootDir, getCfg, resolveWs, workspaces, sigCh)

	for _, ws := range workspaces {
		if ws.watcher != nil {
			ws.watcher.Close()
		}
	}
	return nil
}

func initWorkspace(dir string, noWatch bool) (*workspace, error) {
	rootDir, cfg, err := baked.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := shim.WriteShims(rootDir, cfg, "bake"); err != nil {
		return nil, fmt.Errorf("write shims: %w", err)
	}

	dotBake := filepath.Join(rootDir, ".bake")
	os.MkdirAll(dotBake, 0755)

	q, _ := baked.NewQueue(rootDir)

	ws := &workspace{rootDir: rootDir, cfg: cfg, queue: q}

	if !noWatch {
		oldCfg := cfg
		watchPaths := []string{cfg.BakePath}
		for _, imp := range cfg.Imports {
			watchPaths = append(watchPaths, filepath.Join(rootDir, imp))
		}
		w, err := baked.NewWatcher(func() (*config.File, error) {
			_, newCfg, err := baked.Load(rootDir)
			if err != nil {
				return nil, err
			}
			if err := shim.WriteShims(rootDir, newCfg, "bake"); err != nil {
				log.Printf("shim update: %v", err)
			}
			// Hot reload: diff daemons and apply changes
			added, removed, changed := baked.DaemonDiff(oldCfg, newCfg)
			if len(added)+len(removed)+len(changed) > 0 {
				baked.ApplyDaemonDiff(context.Background(), rootDir, newCfg, added, removed, changed)
			}
			oldCfg = newCfg
			return newCfg, nil
		})
		if err != nil {
			return nil, fmt.Errorf("create watcher: %w", err)
		}
		ws.watcher = w
		if err := w.Add(watchPaths); err != nil {
			return nil, fmt.Errorf("watch paths: %w", err)
		}
		w.SetConfig(cfg)
		go func() {
			if err := w.Run(); err != nil {
				log.Printf("watcher: %v", err)
			}
		}()
	}

	return ws, nil
}

// makeCfgGetter returns a function that always returns the latest config.
func makeCfgGetter(watcher *baked.Watcher, fallback *config.File) func() *config.File {
	return func() *config.File {
		if watcher != nil {
			if c := watcher.Config(); c != nil {
				return c
			}
		}
		return fallback
	}
}

// serve accepts connections on listeners and handles them until a shutdown signal arrives.
func serve(unixLn, tcpLn net.Listener, rootDir string, getCfg func() *config.File, resolveWs func(string) *workspace, workspaces map[string]*workspace, sigCh <-chan os.Signal) {
	var runMu sync.Mutex
	done := make(chan struct{})

	acceptLoop := func(ln net.Listener) {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					log.Printf("accept: %v", err)
				}
				return
			}
			go handleConn(conn, rootDir, getCfg, resolveWs, &runMu)
		}
	}

	go acceptLoop(unixLn)
	if tcpLn != nil {
		go acceptLoop(tcpLn)
	}

	for sig := range sigCh {
		switch sig {
		case syscall.SIGUSR1:
			log.Printf("SIGUSR1: reloading config")
			for _, ws := range workspaces {
				if ws.watcher != nil {
					ws.watcher.ForceReload()
				}
			}
			continue
		case syscall.SIGUSR2:
			for _, ws := range workspaces {
				status := baked.StatusDump(ws.rootDir, nil)
				log.Print(status)
			}
			continue
		default:
			// SIGINT, SIGTERM, SIGHUP — shut down
			close(done)
			unixLn.Close()
			if tcpLn != nil {
				tcpLn.Close()
			}
			return
		}
	}
}

func handleConn(conn net.Conn, rootDir string, getCfg func() *config.File, resolveWs func(string) *workspace, runMu *sync.Mutex) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	var req daemonproto.Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeResp(conn, 2, err.Error(), "")
		return
	}

	// Status request
	if req.Status {
		ws := resolveWs(req.Workspace)
		status := baked.StatusDump(ws.rootDir, nil)
		writeResp(conn, 0, "", status)
		return
	}

	// Reload request
	if req.Reload {
		ws := resolveWs(req.Workspace)
		if ws.watcher != nil {
			ws.watcher.ForceReload()
		}
		writeResp(conn, 0, "", "")
		return
	}

	ws := resolveWs(req.Workspace)
	c := ws.getCfg()
	if c == nil {
		writeResp(conn, 2, "no config loaded", "")
		return
	}

	// Persistent queue: enqueue, execute, dequeue
	queueID := ""
	if ws.queue != nil {
		id, _ := ws.queue.Enqueue(req, ws.rootDir)
		queueID = id
	}

	runMu.Lock()
	exitCode, errMsg := runRequest(context.Background(), ws.rootDir, c, &req)
	runMu.Unlock()

	if ws.queue != nil && queueID != "" {
		_ = ws.queue.Dequeue(queueID)
	}

	writeResp(conn, exitCode, errMsg, "")
}

func replayQueue(ws *workspace) {
	if ws.queue == nil {
		return
	}
	pending := ws.queue.Pending()
	if len(pending) == 0 {
		return
	}
	log.Printf("replaying %d queued items for %s", len(pending), ws.rootDir)
	c := ws.getCfg()
	if c == nil {
		return
	}
	for _, entry := range pending {
		exitCode, errMsg := runRequest(context.Background(), ws.rootDir, c, &entry.Request)
		if errMsg != "" {
			log.Printf("queue replay %s: exit=%d err=%s", entry.ID, exitCode, errMsg)
		}
		_ = ws.queue.Dequeue(entry.ID)
	}
}

func runRequest(ctx context.Context, rootDir string, cfg *config.File, req *daemonproto.Request) (exitCode int, errMsg string) {
	if req.Up {
		if err := baked.RunUp(ctx, cfg); err != nil {
			return 1, err.Error()
		}
		return 0, ""
	}
	if req.Down {
		if err := baked.RunDown(rootDir, req.Daemon); err != nil {
			return 1, err.Error()
		}
		return 0, ""
	}
	if req.Run != "" {
		if cfg.SuiteByName(req.Run) != nil {
			if err := baked.RunSuite(ctx, cfg, req.Run); err != nil {
				return 1, err.Error()
			}
			return 0, ""
		}
		if err := baked.RunTarget(ctx, cfg, req.Run); err != nil {
			return 1, err.Error()
		}
		return 0, ""
	}
	return 2, "missing run, up, or down in request"
}

func writeResp(conn net.Conn, exitCode int, errMsg, output string) {
	resp := daemonproto.Response{ExitCode: exitCode, Error: errMsg, Output: output}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	conn.Write(b)
}
