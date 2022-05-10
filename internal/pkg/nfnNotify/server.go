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
	"net"
	"os"
	"strings"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/auth"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/node"
	chaining "github.com/akraino-edge-stack/icn-nodus/internal/pkg/utils"
	v1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	clientset "github.com/akraino-edge-stack/icn-nodus/pkg/generated/clientset/versioned"

	pb "github.com/akraino-edge-stack/icn-nodus/internal/pkg/nfnNotify/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kapi "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("rpc-server")

type client struct {
	context *pb.SubscribeContext
	stream  pb.NfnNotify_SubscribeServer
}

type serverDB struct {
	name       string
	clientList map[string]client
	pb.UnimplementedNfnNotifyServer
}

var notifServer *serverDB
var stopChan chan interface{}

var pnClientset *clientset.Clientset
var kubeClientset *kubernetes.Clientset

func newServer() *serverDB {
	return &serverDB{name: "nfnNotifServer", clientList: make(map[string]client)}
}

// Subscribe stores the client information & sends data
func (s *serverDB) Subscribe(sc *pb.SubscribeContext, ss pb.NfnNotify_SubscribeServer) error {
	nodeName := sc.GetNodeName()
	log.Info("Subscribe request from node", "Node Name", nodeName)
	if nodeName == "" {
		return fmt.Errorf("Node name can't be empty")
	}

	nodeIntfMacAddr, nodeIntfIPAddr, nodeIntfIPv6Addr, err := node.AddNodeLogicalPorts(nodeName)
	if err != nil {
		return fmt.Errorf("Error in creating node logical port for node- %s: %v", nodeName, err)
	}
	cp := client{
		context: sc,
		stream:  ss,
	}
	s.clientList[nodeName] = cp

	providerNetworklist, err := pnClientset.K8sV1alpha1().ProviderNetworks("default").List(context.TODO(), v1.ListOptions{})
	if err == nil {
		for _, pn := range providerNetworklist.Items {
			log.Info("Send message", "Provider Network", pn.GetName())
			SendNotif(&pn, "create", nodeName)
		}
	}
	inSyncMsg := pb.Notification{
		CniType: "ovn4nfv",
		Payload: &pb.Notification_InSync{
			InSync: &pb.InSync{
				NodeIntfIpAddress:  nodeIntfIPAddr,
				NodeIntfMacAddress: nodeIntfMacAddr,
				NodeIntfIpv6Address: nodeIntfIPv6Addr,
			},
		},
	}
	log.Info("Send Insync")
	if err = cp.stream.Send(&inSyncMsg); err != nil {
		log.Error(err, "Unable to send sync", "node name", nodeName)
	}
	log.Info("Subscribe Completed")
	// Keep stream open
	for {
		select {
		case <-stopChan:
		}
	}
}

func (s *serverDB) GetClient(nodeName string) client {
	if val, ok := s.clientList[nodeName]; ok {
		return val
	}
	return client{}
}

func updatePnStatus(pn *v1alpha1.ProviderNetwork, status string) error {
	pnCopy := pn.DeepCopy()
	pnCopy.Status.State = status
	_, err := pnClientset.K8sV1alpha1().ProviderNetworks(pn.Namespace).Update(context.TODO(), pnCopy, v1.UpdateOptions{})
	return err
}

func createVlanMsg(pn *v1alpha1.ProviderNetwork) pb.Notification {
	msg := pb.Notification{
		CniType: "ovn4nfv",
		Payload: &pb.Notification_ProviderNwCreate{
			ProviderNwCreate: &pb.ProviderNetworkCreate{
				ProviderNwName: pn.Name,
				Vlan: &pb.VlanInfo{
					VlanId:       pn.Spec.Vlan.VlanId,
					ProviderIntf: pn.Spec.Vlan.ProviderInterfaceName,
					LogicalIntf:  pn.Spec.Vlan.LogicalInterfaceName,
				},
			},
		},
	}
	return msg
}

func deleteVlanMsg(pn *v1alpha1.ProviderNetwork) pb.Notification {
	msg := pb.Notification{
		CniType: "ovn4nfv",
		Payload: &pb.Notification_ProviderNwRemove{
			ProviderNwRemove: &pb.ProviderNetworkRemove{
				ProviderNwName:  pn.Name,
				VlanLogicalIntf: pn.Spec.Vlan.LogicalInterfaceName,
			},
		},
	}
	return msg
}

