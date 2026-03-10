package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func TestDockerNetVolRegistry_EnsureNetwork_caches(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNetVolClient{netID: "my-net-id"}
	reg := NewDockerNetVolRegistry(fake)
	id1, err := reg.EnsureNetwork(ctx, "mynet")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := reg.EnsureNetwork(ctx, "mynet")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("second call should return same id: %q vs %q", id1, id2)
	}
	if id1 != "my-net-id" {
		t.Errorf("expected my-net-id, got %q", id1)
	}
}

func TestDockerNetVolRegistry_EnsureNetwork_createError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNetVolClient{networkCreateErr: errFake}
	reg := NewDockerNetVolRegistry(fake)
	_, err := reg.EnsureNetwork(ctx, "mynet")
	if err != errFake {
		t.Errorf("expected errFake, got %v", err)
	}
}

var errFake = &fakeError{"fake"}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

func TestDockerNetVolRegistry_EnsureVolume_bindMount(t *testing.T) {
	ctx := context.Background()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{})
	dir := t.TempDir()
	ref := config.VolumeRef{Name: "data", HostPath: "sub"}
	src, tgt, err := reg.EnsureVolume(ctx, ref, dir)
	if err != nil {
		t.Fatal(err)
	}
	absSub := filepath.Join(dir, "sub")
	absExpected, _ := filepath.Abs(absSub)
	if src != absExpected {
		t.Errorf("source want %q got %q", absExpected, src)
	}
	if tgt != "/mnt/data" {
		t.Errorf("target want /mnt/data got %q", tgt)
	}
}

func TestDockerNetVolRegistry_EnsureVolume_namedCaches(t *testing.T) {
	ctx := context.Background()
	reg := NewDockerNetVolRegistry(&fakeNetVolClient{})
	dir := t.TempDir()
	ref := config.VolumeRef{Name: "myvol"}
	src1, tgt1, err := reg.EnsureVolume(ctx, ref, dir)
	if err != nil {
		t.Fatal(err)
	}
	src2, tgt2, err := reg.EnsureVolume(ctx, ref, dir)
	if err != nil {
		t.Fatal(err)
	}
	if src1 != src2 || tgt1 != tgt2 {
		t.Errorf("second call should return same: %q,%q vs %q,%q", src1, tgt1, src2, tgt2)
	}
	if tgt1 != "/mnt/myvol" {
		t.Errorf("target want /mnt/myvol got %q", tgt1)
	}
}

func TestDockerNetVolRegistry_EnsureVolume_createError(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNetVolClient{volumeCreateErr: errFake}
	reg := NewDockerNetVolRegistry(fake)
	_, _, err := reg.EnsureVolume(ctx, config.VolumeRef{Name: "v"}, t.TempDir())
	if err != errFake {
		t.Errorf("expected errFake, got %v", err)
	}
}

// TestNewDockerRegistryFromEnv_error covers the error path when the client cannot be created.
func TestNewDockerRegistryFromEnv_error(t *testing.T) {
	// DOCKER_HOST with an invalid URL can cause NewClientWithOpts to fail on some platforms.
	orig := os.Getenv("DOCKER_HOST")
	os.Setenv("DOCKER_HOST", "invalid://bad")
	defer func() { os.Setenv("DOCKER_HOST", orig) }()
	_, closeFn, err := NewDockerRegistryFromEnv()
	if err != nil {
		return // error path covered
	}
	if closeFn != nil {
		closeFn()
	}
	// If we get here, client was created (e.g. Docker tolerates invalid host); skip.
	t.Skip("DOCKER_HOST=invalid did not produce an error on this system")
}
