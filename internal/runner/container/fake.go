package container

import (
	"bytes"
	"context"
	"io"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

// fakeBackend implements DockerBackend for tests without a Docker daemon.
type fakeBackend struct {
	imageInspectErr error
	imagePullErr    error
	containerID     string
	execID          string
	exitCode        int
}

func (f *fakeBackend) ImageInspect(ctx context.Context, image string, opts ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, f.imageInspectErr
}

func (f *fakeBackend) ImagePull(ctx context.Context, ref string, opts client.ImagePullOptions) (imagePullResponse, error) {
	if f.imagePullErr != nil {
		return nil, f.imagePullErr
	}
	return &fakePullResponse{}, nil
}

func (f *fakeBackend) ContainerCreate(ctx context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	id := f.containerID
	if id == "" {
		id = "fake-container-id"
	}
	return client.ContainerCreateResult{ID: id}, nil
}

func (f *fakeBackend) ContainerRemove(ctx context.Context, containerID string, opts client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (f *fakeBackend) ContainerStart(ctx context.Context, containerID string, opts client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, nil
}

func (f *fakeBackend) NetworkConnect(ctx context.Context, networkID string, opts client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	return client.NetworkConnectResult{}, nil
}

func (f *fakeBackend) ExecCreate(ctx context.Context, containerID string, opts client.ExecCreateOptions) (client.ExecCreateResult, error) {
	eid := f.execID
	if eid == "" {
		eid = "fake-exec-id"
	}
	return client.ExecCreateResult{ID: eid}, nil
}

func (f *fakeBackend) ExecAttach(ctx context.Context, execID string, opts client.ExecAttachOptions) (ExecAttachStream, error) {
	// Return a stream that reads to EOF immediately (no output); StdCopy will finish.
	return &fakeAttachStream{Reader: bytes.NewReader(nil)}, nil
}

func (f *fakeBackend) ExecInspect(ctx context.Context, execID string, opts client.ExecInspectOptions) (client.ExecInspectResult, error) {
	ec := f.exitCode
	return client.ExecInspectResult{ExitCode: ec}, nil
}

type fakePullResponse struct {
	io.Reader
}

func (f *fakePullResponse) Read(p []byte) (n int, err error) { return 0, io.EOF }
func (f *fakePullResponse) Close() error                    { return nil }
func (f *fakePullResponse) Wait(ctx context.Context) error   { return nil }

type fakeAttachStream struct {
	io.Reader
}

func (f *fakeAttachStream) Read(p []byte) (n int, err error) { return f.Reader.Read(p) }
func (f *fakeAttachStream) Close()                           {}

// fakeNetVolClient implements dockerNetVolClient for registry tests.
type fakeNetVolClient struct {
	networkCreateErr error
	volumeCreateErr  error
	netID            string
}

func (f *fakeNetVolClient) NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
	if f.networkCreateErr != nil {
		return client.NetworkCreateResult{}, f.networkCreateErr
	}
	id := f.netID
	if id == "" {
		id = "fake-net-" + name
	}
	return client.NetworkCreateResult{ID: id}, nil
}

func (f *fakeNetVolClient) VolumeCreate(ctx context.Context, options client.VolumeCreateOptions) (client.VolumeCreateResult, error) {
	if f.volumeCreateErr != nil {
		return client.VolumeCreateResult{}, f.volumeCreateErr
	}
	return client.VolumeCreateResult{
		Volume: volume.Volume{Name: options.Name, Driver: "local", Labels: map[string]string{}, Mountpoint: "/var/lib/docker/volumes/" + options.Name, Options: map[string]string{}},
	}, nil
}