func createDirectMsg(pn *v1alpha1.ProviderNetwork) pb.Notification {
	msg := pb.Notification{
		CniType: "ovn4nfv",
		Payload: &pb.Notification_ProviderNwCreate{
			ProviderNwCreate: &pb.ProviderNetworkCreate{
				ProviderNwName: pn.Name,
				Direct: &pb.DirectInfo{
					ProviderIntf: pn.Spec.Direct.ProviderInterfaceName,
				},
			},
		},
	}
	return msg
}

func deleteDirectMsg(pn *v1alpha1.ProviderNetwork) pb.Notification {
	msg := pb.Notification{
		CniType: "ovn4nfv",
		Payload: &pb.Notification_ProviderNwRemove{
			ProviderNwRemove: &pb.ProviderNetworkRemove{
				ProviderNwName:     pn.Name,
				DirectProviderIntf: pn.Spec.Direct.ProviderInterfaceName,
			},
		},
	}
	return msg
}

//SendNotif to client
func SendNotif(pn *v1alpha1.ProviderNetwork, msgType string, nodeReq string) error {
	var msg pb.Notification
	var err error

	switch {
	case pn.Spec.CniType == "ovn4nfv":
		switch {
		case pn.Spec.ProviderNetType == "VLAN":
			if msgType == "create" {
				msg = createVlanMsg(pn)
			} else if msgType == "delete" {
				msg = deleteVlanMsg(pn)
			}
			if strings.EqualFold(pn.Spec.Vlan.VlanNodeSelector, "SPECIFIC") {
				for _, label := range pn.Spec.Vlan.NodeLabelList {
					l := strings.Split(label, "=")
					if len(l) == 0 {
						log.Error(fmt.Errorf("Syntax error label: %v", label), "NodeListIterator")
						return nil
					}
				}
				labels := strings.Join(pn.Spec.Vlan.NodeLabelList[:], ",")
				err = sendMsg(msg, labels, "specific", nodeReq)
			} else if strings.EqualFold(pn.Spec.Vlan.VlanNodeSelector, "ALL") {
				err = sendMsg(msg, "", "all", nodeReq)
			} else if strings.EqualFold(pn.Spec.Vlan.VlanNodeSelector, "ANY") {
				if pn.Status.State != v1alpha1.Created {
					err = sendMsg(msg, "", "any", nodeReq)
					if err == nil {
						updatePnStatus(pn, v1alpha1.Created)
					}
				}
			}
		case pn.Spec.ProviderNetType == "DIRECT":
			if msgType == "create" {
				msg = createDirectMsg(pn)
			} else if msgType == "delete" {
				msg = deleteDirectMsg(pn)
			}
			if strings.EqualFold(pn.Spec.Direct.DirectNodeSelector, "SPECIFIC") {
				for _, label := range pn.Spec.Direct.NodeLabelList {
					l := strings.Split(label, "=")
					if len(l) == 0 {
						log.Error(fmt.Errorf("Syntax error label: %v", label), "NodeListIterator")
						return nil
					}
				}
				labels := strings.Join(pn.Spec.Direct.NodeLabelList[:], ",")
				err = sendMsg(msg, labels, "specific", nodeReq)
			} else if strings.EqualFold(pn.Spec.Direct.DirectNodeSelector, "ALL") {
				err = sendMsg(msg, "", "all", nodeReq)
			} else if strings.EqualFold(pn.Spec.Direct.DirectNodeSelector, "ANY") {
				if pn.Status.State != v1alpha1.Created {
					err = sendMsg(msg, "", "any", nodeReq)
					if err == nil {
						updatePnStatus(pn, v1alpha1.Created)
					}
				}
			}
		default:
			return fmt.Errorf("Unsupported Provider Network type")
		}
	default:
		return fmt.Errorf("Unsupported CNI type")
	}
	return err
}

// sendMsg send notification to client
func sendMsg(msg pb.Notification, labels string, option string, nodeReq string) error {
	if option == "all" {
		for name, client := range notifServer.clientList {
			if nodeReq != "" && nodeReq != name {
				continue
			}
			if client.stream != nil {
				if err := client.stream.Send(&msg); err != nil {
					log.Error(err, "Msg Send failed", "Node name", name)
				}
			}
		}
		return nil
	} else if option == "any" {
		// Always select the first
		for _, client := range notifServer.clientList {
			if client.stream != nil {
				if err := client.stream.Send(&msg); err != nil {
					return err
				}
				// return after first successful send
				return nil
			}
		}
		return nil
	}
	// This is specific case
	for name := range nodeListIterator(labels) {
		if nodeReq != "" && nodeReq != name {
			continue
		}
		client := notifServer.GetClient(name)
		if client.stream != nil {
			if err := client.stream.Send(&msg); err != nil {
				return err
			}
		}
	}
	return nil
}

