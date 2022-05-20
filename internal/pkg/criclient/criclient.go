package criclient

import (
	"fmt"
	"strings"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/containerd"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/crio"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/docker"

	kapi "k8s.io/api/core/v1"
)

const (
	criDocker     = "docker"
	criContainerd = "containerd"
	criCrio       = "cri-o"

	separator = ":"
)

// CRIClient is common interface for various CRI clients (containerd, CRI-O, Docker)
type CRIClient interface {
	GetPidForContainer(string) (int, error)
}

// NewCRIClient returns new CRIClient for specified Node
func NewCRIClient(node *kapi.Node) (CRIClient, error) {
	critype := strings.Split(node.Status.NodeInfo.ContainerRuntimeVersion, separator)[0]

	switch critype {
	case criDocker:
		return docker.NewClient()
	case criContainerd:
		return containerd.NewClient()
	case criCrio:
		return crio.NewClient()
	}

	return nil, fmt.Errorf("Unsupported CRI type: %s", critype)
}
