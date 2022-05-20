package containerd

import (
	"context"
	"strings"

	ctd "github.com/google/cadvisor/container/containerd"
	"github.com/google/cadvisor/container/containerd/namespaces"
)

const (
	defaultContainerdSocket = "/var/run/containerd/containerd.sock"

	separator = "://"
)

// Client is a client for contanierd that implements our common CRI interface
type Client struct {
	cli ctd.ContainerdClient
	ctx context.Context
}

// GetPidForContainer returns PID for provided container (ID)
func (c *Client) GetPidForContainer(id string) (int, error) {
	if strings.Contains(id, separator) {
		id = strings.Split(id, separator)[1]
	}
	pid, err := c.cli.TaskPid(c.ctx, id)
	if err != nil {
		return -1, err
	}
	return int(pid), nil
}

// NewClient returns new containerd client
func NewClient() (*Client, error) {
	cc, err := ctd.Client(defaultContainerdSocket,"")
	if err != nil {
		return nil, err
	}
	return &Client{
		cli: cc,
		ctx: namespaces.NamespaceFromEnv(context.TODO()),
	}, nil
}
