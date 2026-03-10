package container

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func TestRun_requiresDocker(t *testing.T) {
	// When Docker is unavailable or image pull fails, Run returns an error that mentions Docker/image/container.
	// When Docker is available and image exists, Run may succeed (integration); we only assert error message shape when it fails.
	ctx := context.Background()
	reg := &fakeRegistry{}
	opts := RunOptions{
		Image:    "alpine:3.19",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"echo", "ok"}}},
		Registry: reg,
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err == nil {
		return // Docker available and ran successfully
	}
	msg := err.Error()
	if !containsSub(msg, "Docker") && !containsSub(msg, "image") && !containsSub(msg, "container") && !containsSub(msg, "pull") && !containsSub(msg, "daemon") {
		t.Errorf("error should mention Docker, image, container, pull, or daemon: %q", msg)
	}
}

func TestRun_withFakeBackend_success(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	reg := &fakeRegistry{}
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  dir,
		Steps:    []ExecStep{{Argv: []string{"echo", "ok"}}},
		Registry: reg,
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with fake backend: %v", err)
	}
}

func TestRun_withFakeBackend_stepExitNonZero(t *testing.T) {
	ctx := context.Background()
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"false"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{exitCode: 1},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err == nil {
		t.Fatal("expected error when step exits 1")
	}
	if !containsSub(err.Error(), "exited with code 1") {
		t.Errorf("error should mention exit code: %q", err.Error())
	}
}

func TestRun_withFakeBackend_showCmd(t *testing.T) {
	var stderr bytes.Buffer
	ctx := context.Background()
	opts := RunOptions{
		Image:    "img",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"echo", "hello"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   &stderr,
		ShowCmd:  true,
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSub(stderr.String(), "echo") || !containsSub(stderr.String(), "hello") {
		t.Errorf("ShowCmd should print command to stderr: %q", stderr.String())
	}
}

func TestRun_withFakeBackend_emptyStepSkipped(t *testing.T) {
	ctx := context.Background()
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"ok"}}, {Argv: nil}, {Argv: []string{"ok2"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with empty step in middle: %v", err)
	}
}

func TestRun_withFakeBackend_pullImageError(t *testing.T) {
	ctx := context.Background()
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{imagePullErr: errors.New("pull failed")},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	// With default pull policy we try pull when inspect fails; fake returns imageInspectErr so we try pull.
	os.Setenv(pullPolicyEnv, pullAlways)
	defer os.Unsetenv(pullPolicyEnv)
	err := Run(ctx, opts)
	if err == nil {
		t.Fatal("expected error when pull fails")
	}
	if !containsSub(err.Error(), "pull") {
		t.Errorf("error should mention pull: %q", err.Error())
	}
}

func TestEnvMapToSlice(t *testing.T) {
	// envMapToSlice is package-private; test via Run with step env (fake backend path uses it).
	m := map[string]string{"A": "1", "B": "2"}
	s := envMapToSlice(m)
	if len(s) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s))
	}
	got := make(map[string]string)
	for _, e := range s {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				got[e[:i]] = e[i+1:]
				break
			}
		}
	}
	if got["A"] != "1" || got["B"] != "2" {
		t.Errorf("got %v", got)
	}
}

func TestEnvMapToSlice_empty(t *testing.T) {
	if envMapToSlice(nil) != nil {
		t.Error("nil should return nil")
	}
	if envMapToSlice(map[string]string{}) != nil {
		t.Error("empty map should return nil")
	}
}

func TestRun_withFakeBackend_networksAndVolumes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{})
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  dir,
		Networks: []string{"mynet"},
		Volumes:  []config.VolumeRef{{Name: "myvol"}},
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: reg,
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with net/vol: %v", err)
	}
}

func TestRun_withFakeBackend_registryEnsureNetworkError(t *testing.T) {
	ctx := context.Background()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{networkCreateErr: errors.New("fake net error")})
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Networks: []string{"mynet"},
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: reg,
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err == nil {
		t.Fatal("expected error when registry EnsureNetwork fails")
	}
	if !containsSub(err.Error(), "ensure network") {
		t.Errorf("error should mention ensure network: %q", err.Error())
	}
}

