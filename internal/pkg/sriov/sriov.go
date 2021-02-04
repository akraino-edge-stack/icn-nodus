package sriov

import (
        "fmt"
        "context"
        "net"
        "time"
        "github.com/containernetworking/cni/pkg/skel"
        "github.com/containernetworking/cni/pkg/types"
        "k8s.io/kubernetes/pkg/kubelet/util"
        "k8s.io/kubernetes/pkg/kubelet/apis/podresources"
        podresourcesv1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
        utils "github.com/k8snetworkplumbingwg/sriov-cni/pkg/utils"
        logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
        // Kubelet internal cgroup name for node allocatable cgroup.
        defaultNodeAllocatableCgroup = "kubepods"
        // defaultPodResourcesPath is the path to the local endpoint serving the podresources GRPC service.
        defaultPodResourcesPath    = "/var/lib/kubelet/pod-resources"
        defaultPodResourcesTimeout = 10 * time.Second
        defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
)

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
        types.CommonArgs
        IP                         net.IP
        K8S_POD_NAME               types.UnmarshallableString
        K8S_POD_NAMESPACE          types.UnmarshallableString
        K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

type KubeletPodResources struct {
        resources []*podresourcesv1alpha1.PodResources
}

var log = logf.Log.WithName("sriov")

// GetK8sArgs gets k8s related args from CNI args
func GetK8sArgs(args *skel.CmdArgs) (*K8sArgs, error) {
        k8sArgs := &K8sArgs{}

        err := types.LoadArgs(args.Args, k8sArgs)
        if err != nil {
                return nil, err
        }

        return k8sArgs, nil
}

// Inspired by and based on:
// https://github.com/kubernetes/kubernetes/blob/master/test/e2e_node/util.go#L67-L73
// https://github.com/kubernetes/kubernetes/blob/master/test/e2e_node/util.go#L110-L127
// https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/pkg/k8sclient/k8sclient.go#L296-L307
// https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/pkg/types/types.go#L153-L160
func GetNodeDevices() (*podresourcesv1alpha1.ListPodResourcesResponse, error) {

        endpoint := util.LocalEndpoint(defaultPodResourcesPath, podresources.Socket)
        if endpoint == "" {
              return nil, fmt.Errorf("Error getting local endpoint: %v", endpoint)
        }

        client, conn, err := podresources.GetClient(endpoint, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
        if err != nil {
                return nil, fmt.Errorf("Error getting grpc client: %v", err)
        }

        defer conn.Close()

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        resp, err := client.List(ctx, &podresourcesv1alpha1.ListPodResourcesRequest{})
        if err != nil {
                return nil, fmt.Errorf("%v.Get(_) = _, %v", client, err)
        }

        return resp, nil
}

func GetPodResourceRequests(podName string, podNamespace string) (string, error) {
        podResources, err := GetNodeDevices()
        if err != nil{
                fmt.Errorf("Error in getting the node devices %v %v", podResources, err)
        }

        var kpr KubeletPodResources
        kpr.resources = podResources.GetPodResources()
	podDeviceIDs := ""
        for _, pr := range kpr.resources {
                if pr.Name == podName && pr.Namespace == podNamespace {
                        for _, ctr := range pr.Containers {
                                for _, dev := range ctr.Devices {
                                        podDeviceIDs += fmt.Sprint(dev.DeviceIds)
                                }
				if len(podDeviceIDs) > 2 {
					// example: [0000:81:02.0 0000:81:0a.3]
					// remove "[]" and return DeviceIds
					length := len(podDeviceIDs) - 1
				        podDeviceIDs = podDeviceIDs[1:length]
					log.Info("pod DeviceIds of ", podName, podNamespace, "are: ", podDeviceIDs)
					return podDeviceIDs, nil
				}
                        }
                }
        }

	return "", nil
}

// Inspired by and based on:
// https://github.com/k8snetworkplumbingwg/sriov-cni/blob/master/pkg/config/config.go#L90-L105
// GetVfInfo returns VF's PF and VF ID given its PCI addr
func GetVfInfo(pciAddr string) (string, int, error) {
        var vfID int

        pf, err := utils.GetPfName(pciAddr)
        if err != nil {
                return "", vfID, err
        }

        vfID, err = utils.GetVfid(pciAddr, pf)
        if err != nil {
                return "", vfID, err
        }

        return pf, vfID, nil
}

// GetVFLinkNames returns VF's network interface name given its PCI addr
func GetVFLinkNames(pciAddr string) (string, error) {
        return utils.GetVFLinkNames(pciAddr)
}
