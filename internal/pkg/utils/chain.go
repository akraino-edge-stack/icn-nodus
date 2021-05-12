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
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/kubernetes"

	pb "ovn4nfv-k8s-plugin/internal/pkg/nfnNotify/proto"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/client"
	"github.com/mitchellh/mapstructure"
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
	RightNetworkRoute    []k8sv1alpha1.Route // TODO: Update to support multiple networks
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

const (
	// Ovn4nfvAnnotationTag tag on already processed Pods
	SFCannotationTag = "k8s.plugin.opnfv.org/sfc"
	SFCcreated       = "created"
	SFCprocessing    = "processing"
	SFCNotrequired   = "notrequired"
	SFCHead          = "sfchead"
	SFCTail          = "sfctail"
)

//IsEmpty return true or false
func (r RoutingInfo) IsEmpty() bool {
	return reflect.DeepEqual(r, RoutingInfo{})
}

//configurePodSelectorDeployment
func configurePodSelectorDeployment(ln k8sv1alpha1.RoutingNetwork, sfcEntryPodLabel string, toDelete bool, mode string, networklabel string, sfcposition string, dst []string) ([]RoutingInfo, []PodNetworkInfo, error) {
	var rt []RoutingInfo
	var pni []PodNetworkInfo
	var networkname string
	var defaultRoute k8sv1alpha1.Route

	// Get a config to talk to the apiserver
	clientset, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return nil, nil, err
	}

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

	sfcEntryIP, err := ovn.GetIPAdressForPod(networkname, podName)
	if err != nil {
		return nil, nil, err
	}

	if sfcposition == SFCHead {
		subnet := dst[0]
		//Add Default Route based on Right Network
		defaultRoute = k8sv1alpha1.Route{
			GW:  sfcEntryIP,
			Dst: subnet,
		}
	}

	if sfcposition == SFCTail {
		for _, d := range dst {
			log.Info("list the sfc tail route", "dst", d, "gw", sfcEntryIP)
		}
	}

	nsLabel := labels.Set(ln.NamespaceSelector.MatchLabels)
	nslist, err := clientset.CoreV1().Namespaces().List(v1.ListOptions{LabelSelector: nsLabel.AsSelector().String()})
	if err != nil {
		log.Error(err, "Error in kube clientset in listing the namespaces")
		return nil, nil, err
	}

	log.Info("Value of the nslabel", "nslabel", nsLabel)
	log.Info("Value of the nslabel", "nslist", nslist)
	for _, ns := range nslist.Items {
		if ns.GetLabels() == nil {
			log.Info("The namespace label is empty", "namespace", ns.GetName())
			continue
		}

		log.Info("Value of the ns.GetLabels", "ns.GetLabels()", ns.GetLabels())
		set := labels.Set(ns.GetLabels())
		log.Info("Value of the nslabel", "set", set)
		pods, err := clientset.CoreV1().Pods(ns.GetName()).List(v1.ListOptions{LabelSelector: set.AsSelector().String()})
		if err != nil {
			log.Error(err, "Error in kube clientset in listing the pods for namespace", "namespace", ns.GetName())
			return nil, nil, err
		}

		for _, pod := range pods.Items {
			var IsNetworkattached bool
			var netinfo string

			log.Info("Value of the pod", "pod", pod.GetName())

			if toDelete != true {
				annotation := pod.GetAnnotations()
				_, ok := annotation[SFCannotationTag]
				if ok {
					continue
				}
			}
			IsNetworkattached, err := IsPodNetwork(pod, networkname)
			if !IsNetworkattached {
				if err != nil {
					log.Error(err, "Error getting pod network", "network", networkname)
					return nil, nil, err
				}
				netinfo, err = AddPodNetworkAnnotations(pod, networkname, toDelete)
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
				if sfcposition == SFCHead {
					r.DynamicNetworkRoutes = append(r.DynamicNetworkRoutes, defaultRoute)
				}
				rt = append(rt, r)
			} else {
				var p PodNetworkInfo
				p.Id = strings.TrimPrefix(pod.Status.ContainerStatuses[0].ContainerID, "docker://")
				p.Namespace = pod.GetNamespace()
				p.Name = pod.GetName()
				p.Node = pod.Spec.NodeName
				p.NetworkInfo = netinfo
				if sfcposition == SFCHead {
					p.Route = defaultRoute
				}
				pni = append(pni, p)
			}
			if toDelete != true {
				kubecli := &kube.Kube{KClient: clientset}
				key := SFCannotationTag
				value := SFCcreated
				err = kubecli.SetAnnotationOnPod(&pod, key, value)
				if err != nil {
					log.Error(err, "Error in Setting the SFC annotation")
				}
			}
		}
	}

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

	// Calcluate IP addresses for next neighbours on right sides
	for _, right := range rn {
		var routeinfo k8sv1alpha1.Route
		if pos == num-1 {
			nextRightIP = right.GatewayIP
		} else {
			name := strings.Split(deploymentList[pos+1], "=")
			nextRightIP, err = ovn.GetIPAdressForPod(networkList[pos], name[1])
			if err != nil {
				return RoutingInfo{}, err
			}
		}
		routeinfo.Dst = right.Subnet
		routeinfo.GW = nextRightIP
		r.RightNetworkRoute = append(r.RightNetworkRoute, routeinfo)
	}

	// Calcluate IP addresses for next neighbours on right sides
	//if pos == num-1 {
	//	nextRightIP = rn[0].GatewayIP
	//} else {
	//	name := strings.Split(deploymentList[pos+1], "=")
	//	nextRightIP, err = ovn.GetIPAdressForPod(networkList[pos], name[1])
	//	if err != nil {
	//		return RoutingInfo{}, err
	//	}
	//}
	// Calcuate left right Route to be inserted in Pod
	//r.RightNetworkRoute.Dst = rn[0].Subnet
	//r.RightNetworkRoute.GW = nextRightIP

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

	//Add Default Route based on Right Network
	rt := k8sv1alpha1.Route{
		GW:  nextRightIP,
		Dst: "0.0.0.0",
	}
	r.DynamicNetworkRoutes = append(r.DynamicNetworkRoutes, rt)
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

