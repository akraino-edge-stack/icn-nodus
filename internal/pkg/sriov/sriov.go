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
        //logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

// GetK8sArgs gets k8s related args from CNI args
func GetK8sArgs(args *skel.CmdArgs) (*K8sArgs, error) {
        k8sArgs := &K8sArgs{}

        err := types.LoadArgs(args.Args, k8sArgs)
        if err != nil {
                return nil, err
        }

        return k8sArgs, nil
}

func GetNodeDevices() (*podresourcesv1alpha1.ListPodResourcesResponse, error) {

        //endpoint, err := util.LocalEndpoint(defaultPodResourcesPath, podresources.Socket)
        endpoint := util.LocalEndpoint(defaultPodResourcesPath, podresources.Socket)
        //if err != nil {
        //      return nil, fmt.Errorf("Error getting local endpoint: %v", err)
        //}

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
        //fmt.Println("SRIOV_PKG pod resources %+v", kpr.resources)
	podDeviceIDs := ""
        for _, pr := range kpr.resources {
                //fmt.Println("SRIOV_PKG Name %+v, Namespace %+v", pr.Name, pr.Namespace)
                if pr.Name == podName && pr.Namespace == podNamespace {
                        for _, ctr := range pr.Containers {
                                //fmt.Println("SRIOV_PKG Container %+v", ctr)
                                for _, dev := range ctr.Devices {
                                        //fmt.Println("SRIOV Device IDs %+v", dev.DeviceIds)
                                        podDeviceIDs += fmt.Sprint(dev.DeviceIds)
					//fmt.Println("SRIOV POD Device IDs %+v", podDeviceIDs)
                                }
				if len(podDeviceIDs) > 2 {
					// example: [0000:81:02.0 0000:81:0a.3]
					// remove "[]" and return DeviceIds
					length := len(podDeviceIDs) - 1
				        podDeviceIDs = podDeviceIDs[1:length]
					return podDeviceIDs, nil
				}
                        }
                }
        }

	return "", nil
}
