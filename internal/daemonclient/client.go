package daemonclient

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/evmac/go-bake/internal/daemonproto"
)

const dialTimeout = 2 * time.Second

// waitForSocket must be generous for slow CI, -race, and cold start of baked.
const waitForSocket = 10 * time.Second

// SocketPath returns the baked socket path for the given root dir.
func SocketPath(rootDir string) string {
	return filepath.Join(rootDir, ".bake", "baked.sock")
}

// Dial connects to the baked daemon at rootDir. Returns nil if baked is not running or connection fails.
func Dial(rootDir string) (net.Conn, error) {
	sock := SocketPath(rootDir)
	conn, err := net.DialTimeout("unix", sock, dialTimeout)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// EnsureStarted starts baked if the socket is not present, then waits for it. Returns nil when socket is ready.
func EnsureStarted(rootDir string) error {
	sock := SocketPath(rootDir)
	if _, err := os.Stat(sock); err == nil {
		return nil
	}
	bakedExe := "baked"
	if exe, err := exec.LookPath("baked"); err == nil {
		bakedExe = exe
	}
	cmd := exec.Command(bakedExe)
	cmd.Dir = rootDir
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Process.Release()
	deadline := time.Now().Add(waitForSocket)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(sock); err == nil {
			return nil
		}
	}
	return nil
}

// SendRequest sends req to the daemon on conn and returns the response.
func SendRequest(conn net.Conn, req *daemonproto.Request) (*daemonproto.Response, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	var resp daemonproto.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