func noSFCrequired(clientset *kubernetes.Clientset, podname string, podnamespace string) error {
	pod, err := clientset.CoreV1().Pods(podnamespace).Get(podname, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("noSFCrequired - Error in getting the pod - %s clientset get options - %v", podname, err)
	}

	kubecli := &kube.Kube{KClient: clientset}
	key := SFCannotationTag
	value := SFCNotrequired
	err = kubecli.SetAnnotationOnPod(pod, key, value)
	if err != nil {
		log.Error(err, "Error in Setting the SFC annotation")
	}

	log.Info("Pod SFC configuration is not required", "podname", podname)
	return nil
}

func compareEachLabel(a map[string]string, b map[string]string) bool {
	var isEqual bool

	for akey, aValue := range a {
		for bkey, bValue := range b {
			if akey == bkey && aValue == bValue {
				isEqual = true
				break
			}
			if isEqual == true {
				break
			}
		}
	}
	return isEqual
}

//ConfigureforSFC returns
func ConfigureforSFC(podname string, podnamespace string) (bool, []PodNetworkInfo, []RoutingInfo, error) {
	var sfcname string
	var nl, pl map[string]string

	// Get a config to talk to the apiserver
	clientset, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in kube clientset")
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in kube clientset - %v", err)
	}

	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in k8sv1alpha clientset - %v", err)
	}

	sfc, err := k8sv1alpha1Clientset.NetworkChainings("default").List(v1.ListOptions{})
	if err != nil {
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in listing the k8sv1alpha network chainings - %v", err)
	}

	pod, err := clientset.CoreV1().Pods(podnamespace).Get(podname, v1.GetOptions{})
	if err != nil {
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in getting the pod - %s clientset get options - %v", podname, err)
	}

	if pod.GetLabels() == nil {
		err = noSFCrequired(clientset, podname, podnamespace)
		if err != nil {
			log.Error(err, "error in seting SFC not required")
		}
		return false, nil, nil, nil
	}

	pdlabel := pod.GetLabels()
	namespace, err := clientset.CoreV1().Namespaces().Get(podnamespace, v1.GetOptions{})
	if err != nil {
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in getting the pod namespace - %s clientset get options - %v", podnamespace, err)
	}

	if namespace.GetLabels() == nil {
		err = noSFCrequired(clientset, podname, podnamespace)
		if err != nil {
			log.Error(err, "ConfigureforSFC - error in seting SFC not required")
		}
		return false, nil, nil, nil
	}

	if len(sfc.Items) == 0 {
		log.Info("ConfigureforSFC - No SFC created", "podname", podname)
		return false, nil, nil, nil
	}

	pdnslabel := namespace.GetLabels()
	var isSFCExist bool
	for _, nc := range sfc.Items {
		sfcname = nc.GetName()
		left := nc.Spec.RoutingSpec.LeftNetwork
		for _, l := range left {
			pl = l.PodSelector.MatchLabels
			nl = l.NamespaceSelector.MatchLabels
			if compareEachLabel(pl, pdlabel) && compareEachLabel(nl, pdnslabel) {
				isSFCExist = true
				break
			}
		}
		if isSFCExist {
			break
		}
	}

	if isSFCExist == false {
		return false, nil, nil, nil
	}

	cr, err := k8sv1alpha1Clientset.NetworkChainings("default").Get(sfcname, v1.GetOptions{})
	if err != nil {
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in getting the network chaining - %s k8sv1alpha1 clientset get options - %v", sfcname, err)
	}

	podnetworkList, routeList, err := CalculateRoutes(cr, false, true)
	if err != nil {
		return false, nil, nil, fmt.Errorf("ConfigureforSFC - Error in calculate routes for configuring pod for SFC - %v", err)
	}

	log.Info("Pod SFC configuration is successful", "podname", podname)
	return true, podnetworkList, routeList, nil
}

