package crio

import (
	"strings"

	cacrio "github.com/google/cadvisor/container/crio"
)


const (
	separator = "://"
)

// Client is a client for contanierd that implements our common CRI interface
type Client struct {
	cli cacrio.CrioClient
}

// GetPidForContainer returns PID for provided container (ID)
func (c *Client) GetPidForContainer(id string) (int, error) {
	if strings.Contains(id, separator) {
		id = strings.Split(id, separator)[1]
	}
	ci, err := c.cli.ContainerInfo(id)
	if err != nil {
		return 0, err
	}
	return ci.Pid, nil
}

// NewClient returns new containerd client
func NewClient() (*Client, error) {
	cc, err := cacrio.Client()
	if err != nil {
		return nil, err
	}
	return &Client{
		cli: cc,
	}, nil
}