//SendRouteNotif return ...
func SendRouteNotif(chainRoutingInfo []chaining.RoutingInfo, msgType string) error {
	var msg pb.Notification
	var err error

	for _, r := range chainRoutingInfo {
		var ins pb.ContainerRouteInsert
		ins.ContainerId = r.Id
		ins.Route = nil

		//if !r.LeftNetworkRoute.IsEmpty() {
		//	rt := &pb.RouteData{
		//		Dst: r.LeftNetworkRoute.Dst,
		//		Gw:  r.LeftNetworkRoute.GW,
		//	}
		//	ins.Route = append(ins.Route, rt)
		//}

		for _, ln := range r.LeftNetworkRoute {
			rt := &pb.RouteData{
				Dst: ln.Dst,
				Gw:  ln.GW,
			}
			ins.Route = append(ins.Route, rt)
		}

		for _, rn := range r.RightNetworkRoute {
			rt := &pb.RouteData{
				Dst: rn.Dst,
				Gw:  rn.GW,
			}
			ins.Route = append(ins.Route, rt)
		}
		//if !r.RightNetworkRoute.IsEmpty() {
		//	rt := &pb.RouteData{
		//		Dst: r.RightNetworkRoute.Dst,
		//		Gw:  r.RightNetworkRoute.GW,
		//	}
		//	ins.Route = append(ins.Route, rt)
		//}

		for _, d := range r.DynamicNetworkRoutes {
			if !d.IsEmpty() {
				rt := &pb.RouteData{
					Dst: d.Dst,
					Gw:  d.GW,
				}
				ins.Route = append(ins.Route, rt)
			}
		}
		if msgType == "create" {
			msg = pb.Notification{
				CniType: "ovn4nfv",
				Payload: &pb.Notification_ContainterRtInsert{
					ContainterRtInsert: &ins,
				},
			}
		}

		client := notifServer.GetClient(r.Node)
		if client.stream != nil {
			if err := client.stream.Send(&msg); err != nil {
				log.Error(err, "Failed to send msg", "Node", r.Node)
				return err
			}
		}
		// TODO: Handle Delete
	}
	return err
}

//SendDeleteRouteNotif return ...
func SendDeleteRouteNotif(chainRoutingInfo []chaining.RoutingInfo, msgType string) error {
	var msg pb.Notification
	var err error

	for _, r := range chainRoutingInfo {
		var rve pb.ContainerRouteRemove
		rve.ContainerId = r.Id
		rve.Route = nil

		for _, ln := range r.LeftNetworkRoute {
			rt := &pb.RouteData{
				Dst: ln.Dst,
				Gw:  ln.GW,
			}
			rve.Route = append(rve.Route, rt)
		}

		for _, rn := range r.RightNetworkRoute {
			rt := &pb.RouteData{
				Dst: rn.Dst,
				Gw:  rn.GW,
			}
			rve.Route = append(rve.Route, rt)
		}

		//if !r.RightNetworkRoute.IsEmpty() {
		//	rt := &pb.RouteData{
		//		Dst: r.RightNetworkRoute.Dst,
		//		Gw:  r.RightNetworkRoute.GW,
		//	}
		//	rve.Route = append(rve.Route, rt)
		//}

		for _, d := range r.DynamicNetworkRoutes {
			if !d.IsEmpty() {
				rt := &pb.RouteData{
					Dst: d.Dst,
					Gw:  d.GW,
				}
				rve.Route = append(rve.Route, rt)
			}
		}
		if msgType == "delete" {
			msg = pb.Notification{
				CniType: "ovn4nfv",
				Payload: &pb.Notification_ContainterRtRemove{
					ContainterRtRemove: &rve,
				},
			}
		}

		client := notifServer.GetClient(r.Node)
		if client.stream != nil {
			if err := client.stream.Send(&msg); err != nil {
				log.Error(err, "Failed to send msg", "Node", r.Node)
				return err
			}
		}
	}
	return err
}

//SendPodNetworkNotif return ...
func SendPodNetworkNotif(pni []chaining.PodNetworkInfo, msgType string) error {
	var msg pb.Notification
	var err error

	for _, p := range pni {
		var add pb.PodAddNetwork
		add.Pod = &pb.PodInfo{
			Namespace: p.Namespace,
			Name:      p.Name,
		}
		add.ContainerId = p.Id
		add.Net = &pb.NetConf{
			Data: p.NetworkInfo,
		}

		for _, d := range p.Route {
			if !d.IsEmpty() {
				rt := &pb.RouteData{
					Dst: d.Dst,
					Gw:  d.GW,
				}
				add.Route = append(add.Route, rt)
			}
		}
		//add.Route = &pb.RouteData{
		//	Dst: p.Route.Dst,
		//	Gw:  p.Route.GW,
		//}

		if msgType == "create" {
			msg = pb.Notification{
				CniType: "ovn4nfv",
				Payload: &pb.Notification_PodAddNetwork{
					PodAddNetwork: &add,
				},
			}
		}
		client := notifServer.GetClient(p.Node)
		if client.stream != nil {
			if err := client.stream.Send(&msg); err != nil {
				log.Error(err, "Failed to send msg", "Node", p.Node)
				return err
			}
		}
		// TODO: Handle Delete
	}
	return err
}