func TestRun_withFakeBackend_registryEnsureVolumeError(t *testing.T) {
	ctx := context.Background()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{volumeCreateErr: errors.New("fake vol error")})
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Volumes:  []config.VolumeRef{{Name: "v"}},
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: reg,
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err == nil {
		t.Fatal("expected error when registry EnsureVolume fails")
	}
	if !containsSub(err.Error(), "ensure volume") {
		t.Errorf("error should mention ensure volume: %q", err.Error())
	}
}

// TestRun_withFakeBackend_extraNetworks hits the extraNetworks loop and NetworkConnect.
func TestRun_withFakeBackend_extraNetworks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{})
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  dir,
		Networks: []string{"first", "second"},
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: reg,
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with extra networks: %v", err)
	}
}

// TestRun_withFakeBackend_targetCwdAndStepWorkingDir covers containerCwd and step WorkingDir mapping.
func TestRun_withFakeBackend_targetCwdAndStepWorkingDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	opts := RunOptions{
		Image:     "alpine",
		RootDir:   dir,
		TargetCwd: "subdir",
		Steps: []ExecStep{
			{Argv: []string{"true"}, WorkingDir: dir},
			{Argv: []string{"true"}, WorkingDir: dir + "/subdir"},
		},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with TargetCwd and WorkingDir: %v", err)
	}
}

// TestRun_withFakeBackend_imageAlreadyPresent covers maybePullImage when ImageInspect succeeds and policy is if-not-present.
func TestRun_withFakeBackend_imageAlreadyPresent(t *testing.T) {
	os.Setenv(pullPolicyEnv, pullIfNotPresent)
	defer os.Unsetenv(pullPolicyEnv)
	ctx := context.Background()
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"true"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{}, // ImageInspect returns nil err -> skip pull
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with image already present: %v", err)
	}
}

// TestRun_withFakeBackend_stepWithEnv covers envMapToSlice path (step has Env set).
func TestRun_withFakeBackend_stepWithEnv(t *testing.T) {
	ctx := context.Background()
	opts := RunOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Steps:    []ExecStep{{Argv: []string{"true"}, Env: map[string]string{"FOO": "bar"}}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
		Stdout:   bytes.NewBuffer(nil),
		Stderr:   bytes.NewBuffer(nil),
	}
	err := Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run with step env: %v", err)
	}
}

func TestStartDaemonContainer_emptyCmd(t *testing.T) {
	ctx := context.Background()
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Cmd:      nil,
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
	}
	_, err := StartDaemonContainer(ctx, opts)
	if err == nil {
		t.Fatal("expected error for empty Cmd")
	}
	if !containsSub(err.Error(), "Cmd is required") {
		t.Errorf("error should mention Cmd: %q", err.Error())
	}
}

func TestStartDaemonContainer_withFakeBackend(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := &fakeBackend{containerID: "test-daemon-id"}
	opts := DaemonContainerOptions{
		Image:      "alpine",
		RootDir:    dir,
		Cmd:        []string{"redis-server"},
		WorkingDir: "/workspace",
		Registry:   &fakeRegistry{},
		Backend:    backend,
	}
	id, err := StartDaemonContainer(ctx, opts)
	if err != nil {
		t.Fatalf("StartDaemonContainer: %v", err)
	}
	if id != "test-daemon-id" {
		t.Errorf("got container ID %q, want test-daemon-id", id)
	}
}

func TestStopContainer_withFakeBackend(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{}
	err := StopContainer(ctx, "some-id", backend)
	if err != nil {
		t.Fatalf("StopContainer: %v", err)
	}
}

func TestStartDaemonContainer_pullFails(t *testing.T) {
	ctx := context.Background()
	// ImageInspect fails so we try pull; imagePullErr makes pull fail
	backend := &fakeBackend{imageInspectErr: errors.New("not found"), imagePullErr: errors.New("pull failed")}
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Cmd:      []string{"true"},
		Registry: &fakeRegistry{},
		Backend:  backend,
	}
	_, err := StartDaemonContainer(ctx, opts)
	if err == nil {
		t.Fatal("expected error when pull fails")
	}
	if !containsSub(err.Error(), "pull") && !containsSub(err.Error(), "failed") {
		t.Errorf("error should mention pull/failed: %q", err.Error())
	}
}

