package client_test

import (
	"context"
	"testing"

	"github.com/rstrom1763/MC-Backuper/client"
)

// mockDockerClient implements client.DockerClient for testing
type mockDockerClient struct {
	running bool
	output  string
	err     error
}

func (m *mockDockerClient) RunCommand(ctx context.Context, containerName string, command string) (string, error) {
	return m.output, m.err
}

func (m *mockDockerClient) IsContainerRunning(ctx context.Context, containerName string) (bool, error) {
	return m.running, m.err
}

func (m *mockDockerClient) Close() error {
	return nil
}

func TestDockerClientInterface(t *testing.T) {
	var _ client.DockerClient = (*mockDockerClient)(nil)
}