func calculateDstforTail(cr *k8sv1alpha1.NetworkChaining) ([]string, error) {

	return nil, nil
}

// CalculateRoutes returns the routing info
func CalculateRoutes(cr *k8sv1alpha1.NetworkChaining, cs bool, onlyPodSelector bool) ([]PodNetworkInfo, []RoutingInfo, error) {
	var deploymentList []string
	var networkList []string
	var sfctaillabel, sfcheadlabel string

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

	var chainRoutingInfo []RoutingInfo
	var lnRoutingInfo []RoutingInfo
	var rnRoutingInfo []RoutingInfo
	var podsNetworkInfo []PodNetworkInfo
	var dst []string
	//var rnRoutingInfo []RoutingInfo

	for _, leftNetworks := range cr.Spec.RoutingSpec.LeftNetwork {
		var r []RoutingInfo
		var pni []PodNetworkInfo

		//For the sfc head dst will be default
		dst = append(dst, "0.0.0.0")

		r, pni, err := configurePodSelectorDeployment(leftNetworks, deploymentList[0], cs, mode, sfcheadlabel, SFCHead, dst)
		if err != nil {
			return nil, nil, err
		}

		lnRoutingInfo = append(lnRoutingInfo, r...)
		podsNetworkInfo = append(podsNetworkInfo, pni...)
	}

	chainRoutingInfo = append(chainRoutingInfo, lnRoutingInfo...)

	if mode == k8sv1alpha1.VirutalMode {
		l, err := configureNetworkFromLabel(sfcheadlabel)
		if err != nil {
			return nil, nil, err
		}
		ln = append(ln, l)

		r, err := configureNetworkFromLabel(sfctaillabel)
		if err != nil {
			return nil, nil, err
		}
		rn = append(rn, r)
	}

	for _, rightNetworks := range cr.Spec.RoutingSpec.RightNetwork {
		log.Info("Display the rn", "GatewayIP", rightNetworks.GatewayIP)
		log.Info("Display the rn", "NetworkName", rightNetworks.NetworkName)
		log.Info("Display the rn", "Subnet", rightNetworks.Subnet)
		log.Info("Display the rn", "PodSelector.MatchLabels", rightNetworks.PodSelector.MatchLabels)
		log.Info("Display the rn", "NamespaceSelector.MatchLabels", rightNetworks.NamespaceSelector.MatchLabels)
		var r []RoutingInfo
		var pni []PodNetworkInfo

		//For the sfc tail dst will be all right network subnet
		for _, net := range rn {
			dst = append(dst, net.Subnet)
		}

		r, pni, err := configurePodSelectorDeployment(rightNetworks, deploymentList[num-1], cs, mode, sfctaillabel, SFCTail, dst)
		if err != nil {
			return nil, nil, err
		}

		rnRoutingInfo = append(rnRoutingInfo, r...)
		podsNetworkInfo = append(podsNetworkInfo, pni...)
	}

	chainRoutingInfo = append(chainRoutingInfo, lnRoutingInfo...)

	if onlyPodSelector != true {
		for i, deployment := range deploymentList {
			r, err := calculateDeploymentRoutes(cr.Namespace, deployment, i, num, ln, rn, networkList, deploymentList)
			if err != nil {
				return nil, nil, err
			}
			chainRoutingInfo = append(chainRoutingInfo, r)
		}
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

const (
	nfnNetAnnotation = "k8s.plugin.opnfv.org/nfn-network"
)

type nfnNet struct {
	Type      string                   "json:\"type\""
	Interface []map[string]interface{} "json:\"interface\""
}

//IsPodNetwork return ...
func IsPodNetwork(pod corev1.Pod, networkname string) (bool, error) {
	log.Info("checking the pod network %s on pod %s", networkname, pod.GetName())
	annotations := pod.GetAnnotations()
	annotationsValue, result := annotations[nfnNetAnnotation]
	if !result {
		return false, nil
	}

	var nfn nfnNet
	err := json.Unmarshal([]byte(annotationsValue), &nfn)
	if err != nil {
		log.Error(err, "Invalid nfn annotaion", "annotation", annotationsValue)
		return false, err
	}

	if nfn.Type != "ovn4nfv" {
		// to action required
		return false, nil
	}

	var net ovn.NetInterface
	for _, v := range nfn.Interface {
		err := mapstructure.Decode(v, &net)
		if err != nil {
			log.Error(err, "mapstruct error", "network", v)
			return false, err
		}

		if net.Name == networkname {
			return true, nil
		}
	}

	return false, nil
}

func buildNfnAnnotations(pod corev1.Pod, ifname, networkname string, toDelete bool) (string, error) {
	var IsExtraInterfaces bool

	annotations := pod.GetAnnotations()
	_, result := annotations[ovn.Ovn4nfvAnnotationTag]
	if result {
		IsExtraInterfaces = true
	} else {
		// no ovnInterfaces annotations, create a new one
		return "", nil
	}

	nfnInterface := ovn.NetInterface{
		Name:      networkname,
		Interface: ifname,
	}

	//code from here
	var nfnInterfacemap map[string]interface{}
	var nfnInterfaces []map[string]interface{}

	rawByte, err := json.Marshal(nfnInterface)
	if err != nil {
		//handle error handle properly
		return "", err
	}

	err = json.Unmarshal(rawByte, &nfnInterfacemap)
	if err != nil {
		return "", err
	}

	nfnInterfaces = append(nfnInterfaces, nfnInterfacemap)
	nfn := &nfnNet{
		Type:      "ovn4nfv",
		Interface: nfnInterfaces,
	}

	//already ovnInterface annotations is there
	ovnCtl, err := ovn.GetOvnController()
	if err != nil {
		return "", err
	}

	key, value := ovnCtl.AddLogicalPorts(&pod, nfn.Interface, IsExtraInterfaces)
	if len(value) == 0 {
		log.Info("Extra Annotations value is nil: key - %v | value - %v", key, value)
		return "", fmt.Errorf("requested annotation value from the AddLogicalPorts() can't be empty")
	}

	if len(value) > 0 {
		log.Info("Extra Annotations values key - %v | value - %v", key, value)
	}

	if !toDelete {
		k, err := kube.GetKubeConfig()
		if err != nil {
			log.Error(err, "Error in kube clientset")
			return "", fmt.Errorf("Error in getting kube clientset - %v", err)
		}

		kubecli := &kube.Kube{KClient: k}
		err = kubecli.AppendAnnotationOnPod(&pod, key, value)
		if err != nil {
			return "", fmt.Errorf("error in the appending annotation in pod -%v", err)
		}
	}
	//netinformation already appended into the pod annotation
	appendednetinfo := strings.ReplaceAll(value, "\\", "")

	return appendednetinfo, nil
}

//AddPodNetworkAnnotations returns ...
func AddPodNetworkAnnotations(pod corev1.Pod, networkname string, toDelete bool) (string, error) {
	log.Info("checking the pod network %s on pod %s", networkname, pod.GetName())
	annotations := pod.GetAnnotations()
	sfcIfname := ovn.GetSFCNetworkIfname()
	inet := sfcIfname()
	annotationsValue, result := annotations[nfnNetAnnotation]
	if !result {
		// no nfn-network annotations, create a new one
		networkInfo, err := buildNfnAnnotations(pod, inet, networkname, toDelete)
		if err != nil {
			return "", err
		}
		return networkInfo, nil
	}

	// nfn-network annotations exist, but have to find the interface names to
	// avoid the conflict with the inteface name
	var nfn nfnNet
	err := json.Unmarshal([]byte(annotationsValue), &nfn)
	if err != nil {
		log.Error(err, "Invalid nfn annotaion", "annotation", annotationsValue)
		return "", err
	}

	//Todo for external controller
	//if nfn.Type != "ovn4nfv" {
	// no nfn-network annotations for the type ovn4nfv, create a new one
	//	return "", nil
	//}

	// nfn-network annotations exist and type is ovn4nfv
	// check the additional network interfaces names.
	var net ovn.NetInterface

	for _, v := range nfn.Interface {
		err := mapstructure.Decode(v, &net)
		if err != nil {
			log.Error(err, "mapstruct error", "network", v)
			return "", err
		}

		if net.Interface == inet {
			inet = sfcIfname()
		}
	}

	// set pod annotation with nfn-intefaces
	// In this case, we already have annotation.
	networkInfo, err := buildNfnAnnotations(pod, inet, networkname, toDelete)
	if err != nil {
		return "", err
	}
	return networkInfo, nil
}