//SendDeletePodNetworkNotif return ...
func SendDeletePodNetworkNotif(pni []chaining.PodNetworkInfo, msgType string) error {
	var msg pb.Notification
	var err error

	for _, p := range pni {
		var rve pb.PodDelNetwork
		rve.Pod = &pb.PodInfo{
			Namespace: p.Namespace,
			Name:      p.Name,
		}
		rve.ContainerId = p.Id
		rve.Net = &pb.NetConf{
			Data: p.NetworkInfo,
		}

		for _, d := range p.Route {
			if !d.IsEmpty() {
				rt := &pb.RouteData{
					Dst: d.Dst,
					Gw:  d.GW,
				}
				rve.Route = append(rve.Route, rt)
			}
		}
		//rve.Route = &pb.RouteData{
		//	Dst: p.Route.Dst,
		//	Gw:  p.Route.GW,
		//}

		if msgType == "delete" {
			msg = pb.Notification{
				CniType: "ovn4nfv",
				Payload: &pb.Notification_PodDelNetwork{
					PodDelNetwork: &rve,
				},
			}
		}
		client := notifServer.GetClient(p.Node)
		if client.stream != nil {
			if err := client.stream.Send(&msg); err != nil {
				log.Error(err, "Failed to send msg", "Node", p.Node)
				return err
			}
		}
	}
	return err
}

func nodeListIterator(labels string) <-chan string {
	ch := make(chan string)

	lo := v1.ListOptions{LabelSelector: labels}
	// List the Nodes matching the Labels
	nodes, err := kubeClientset.CoreV1().Nodes().List(context.TODO(), lo)
	if err != nil {
		log.Info("No Nodes found with labels", "list:", lo)
		return nil
	}
	go func() {
		for _, node := range nodes.Items {
			log.Info("Send message to", " node:", node.ObjectMeta.Name)
			ch <- node.ObjectMeta.Name
		}
		close(ch)
	}()
	return ch
}

//SetupNotifServer initilizes the gRpc nfn notif server
func SetupNotifServer(kConfig *rest.Config) {
	log.Info("Starting Notif Server")
	var err error

	// creates the clientset
	pnClientset, err = clientset.NewForConfig(kConfig)
	if err != nil {
		log.Error(err, "Error building clientset")
	}
	kubeClientset, err = kubernetes.NewForConfig(kConfig)
	if err != nil {
		log.Error(err, "Error building Kuberenetes clientset")
	}

	stopChan = make(chan interface{})

	// Start GRPC server
	lis, err := net.Listen("tcp", ":50000")
	if err != nil {
		log.Error(err, "failed to listen")
	}

	namespace := os.Getenv(auth.NamespaceEnv)
	nfnSvcIP := os.Getenv(auth.NfnOperatorHostEnv)

	kubecli := &kube.Kube{KClient: kubeClientset}

	// get certificate
	crt, err := auth.GetCert(namespace, auth.DefaultCert)
	if err != nil {
		log.Error(err, "Error while obtaining the certificate")
	}

	// load secret associated with obtained certificate
	var sec *kapi.Secret
	if !auth.IsCertIPUpToDate(crt, nfnSvcIP) {
		// update certificate IP if not up-to-date
		crt, sec, err = auth.UpdateCertIP(crt, nfnSvcIP)
		if err != nil {
			log.Error(err, "Error while updating the certificate")
		}
	} else {
		// wait for secret to be updated
		sec, err = auth.WaitForSecretIP(kubecli, crt)
	}
	if err != nil {
		log.Error(err, "Error while obtaining secret")
	}

	// create TLS config from secret
	serverTLS, err := auth.CreateServerTLSFromSecret(sec)
	if err != nil {
		log.Error(err, "Error while creating TLS configuration")
	}

	s := grpc.NewServer(grpc.Creds(*serverTLS))
	// Intialize Notify server
	notifServer = newServer()
	pb.RegisterNfnNotifyServer(s, notifServer)

	reflection.Register(s)
	log.Info("Initialization Completed")
	if err := s.Serve(lis); err != nil {
		log.Error(err, "failed to serve")
	}
}
