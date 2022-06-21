package bandwidth

import (
	"fmt"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	IngressBandwidthAnnotation = "kubernetes.io/ingress-bandwidth"
	EgressBandwidthAnnotation  = "kubernetes.io/egress-bandwidth"
)

type Bandwidth struct {
	Ingress string `protobuf:"bytes,2,opt,name=ingress,proto3" json:"ingress,omitempty"`
	Egress  string `protobuf:"bytes,2,opt,name=egress,proto3" json:"egress,omitempty"`
}

func GetBandwidth(namespace, name string) (*Bandwidth, error) {
	k, err := kube.GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("error in kubeclientset:%v", err)
	}

	kubecli := &kube.Kube{KClient: k}

	pod, err := kubecli.GetPod(namespace, name)
	if err != nil {
		return nil, fmt.Errorf("error during getting pod data:%v", err)
	}

	return &Bandwidth{
		pod.Annotations[IngressBandwidthAnnotation],
		pod.Annotations[EgressBandwidthAnnotation]}, nil
}

func ParseBandwidthAsRate(bandwidth string) (int64, error) {
	return parseBandwidth(bandwidth)
}

func ParseBandwidthAsRateInKbps(bandwidth string) (int64, error) {
	bandwidthInt, err := parseBandwidth(bandwidth)
	if err != nil {
		return 0, nil
	}

	return bandwidthInt / 1000, nil
}

func parseBandwidth(bandwidth string) (int64, error) {
	bandwidthQuantity, err := resource.ParseQuantity(bandwidth)
	if err != nil {
		return 0, fmt.Errorf("parsing bandwidth %s failed: %s", bandwidth, err)
	}

	bandwidthInt, ok := bandwidthQuantity.AsInt64()
	if !ok {
		return 0, fmt.Errorf("parsing bandwidth %s as Int64 failed: %s", bandwidth, err)
	}

	return bandwidthInt, nil
}

func ComputeBurst(ingressPolicingRate int64) int64 {
	return ingressPolicingRate / 10
}
