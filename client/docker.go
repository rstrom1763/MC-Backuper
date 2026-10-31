package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerClient interface {
	RunCommand(ctx context.Context, containerName string, command string) (string, error)
	IsContainerRunning(ctx context.Context, containerName string) (bool, error)
	Close() error
}

type client struct {
	cli *dockerClient.Client
}

func NewDockerClient() (DockerClient, error) {
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("could not create Docker client: %w", err)
	}
	return &client{cli: cli}, nil
}

func (c *client) Close() error {
	return c.cli.Close()
}

func (c *client) RunCommand(ctx context.Context, containerName string, command string) (string, error) {
	// Minecraft commands must be sent via rcon-cli or similar inside the container.
	finalCmd := []string{"rcon-cli", command}

	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          finalCmd,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", fmt.Errorf("could not create exec instance: %w", err)
	}

	resp, err := c.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("could not attach to exec instance: %w", err)
	}
	defer resp.Close()

	var outBuf, errBuf strings.Builder
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("could not demultiplex output: %w", err)
	}

	stdout := outBuf.String()
	stderr := errBuf.String()

	if stderr != "" {
		return stdout, fmt.Errorf("exec error: %s", stderr)
	}

	return stdout, nil
}

func (c *client) IsContainerRunning(ctx context.Context, containerName string) (bool, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return false, fmt.Errorf("could not list containers: %w", err)
	}

	for _, containerItem := range containers {
		if slices.Contains(containerItem.Names, "/"+containerName) {
			return true, nil
		}
	}
	return false, nil
}
