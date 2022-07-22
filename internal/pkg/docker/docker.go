package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

// Client is a client for docker that implements our common CRI interface
type Client struct {
	cli *client.Client
}

// GetPidForContainer returns PID for provided container (ID)
func (dc *Client) GetPidForContainer(id string) (int, error) {
	dc.cli.NegotiateAPIVersion(context.Background())
	cj, err := dc.cli.ContainerInspect(context.Background(), id)
	if err != nil {
		fmt.Println("Unable to Inspect docker container")
		return -1, err
	}
	if cj.State.Pid == 0 {
		return -1, fmt.Errorf("Container not found %s", id)
	}
	return cj.State.Pid, nil
}

// NewClient returns new Docker client
func NewClient() (*Client, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	return &Client{
		cli: cli,
	}, nil
}
