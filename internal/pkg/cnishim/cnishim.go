package cni

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"ovn4nfv-k8s-plugin/internal/pkg/cniserver"
	"ovn4nfv-k8s-plugin/internal/pkg/config"
	"ovn4nfv-k8s-plugin/internal/pkg/sriov"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/sirupsen/logrus"
)

const CNIEndpointURLReq string = "http://dummy/"

type Endpoint struct {
	cniServerSocketPath string
}

func CNIEndpoint(cniServerSocketPath string) *Endpoint {
	if len(cniServerSocketPath) == 0 {
		cniServerSocketPath = cniserver.CNIServerSocketPath
	}
	return &Endpoint{cniServerSocketPath: cniServerSocketPath}
}

func cniEndpointRequest(args *skel.CmdArgs) *cniserver.CNIEndpointRequest {
	osEnvMap := make(map[string]string)
	for _, item := range os.Environ() {
		idx := strings.Index(item, "=")
		if idx > 0 {
			osEnvMap[strings.TrimSpace(item[:idx])] = item[idx+1:]
		}
	}

	return &cniserver.CNIEndpointRequest{
		ArgEnv:    osEnvMap,
		NetConfig: args.StdinData,
	}
}

func cniGetPodPciDeviceIdRequest(args *skel.CmdArgs) {
        // Parse pod resources list and get the PCI device for K8S_POD_NAME and K8S_POD_NAMESPACE
        os.Setenv("POD_DEVICEID", "")
        k8sArgs, err := sriov.GetK8sArgs(args)
        if k8sArgs != nil && err == nil {
                deviceid, err := sriov.GetPodResourceRequests(string(k8sArgs.K8S_POD_NAME), string(k8sArgs.K8S_POD_NAMESPACE))
                if deviceid != "" && err == nil {
			os.Setenv("POD_DEVICEID", deviceid)
		} else {
                        logrus.Infof("Error getting pci device ids: %+v", err)
                }
        }
}

func (ep *Endpoint) sendCNIServerReq(req *cniserver.CNIEndpointRequest) ([]byte, error) {
	cnireqdata, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("sendCNIServerReq: failed to Marshal CNIShim Req %v:%v", req, err)
	}

	httpc := http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (net.Conn, error) {
				return net.Dial("unix", ep.cniServerSocketPath)
			},
		},
	}

	reponse, err := httpc.Post(CNIEndpointURLReq, "application/json", bytes.NewReader(cnireqdata))
	if err != nil {
		return nil, fmt.Errorf("Failed to send CNIServer request: %v", err)
	}
	defer reponse.Body.Close()

	rbody, err := ioutil.ReadAll(reponse.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read the CNI Server reponse:%v", err)
	}

	if reponse.StatusCode != 200 {
		return nil, fmt.Errorf("CNI Server request is failed with reponse status %v and reponse body %s", reponse.StatusCode, string(rbody))
	}

	return rbody, nil
}

func (ep *Endpoint) CmdAdd(args *skel.CmdArgs) error {
	logrus.Infof("ovn4nfvk8s-cni: cmdAdd ")
	conf, err := config.ConfigureNetConf(args.StdinData)
	if err != nil {
		return fmt.Errorf("invalid stdin args")
	}
	logrus.Infof("ovn4nfvk8s-cni: cmdAdd configure net conf details -%+v", conf)

        // Parse pod resources list and get the PCI device for K8S_POD_NAME and K8S_POD_NAMESPACE
	cniGetPodPciDeviceIdRequest(args)
	logrus.Infof("ovn4nfvk8s-cni: cmdAdd cniGetPodPciDeviceIdRequest:%+v",os.Getenv("POD_DEVICEID"))

	req := cniEndpointRequest(args)
        logrus.Infof("ovn4nfvk8s-cni: cmdAdd CNIEndpoint Request:%+v",req)
	reponsebody, err := ep.sendCNIServerReq(req)
	if err != nil {
		return err
	}
	result, err := current.NewResult(reponsebody)
	if err != nil {
		return fmt.Errorf("failed to unmarshall CNIServer Result reponse %v - err:%v", string(reponsebody), err)
	}

	return types.PrintResult(result, conf.CNIVersion)
}

func (ep *Endpoint) CmdCheck(args *skel.CmdArgs) error {
	return nil
}

func (ep *Endpoint) CmdDel(args *skel.CmdArgs) error {
	logrus.Infof("ovn4nfvk8s-cni: cmdDel ")

        // Parse pod resources list and get the PCI device for K8S_POD_NAME and K8S_POD_NAMESPACE
        cniGetPodPciDeviceIdRequest(args)
        //logrus.Infof("ovn4nfvk8s-cni: cmdDel cniGetPodPciDeviceIdRequest:%+v",os.Getenv("POD_DEVICEID"))

	req := cniEndpointRequest(args)
	_, err := ep.sendCNIServerReq(req)
	return err
}