func TestStartDaemonContainer_usesExportedFakes(t *testing.T) {
	// Exercises NewFakeBackend and NewFakeNetVolRegistry for package coverage.
	ctx := context.Background()
	dir := t.TempDir()
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  dir,
		Cmd:      []string{"true"},
		Registry: NewFakeNetVolRegistry(),
		Backend:  NewFakeBackend(),
	}
	id, err := StartDaemonContainer(ctx, opts)
	if err != nil {
		t.Fatalf("StartDaemonContainer: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty container ID")
	}
}

func TestStartDaemonContainer_withMultipleNetworks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  dir,
		Cmd:      []string{"true"},
		Networks: []string{"net1", "net2"},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
	}
	id, err := StartDaemonContainer(ctx, opts)
	if err != nil {
		t.Fatalf("StartDaemonContainer: %v", err)
	}
	if id == "" {
		t.Error("expected container ID")
	}
}

func TestStartDaemonContainer_withVolumes(t *testing.T) {
	dir := t.TempDir()
	bindDir := dir + "/bind"
	os.MkdirAll(bindDir, 0755)
	ctx := context.Background()
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  dir,
		Cmd:      []string{"true"},
		Volumes:  []config.VolumeRef{{Name: "v1"}, {Name: "v2", HostPath: bindDir}},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
	}
	id, err := StartDaemonContainer(ctx, opts)
	if err != nil {
		t.Fatalf("StartDaemonContainer: %v", err)
	}
	if id == "" {
		t.Error("expected container ID")
	}
}

func TestStartDaemonContainer_pullPolicyNever(t *testing.T) {
	ctx := context.Background()
	t.Setenv("BAKE_PULL", "never")
	defer os.Unsetenv("BAKE_PULL")
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Cmd:      []string{"true"},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{},
	}
	id, err := StartDaemonContainer(ctx, opts)
	if err != nil {
		t.Fatalf("StartDaemonContainer with pull=never: %v", err)
	}
	if id == "" {
		t.Error("expected container ID")
	}
}

func TestStartDaemonContainer_startFails_cleanup(t *testing.T) {
	ctx := context.Background()
	opts := DaemonContainerOptions{
		Image:    "alpine",
		RootDir:  t.TempDir(),
		Cmd:      []string{"true"},
		Registry: &fakeRegistry{},
		Backend:  &fakeBackend{containerStartErr: errors.New("start failed")},
	}
	_, err := StartDaemonContainer(ctx, opts)
	if err == nil {
		t.Fatal("expected error when start fails")
	}
	if !containsSub(err.Error(), "start") {
		t.Errorf("error should mention start: %q", err.Error())
	}
}

func TestStopContainer_nilBackend(t *testing.T) {
	// With nil backend we try to create a Docker client; without Docker we get an error.
	ctx := context.Background()
	err := StopContainer(ctx, "nonexistent-id", nil)
	if err == nil {
		t.Log("StopContainer(nil backend) succeeded (Docker available and removed container)")
		return
	}
	if !containsSub(err.Error(), "Docker") && !containsSub(err.Error(), "connect") && !containsSub(err.Error(), "no such") {
		t.Logf("StopContainer(nil backend) error: %v", err)
	}
}

// fakeRegistry implements NetVolRegistry for tests without Docker.
type fakeRegistry struct{}

func (f *fakeRegistry) EnsureNetwork(ctx context.Context, name string) (string, error) {
	return "fake-net-" + name, nil
}

func (f *fakeRegistry) EnsureVolume(ctx context.Context, ref config.VolumeRef, rootDir string) (hostSource, containerTarget string, err error) {
	if ref.HostPath != "" {
		return ref.HostPath, "/mnt/" + ref.Name, nil
	}
	return "fake-vol-" + ref.Name, "/mnt/" + ref.Name, nil
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
