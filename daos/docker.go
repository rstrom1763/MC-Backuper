package daos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerDAO interface {
	RunCommand(ctx context.Context, containerName string, command string) (string, error)
	IsContainerRunning(ctx context.Context, containerName string) (bool, error)
	Close() error
}

type dockerDAO struct {
	cli *client.Client
}

func NewDockerDAO() (DockerDAO, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("could not create Docker client: %w", err)
	}
	return &dockerDAO{cli: cli}, nil
}

func (d *dockerDAO) Close() error {
	return d.cli.Close()
}

func (d *dockerDAO) RunCommand(ctx context.Context, containerName string, command string) (string, error) {
	// Minecraft commands must be sent via rcon-cli or similar inside the container.
	finalCmd := []string{"rcon-cli", command}

	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          finalCmd,
	}

	execID, err := d.cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", fmt.Errorf("could not create exec instance: %w", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
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

func (d *dockerDAO) IsContainerRunning(ctx context.Context, containerName string) (bool, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return false, fmt.Errorf("could not list containers: %w", err)
	}

	for _, c := range containers {
		if slices.Contains(c.Names, "/"+containerName) {
			return true, nil
		}
	}
	return false, nil
}
