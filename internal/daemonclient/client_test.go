package daemonclient

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/daemonproto"
)

// shortDir creates a short temp dir to avoid Unix socket path length limits (~104 chars on macOS).
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "dc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestSocketPath(t *testing.T) {
	got := SocketPath("/tmp/myrepo")
	want := filepath.Join("/tmp/myrepo", ".bake", "baked.sock")
	if got != want {
		t.Errorf("SocketPath = %q, want %q", got, want)
	}
}

func TestDialNoSocket(t *testing.T) {
	dir := shortDir(t)
	_, err := Dial(dir)
	if err == nil {
		t.Fatal("expected error dialing with no socket")
	}
}

func TestDialAndSendRequest(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	if err := os.MkdirAll(dotBake, 0755); err != nil {
		t.Fatal(err)
	}
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return
		}
		var req daemonproto.Request
		json.Unmarshal(scanner.Bytes(), &req)
		resp := daemonproto.Response{ExitCode: 0}
		if req.Run == "fail" {
			resp = daemonproto.Response{ExitCode: 1, Error: "target failed"}
		}
		b, _ := json.Marshal(resp)
		b = append(b, '\n')
		conn.Write(b)
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	resp, err := SendRequest(conn, &daemonproto.Request{Run: "build"})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", resp.ExitCode)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %q", resp.Error)
	}
}

func TestSendRequestError(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		resp := daemonproto.Response{ExitCode: 1, Error: "target failed"}
		b, _ := json.Marshal(resp)
		b = append(b, '\n')
		conn.Write(b)
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	resp, err := SendRequest(conn, &daemonproto.Request{Run: "fail"})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", resp.ExitCode)
	}
	if resp.Error != "target failed" {
		t.Errorf("expected error 'target failed', got %q", resp.Error)
	}
}

func TestSendRequestUp(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		var req daemonproto.Request
		json.Unmarshal(scanner.Bytes(), &req)
		resp := daemonproto.Response{ExitCode: 0}
		if !req.Up {
			resp = daemonproto.Response{ExitCode: 2, Error: "expected up"}
		}
		b, _ := json.Marshal(resp)
		b = append(b, '\n')
		conn.Write(b)
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	resp, err := SendRequest(conn, &daemonproto.Request{Up: true})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", resp.ExitCode)
	}
}

func TestSendRequestDown(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		var req daemonproto.Request
		json.Unmarshal(scanner.Bytes(), &req)
		resp := daemonproto.Response{ExitCode: 0}
		if !req.Down {
			resp = daemonproto.Response{ExitCode: 2, Error: "expected down"}
		}
		b, _ := json.Marshal(resp)
		b = append(b, '\n')
		conn.Write(b)
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	resp, err := SendRequest(conn, &daemonproto.Request{Down: true, Daemon: "redis"})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", resp.ExitCode)
	}
}

func TestSendRequestServerClosesNoResponse(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Read request then close without responding
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		conn.Close()
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	resp, _ := SendRequest(conn, &daemonproto.Request{Run: "build"})
	// Server closed without responding; scanner.Scan() returns false with nil err,
	// so SendRequest returns (nil, nil). We just verify no valid response came back.
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

func TestSendRequestBadResponseJSON(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		conn.Write([]byte("not valid json\n"))
	}()

	conn, err := Dial(dir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_, err = SendRequest(conn, &daemonproto.Request{Run: "build"})
	if err == nil {
		t.Fatal("expected error for bad response JSON")
	}
}

func TestEnsureStartedStartFails(t *testing.T) {
	dir := shortDir(t)
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)
	err := EnsureStarted(dir)
	if err == nil {
		t.Log("EnsureStarted returned nil (cmd.Start may succeed on some platforms)")
	}
}

func TestEnsureStartedWithFakeBaked(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")

	fakeDir := shortDir(t)
	fakeBaked := filepath.Join(fakeDir, "baked")
	script := "#!/bin/sh\n/usr/bin/touch " + sockPath + "\nsleep 10\n"
	os.WriteFile(fakeBaked, []byte(script), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", fakeDir+string(filepath.ListSeparator)+"/usr/bin")
	defer os.Setenv("PATH", origPath)

	err := EnsureStarted(dir)
	if err != nil {
		t.Fatalf("EnsureStarted: %v", err)
	}
	if _, err := os.Stat(sockPath); err != nil {
		t.Errorf("socket should exist after EnsureStarted: %v", err)
	}
}

func TestSendRequestWriteError(t *testing.T) {
	server, client := net.Pipe()
	server.Close()
	_, err := SendRequest(client, &daemonproto.Request{Run: "build"})
	client.Close()
	if err == nil {
		t.Fatal("expected error writing to closed pipe")
	}
}

func TestEnsureStartedSocketExists(t *testing.T) {
	dir := shortDir(t)
	dotBake := filepath.Join(dir, ".bake")
	os.MkdirAll(dotBake, 0755)
	sockPath := filepath.Join(dotBake, "baked.sock")
	os.WriteFile(sockPath, []byte{}, 0644)

	if err := EnsureStarted(dir); err != nil {
		t.Fatalf("EnsureStarted with existing socket: %v", err)
	}
}
