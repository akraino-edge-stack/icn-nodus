/*
 * Copyright 2020 Intel Corporation, Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package nfn

import (
	"context"
	"fmt"
	"ovn4nfv-k8s-plugin/internal/pkg/cniserver"
	"ovn4nfv-k8s-plugin/internal/pkg/config"
	"ovn4nfv-k8s-plugin/internal/pkg/kube"
	"ovn4nfv-k8s-plugin/internal/pkg/network"
	"ovn4nfv-k8s-plugin/internal/pkg/ovn"
	k8sv1alpha1 "ovn4nfv-k8s-plugin/pkg/apis/k8s/v1alpha1"
	pc "ovn4nfv-k8s-plugin/pkg/controller/pod"
	"reflect"
	"strings"

	pb "ovn4nfv-k8s-plugin/internal/pkg/nfnNotify/proto"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/json"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("chaining")

// RoutingInfo is ...
type RoutingInfo struct {
	Name                 string              // Name of the pod
	Namespace            string              // Namespace of the Pod
	Id                   string              // Container ID for pod
	Node                 string              // Hostname where Pod is scheduled
	LeftNetworkRoute     []k8sv1alpha1.Route // TODO: Update to support multiple networks
	RightNetworkRoute    k8sv1alpha1.Route   // TODO: Update to support multiple networks
	DynamicNetworkRoutes []k8sv1alpha1.Route
}

// PodNetworkInfo is ...
type PodNetworkInfo struct {
	Name        string
	Namespace   string
	Id          string
	Node        string
	NetworkInfo string
	Route       k8sv1alpha1.Route
}

//IsEmpty return true or false
func (r RoutingInfo) IsEmpty() bool {
	return reflect.DeepEqual(r, RoutingInfo{})
}

//configurePodSelectorDeployment
func configurePodSelectorDeployment(ln k8sv1alpha1.RoutingNetwork, sfcEntryPodLabel string, toDelete bool, mode string, networklabel string) ([]RoutingInfo, []PodNetworkInfo, error) {
	var rt []RoutingInfo
	var pni []PodNetworkInfo
	var networkname string

	// Get a config to talk to the apiserver
	clientset, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return nil, nil, err
	}

	log.Info("The value of sfcEntryPodLabel", "sfcEntryPodLabel", sfcEntryPodLabel)
	log.Info("The value of ln.NetworkName", "ln.NetworkName", ln.NetworkName)

	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return nil, nil, err
	}

	if mode != k8sv1alpha1.VirutalMode {
		pn, err := k8sv1alpha1Clientset.ProviderNetworks("default").Get(ln.NetworkName, v1.GetOptions{})
		if err != nil {
			log.Error(err, "Error in getting Provider Networks")
			return nil, nil, err
		}

		networkname = pn.GetName()
	}

	if mode == k8sv1alpha1.VirutalMode {
		vn, err := k8sv1alpha1Clientset.Networks("default").List(v1.ListOptions{LabelSelector: networklabel})
		if err != nil {
			log.Error(err, "Error in getting Provider Networks")
			return nil, nil, err
		}

		if len(vn.Items) != 1 {
			err := fmt.Errorf("Virutal network is not available for the networklabel - %s", networklabel)
			log.Error(err, "Error in kube clientset in listing the pods for namespace", "networklabel", networklabel)
			return nil, nil, err
		}

		networkname = vn.Items[0].GetName()
	}

	pods, err := clientset.CoreV1().Pods("default").List(v1.ListOptions{LabelSelector: sfcEntryPodLabel})
	if err != nil {
		//fmt.Printf("List Pods of namespace[%s] error:%v", ns.GetName(), err)
		log.Error(err, "Error in kube clientset in listing the pods for default namespace with label", "sfcEntryPodLabel", sfcEntryPodLabel)
		return nil, nil, err
	}

	if len(pods.Items) != 1 {
		err := fmt.Errorf("Currently load balancing is not supported, expected SFC deployment has only 1 replica")
		log.Error(err, "Error in kube clientset in listing the pods for namespace", "sfcEntryPodLabel", sfcEntryPodLabel)
		return nil, nil, err
	}

	podName := pods.Items[0].GetName()

	log.Info("The value of podName", "podName", podName)
	log.Info("The value of pnName", "networkname", networkname)

	sfcEntryIP, err := ovn.GetIPAdressForPod(networkname, podName)
	if err != nil {
		return nil, nil, err
	}

	//Add Default Route based on Right Network
	defaultRoute := k8sv1alpha1.Route{
		GW:  sfcEntryIP,
		Dst: "0.0.0.0",
	}

	log.Info("The value of sfcEntryIP", "sfcEntryIP", sfcEntryIP)
	log.Info("The value of namespaceSelector", "namespaceSelector.MatchLabels", ln.NamespaceSelector.MatchLabels)
	nsLabel := labels.Set(ln.NamespaceSelector.MatchLabels)
	log.Info("The value of nslabel", "nsLabel.AsSelector().String()", nsLabel.AsSelector().String())

	nslist, err := clientset.CoreV1().Namespaces().List(v1.ListOptions{LabelSelector: nsLabel.AsSelector().String()})
	if err != nil {
		log.Error(err, "Error in kube clientset in listing the namespaces")
		return nil, nil, err
	}

	//fmt.Printf("There are %d namespaces in the cluster\n", len(nslist.Items))
	log.Info("The value of nslabel", "len(nslist.Items)", len(nslist.Items))

	for _, ns := range nslist.Items {
		if ns.GetLabels() == nil {
			//fmt.Printf("The name of the namesp is %s and label is empty\n", ns.GetName())
			log.Info("The namespace label is empty", "namespace", ns.GetName())
			continue
		}
		//fmt.Printf("The name of the namespace is %s and label is %s\n", ns.GetName(), ns.GetLabels())
		log.Info("The value of ", "namespace", ns.GetName(), "labels", ns.GetLabels())
		set := labels.Set(ns.GetLabels())
		//fmt.Printf("The namespace %s as Selector is %+v\n", ns.GetName(), set.AsSelector())
		log.Info("The value of ", "namespace", ns.GetName(), "selector", set.AsSelector())
		pods, err := clientset.CoreV1().Pods(ns.GetName()).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			//fmt.Printf("List Pods of namespace[%s] error:%v", ns.GetName(), err)
			log.Error(err, "Error in kube clientset in listing the pods for namespace", "namespace", ns.GetName())
			return nil, nil, err
		}

		for _, pod := range pods.Items {
			//fmt.Println(v.GetName(), v.Spec.NodeName)
			var IsNetworkattached bool
			var netinfo string
			log.Info("The value of ", "Pod", pod.GetName(), "Node", pod.Spec.NodeName)
			IsNetworkattached, err := pc.IsPodNetwork(pod, networkname)
			if !IsNetworkattached {
				if err != nil {
					log.Error(err, "Error getting pod network", "network", networkname)
					return nil, nil, err
				}
				log.Info("The pod is not having the network", "pod", pod.GetName(), "network", networkname)
				netinfo, err = pc.AddPodNetworkAnnotations(pod, networkname, toDelete)
				if err != nil {
					log.Error(err, "Error in adding the network pod annotations")
					return nil, nil, err
				}
			}
			if IsNetworkattached {
				// Get the containerID of the first container
				var r RoutingInfo
				r.Id = strings.TrimPrefix(pod.Status.ContainerStatuses[0].ContainerID, "docker://")
				r.Namespace = pod.GetNamespace()
				r.Name = pod.GetName()
				r.Node = pod.Spec.NodeName
				r.DynamicNetworkRoutes = append(r.DynamicNetworkRoutes, defaultRoute)
				log.Info("length of r.LeftNetworkRoute", "r.LeftNetworkRoute", len(r.LeftNetworkRoute))
				log.Info("length of r.LeftNetworkRoute", "r.LeftNetworkRoute", r.RightNetworkRoute.IsEmpty())
				log.Info("length of r.DynamicNetworkRoutes", "r.DynamicNetworkRoutes", len(r.DynamicNetworkRoutes))
				rt = append(rt, r)
			} else {
				var p PodNetworkInfo
				p.Id = strings.TrimPrefix(pod.Status.ContainerStatuses[0].ContainerID, "docker://")
				p.Namespace = pod.GetNamespace()
				p.Name = pod.GetName()
				p.Node = pod.Spec.NodeName
				p.NetworkInfo = netinfo
				p.Route = defaultRoute
				pni = append(pni, p)
			}
		}
	}

	log.Info("Value of rt", "rt", rt)
	return rt, pni, nil
}

// Calcuate route to get to left and right edge networks and other networks (not adjacent) in the chain
func calculateDeploymentRoutes(namespace, label string, pos int, num int, ln []k8sv1alpha1.RoutingNetwork, rn []k8sv1alpha1.RoutingNetwork, networkList, deploymentList []string) (r RoutingInfo, err error) {

	var nextLeftIP string
	var nextRightIP string

	r.Namespace = namespace
	// Get a config to talk to the apiserver
	k, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return RoutingInfo{}, err
	}

	lo := v1.ListOptions{LabelSelector: label}
	// List the Pods matching the Labels
	pods, err := k.CoreV1().Pods(namespace).List(lo)
	if err != nil {
		log.Error(err, "Deloyment with label not found", "label", label)
		return RoutingInfo{}, err
	}
	// LOADBALANCER NOT YET SUPPORTED - Assuming deployment has only one Pod
	if len(pods.Items) <= 0 {
		log.Error(err, "Deloyment with label not found", "label", label)
		return RoutingInfo{}, fmt.Errorf("Pod not found")
	}
	// Get the containerID of the first container
	r.Id = strings.TrimPrefix(pods.Items[0].Status.ContainerStatuses[0].ContainerID, "docker://")
	r.Name = pods.Items[0].GetName()
	r.Node = pods.Items[0].Spec.NodeName

	// Calcluate IP addresses for next neighbours on left
	for _, l := range ln {
		var routeinfo k8sv1alpha1.Route
		if pos == 0 {
			nextLeftIP = l.GatewayIP
		} else {
			name := strings.Split(deploymentList[pos-1], "=")
			nextLeftIP, err = ovn.GetIPAdressForPod(networkList[pos-1], name[1])
			if err != nil {
				return RoutingInfo{}, err
			}
		}
		routeinfo.GW = nextLeftIP
		routeinfo.Dst = l.Subnet
		r.LeftNetworkRoute = append(r.LeftNetworkRoute, routeinfo)
	}

	log.Info("Information of pods leftNetworkRoute", "pod", pods.Items[0].GetName(), "r.LeftNetworkRoute", r.LeftNetworkRoute)
	// Calcluate IP addresses for next neighbours on right sides
	if pos == num-1 {
		nextRightIP = rn[0].GatewayIP
	} else {
		name := strings.Split(deploymentList[pos+1], "=")
		nextRightIP, err = ovn.GetIPAdressForPod(networkList[pos], name[1])
		if err != nil {
			return RoutingInfo{}, err
		}
	}
	// Calcuate left right Route to be inserted in Pod
	//r.LeftNetworkRoute.Dst = ln[0].Subnet
	//r.LeftNetworkRoute.GW = nextLeftIP
	r.RightNetworkRoute.Dst = rn[0].Subnet
	r.RightNetworkRoute.GW = nextRightIP
	// For each network that is not adjacent add route
	for i := 0; i < len(networkList); i++ {
		if i == pos || i == pos-1 {
			continue
		} else {
			var rt k8sv1alpha1.Route
			rt.Dst, err = ovn.GetNetworkSubnet(networkList[i])
			if err != nil {
				return RoutingInfo{}, err
			}
			if i > pos {
				rt.GW = nextRightIP
			} else {
				rt.GW = nextLeftIP
			}
			r.DynamicNetworkRoutes = append(r.DynamicNetworkRoutes, rt)
		}
	}

	log.Info("Information of pods DynamicNetworkRoutes", "pod", pods.Items[0].GetName(), "r.DynamicNetworkRoutes", r.DynamicNetworkRoutes)
	//Add Default Route based on Right Network
	rt := k8sv1alpha1.Route{
		GW:  nextRightIP,
		Dst: "0.0.0.0",
	}
	r.DynamicNetworkRoutes = append(r.DynamicNetworkRoutes, rt)
	log.Info("Information of pods DynamicNetworkRoutes with dst-0.0.0.0", "pod", pods.Items[0].GetName(), "r.DynamicNetworkRoutes", r.DynamicNetworkRoutes)
	return
}

//ValidateNetworkChaining return ...
func ValidateNetworkChaining(cr *k8sv1alpha1.NetworkChaining) (string, error) {
	var mode string

	left := cr.Spec.RoutingSpec.LeftNetwork
	right := cr.Spec.RoutingSpec.RightNetwork

	if (len(left) == 0) || (len(right) == 0) {
		return "", fmt.Errorf("Error - size of left is %d and size of right %d", len(left), len(right))
	}

	chains := strings.Split(cr.Spec.RoutingSpec.NetworkChain, ",")
	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return "", err
	}

	sfcheadnet, err := k8sv1alpha1Clientset.Networks("default").List(v1.ListOptions{LabelSelector: chains[0]})
	if err != nil {
		log.Error(err, "Error in kube clientset in listing the namespaces")
		return "", err
	}

	sfctailnet, err := k8sv1alpha1Clientset.Networks("default").List(v1.ListOptions{LabelSelector: chains[len(chains)-1]})
	if err != nil {
		log.Error(err, "Error in kube clientset in listing the namespaces")
		return "", err
	}

	if (len(sfcheadnet.Items) != 0) && (len(sfctailnet.Items) != 0) {
		mode = k8sv1alpha1.VirutalMode
	}

	return mode, nil
}

//ConfigureNetworkFromLabel return ...
func configureNetworkFromLabel(label string) (r k8sv1alpha1.RoutingNetwork, err error) {
	var route k8sv1alpha1.RoutingNetwork

	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return k8sv1alpha1.RoutingNetwork{}, err
	}

	net, err := k8sv1alpha1Clientset.Networks("default").List(v1.ListOptions{LabelSelector: label})
	if err != nil {
		log.Error(err, "Error in kube clientset in listing the namespaces")
		return k8sv1alpha1.RoutingNetwork{}, err
	}

	if len(net.Items) != 1 {
		err := fmt.Errorf("Virutal network is not available for the networklabel - %s", label)
		log.Error(err, "Error in kube clientset in listing the pods for namespace", "networklabel")
		return k8sv1alpha1.RoutingNetwork{}, err
	}

	route.NetworkName = net.Items[0].GetName()
	ipv4Subnets := net.Items[0].Spec.Ipv4Subnets
	route.GatewayIP = ipv4Subnets[0].ExcludeIps
	route.Subnet = ipv4Subnets[0].Subnet

	return route, nil
}

// CalculateRoutes returns the routing info
func CalculateRoutes(cr *k8sv1alpha1.NetworkChaining, cs bool) ([]PodNetworkInfo, []RoutingInfo, error) {
	//
	var deploymentList []string
	var networkList []string
	var sfctaillabel, sfcheadlabel string

	// TODO: Add Validation of Input to this function
	ln := cr.Spec.RoutingSpec.LeftNetwork
	rn := cr.Spec.RoutingSpec.RightNetwork
	chains := strings.Split(cr.Spec.RoutingSpec.NetworkChain, ",")

	mode, err := ValidateNetworkChaining(cr)
	if err != nil {
		return nil, nil, err
	}

	if mode == k8sv1alpha1.VirutalMode {
		sfcheadlabel = chains[0]
		sfctaillabel = chains[len(chains)-1]
		_ = sfctaillabel
		chains = chains[1 : len(chains)-1]
	}

	i := 0
	for _, chain := range chains {
		if i%2 == 0 {
			deploymentList = append(deploymentList, chain)
		} else {
			networkList = append(networkList, chain)
		}
		i++
	}
	num := len(deploymentList)
	log.Info("Display the num", "num", num)
	log.Info("Display the ln", "ln", ln)
	log.Info("Display the rn", "rn", rn)
	log.Info("Display the networklist", "networkList", networkList)
	log.Info("Display the deploymentlist", "deploymentList", deploymentList)

	var chainRoutingInfo []RoutingInfo
	var lnRoutingInfo []RoutingInfo
	var podsNetworkInfo []PodNetworkInfo
	//var rnRoutingInfo []RoutingInfo

	for _, leftNetworks := range cr.Spec.RoutingSpec.LeftNetwork {
		log.Info("Display the ln", "GatewayIP", leftNetworks.GatewayIP)
		log.Info("Display the ln", "NetworkName", leftNetworks.NetworkName)
		log.Info("Display the ln", "Subnet", leftNetworks.Subnet)
		log.Info("Display the ln", "PodSelector.MatchLabels", leftNetworks.PodSelector.MatchLabels)
		log.Info("Display the ln", "NamespaceSelector.MatchLabels", leftNetworks.NamespaceSelector.MatchLabels)
		var r []RoutingInfo
		var pni []PodNetworkInfo

		r, pni, err := configurePodSelectorDeployment(leftNetworks, deploymentList[0], cs, mode, sfcheadlabel)
		if err != nil {
			return nil, nil, err
		}

		lnRoutingInfo = append(lnRoutingInfo, r...)
		podsNetworkInfo = append(podsNetworkInfo, pni...)
		log.Info("Value of lnRoutingInfo ", "lnRoutingInfo ", lnRoutingInfo)
		log.Info("Value of podsNetworkInfo ", "podsNetworkInfo ", podsNetworkInfo)
	}

	chainRoutingInfo = append(chainRoutingInfo, lnRoutingInfo...)

	for _, rightNetworks := range cr.Spec.RoutingSpec.RightNetwork {
		log.Info("Display the rn", "GatewayIP", rightNetworks.GatewayIP)
		log.Info("Display the rn", "NetworkName", rightNetworks.NetworkName)
		log.Info("Display the rn", "Subnet", rightNetworks.Subnet)
		log.Info("Display the rn", "PodSelector.MatchLabels", rightNetworks.PodSelector.MatchLabels)
		log.Info("Display the rn", "NamespaceSelector.MatchLabels", rightNetworks.NamespaceSelector.MatchLabels)
	}

	if mode == k8sv1alpha1.VirutalMode {
		r, err := configureNetworkFromLabel(sfcheadlabel)
		if err != nil {
			return nil, nil, err
		}
		ln = append(ln, r)
	}

	for i, deployment := range deploymentList {
		r, err := calculateDeploymentRoutes(cr.Namespace, deployment, i, num, ln, rn, networkList, deploymentList)
		if err != nil {
			return nil, nil, err
		}
		chainRoutingInfo = append(chainRoutingInfo, r)
	}

	log.Info("Value of podsNetworkInfo ", "podsNetworkInfo ", podsNetworkInfo)
	return podsNetworkInfo, chainRoutingInfo, nil
}

//ContainerAddInteface return
func ContainerAddInteface(containerPid int, payload *pb.PodAddNetwork) error {
	log.Info("Container pid", "containerPid", containerPid)
	log.Info("payload network", "payload.GetNet()", payload.GetNet())
	log.Info("payload pod", "payload.GetPod()", payload.GetPod())
	log.Info("payload route", "payload.GetRoute()", payload.GetRoute())

	podinfo := payload.GetPod()
	//podroute := payload.GetRoute()
	podnetconf := payload.GetNet()

	var netconfs []map[string]string
	err := json.Unmarshal([]byte(podnetconf.Data), &netconfs)
	if err != nil {
		return fmt.Errorf("Error in unmarshal podnet conf=%v", err)
	}

	cnishimreq := &cniserver.CNIServerRequest{
		Command:      cniserver.CNIAdd,
		PodNamespace: podinfo.Namespace,
		PodName:      podinfo.Name,
		SandboxID:    config.GeneratePodNameID(podinfo.Name),
		Netns:        fmt.Sprintf("/host/proc/%d/ns/net", containerPid),
		IfName:       netconfs[0]["interface"],
		CNIConf:      nil,
	}

	result := cnishimreq.AddMultipleInterfaces(podnetconf.Data, podinfo.Namespace, podinfo.Name)
	if result == nil {
		return fmt.Errorf("result is nil from cni server for adding interface in the existing pod")
	}

	return nil
}

//ContainerDelInteface return
func ContainerDelInteface(containerPid int, payload *pb.PodDelNetwork) error {
	log.Info("Container pid", "containerPid", containerPid)
	log.Info("payload network", "payload.GetNet()", payload.GetNet())
	log.Info("payload pod", "payload.GetPod()", payload.GetPod())
	log.Info("payload route", "payload.GetRoute()", payload.GetRoute())

	podinfo := payload.GetPod()
	podnetconf := payload.GetNet()

	var netconfs []map[string]string
	err := json.Unmarshal([]byte(podnetconf.Data), &netconfs)
	if err != nil {
		return fmt.Errorf("Error in unmarshal podnet conf=%v", err)
	}

	cnishimreq := &cniserver.CNIServerRequest{
		Command:      cniserver.CNIDel,
		PodNamespace: podinfo.Namespace,
		PodName:      podinfo.Name,
		SandboxID:    config.GeneratePodNameID(podinfo.Name),
		Netns:        fmt.Sprintf("/host/proc/%d/ns/net", containerPid),
		IfName:       netconfs[0]["interface"],
		CNIConf:      nil,
	}

	err = cnishimreq.DeleteMultipleInterfaces(podnetconf.Data, podinfo.Namespace, podinfo.Name)
	if err != nil {
		return fmt.Errorf("cni server for deleting interface in the existing pod=%v", err)
	}

	return nil
}

// ContainerDelRoute return containerPid and route
func ContainerDelRoute(containerPid int, route []*pb.RouteData) error {
	str := fmt.Sprintf("/host/proc/%d/ns/net", containerPid)

	hostNet, err := network.GetHostNetwork()
	if err != nil {
		log.Error(err, "Failed to get host network")
		return err
	}

	k, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return err
	}

	kubecli := &kube.Kube{KClient: k}
	_, err = kubecli.GetControlPlaneServiceIPRange()
	if err != nil {
		log.Error(err, "Error in getting svc cidr range")
		return err
	}

	kn, err := kubecli.GetAnotherControlPlaneServiceIPRange()
	if err != nil {
		log.Error(err, "Error in getting svc cidr range")
		return err
	}

	nms, err := ns.GetNS(str)
	if err != nil {
		log.Error(err, "Failed namesapce", "containerID", containerPid)
		return err
	}
	defer nms.Close()
	err = nms.Do(func(_ ns.NetNS) error {
		podGW, err := network.GetGatewayInterface(kn.ServiceSubnet)
		if err != nil {
			log.Error(err, "Failed to get service subnet route gateway")
			return err
		}

		stdout, stderr, err := ovn.RunIP("route", "del", hostNet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route del", "stdout", stdout, "stderr", stderr)
			return err
		}

		stdout, stderr, err = ovn.RunIP("route", "del", kn.ServiceSubnet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route del", "stdout", stdout, "stderr", stderr)
			return err
		}

		stdout, stderr, err = ovn.RunIP("route", "del", kn.PodSubnet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route del", "stdout", stdout, "stderr", stderr)
			return err
		}

		for _, r := range route {
			dst := r.GetDst()
			gw := r.GetGw()
			// Replace default route
			if dst == "0.0.0.0" {
				stdout, stderr, err := ovn.RunIP("route", "replace", "default", "via", podGW)
				if err != nil {
					log.Error(err, "Failed to ip route replace", "stdout", stdout, "stderr", stderr)
					return err
				}
			} else {
				isExist, err := network.IsRouteExist(dst, gw)
				if err != nil {
					log.Error(err, "Failed to get dst route gateway")
					return err
				}
				if isExist == true {
					stdout, stderr, err := ovn.RunIP("route", "del", dst, "via", gw)
					if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
						log.Error(err, "Failed to ip route del", "stdout", stdout, "stderr", stderr)
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Error(err, "Failed Netns Do", "containerID", containerPid)
		return err
	}
	return nil
}

// ContainerAddRoute return containerPid and route
func ContainerAddRoute(containerPid int, route []*pb.RouteData) error {
	str := fmt.Sprintf("/host/proc/%d/ns/net", containerPid)

	hostNet, err := network.GetHostNetwork()
	if err != nil {
		log.Error(err, "Failed to get host network")
		return err
	}

	k, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return err
	}

	kubecli := &kube.Kube{KClient: k}
	_, err = kubecli.GetControlPlaneServiceIPRange()
	if err != nil {
		log.Error(err, "Error in getting svc cidr range")
		return err
	}

	kn, err := kubecli.GetAnotherControlPlaneServiceIPRange()
	if err != nil {
		log.Error(err, "Error in getting svc cidr range")
		return err
	}

	nms, err := ns.GetNS(str)
	if err != nil {
		log.Error(err, "Failed namesapce", "containerID", containerPid)
		return err
	}
	defer nms.Close()
	err = nms.Do(func(_ ns.NetNS) error {
		podGW, err := network.GetDefaultGateway()
		if err != nil {
			log.Error(err, "Failed to get pod default gateway")
			return err
		}

		stdout, stderr, err := ovn.RunIP("route", "add", hostNet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
			return err
		}

		stdout, stderr, err = ovn.RunIP("route", "add", kn.ServiceSubnet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
			return err
		}

		stdout, stderr, err = ovn.RunIP("route", "add", kn.PodSubnet, "via", podGW)
		if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
			log.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
			return err
		}

		for _, r := range route {
			dst := r.GetDst()
			gw := r.GetGw()
			// Replace default route
			if dst == "0.0.0.0" {
				stdout, stderr, err := ovn.RunIP("route", "replace", "default", "via", gw)
				if err != nil {
					log.Error(err, "Failed to ip route replace", "stdout", stdout, "stderr", stderr)
					return err
				}
			} else {
				stdout, stderr, err := ovn.RunIP("route", "add", dst, "via", gw)
				if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
					log.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Error(err, "Failed Netns Do", "containerID", containerPid)
		return err
	}
	return nil
}

func GetPidForContainer(id string) (int, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		fmt.Println("Unable to create docker client")
		return 0, err
	}
	cli.NegotiateAPIVersion(context.Background())
	cj, err := cli.ContainerInspect(context.Background(), id)
	if err != nil {
		fmt.Println("Unable to Inspect docker container")
		return 0, err
	}
	if cj.State.Pid == 0 {
		return 0, fmt.Errorf("Container not found %s", id)
	}
	return cj.State.Pid, nil

}
