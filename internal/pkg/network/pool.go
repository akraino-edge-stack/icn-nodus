package network

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
)

const (
	networkpoolconfig = "nodus-dynamic-network-pool"
	networkpoolns     = "kube-system"
)

var m sync.Mutex

//CheckandCreateNetworkPools returns
func CheckandCreateNetworkPools() error {

	var networkPools []k8sv1alpha1.NetworkPool
	isExist, err := CheckVirtualNetworkConf()
	if err != nil {
		return err
	}

	if isExist != true {
		log.Info("No need to create virtual network", "FileExist", isExist)
		return nil
	}

	nets, err := CalculateVirtualNetworkSubnet()
	if err != nil {
		return err
	}

	log.Info("Value of the virtual Networks", "network-controller value of len(networks)", len(nets))
	log.Info("Value of the virtual Networks", "network-controller value of networks", nets)

	for k, n := range nets {
		var np k8sv1alpha1.NetworkPool
		np.PoolNr = k
		np.Network = n
		np.Available = true
		networkPools = append(networkPools, np)
	}

	err = CreateNetworkPool(networkPools)
	if err != nil {
		return fmt.Errorf("Error in creating CreateNetworkPool - %v", err)
	}

	return nil
}

//CreateNetworkPool returns
func CreateNetworkPool(np []k8sv1alpha1.NetworkPool) error {
	var err error

	for id, sn := range np {
		log.Info("CreateNetworkPool", "id", id, "n.name", sn.PoolNr)
		log.Info("CreateNetworkPool", "id", id, "n.Subnet", sn.Network)
		log.Info("CreateNetworkPool", "id", id, "n.Gateway", sn.Available)
	}

	npsmap := make(map[string]string)
	npsByte, err := json.Marshal(np)
	if err != nil {
		return fmt.Errorf("Error in Marshalling nps -%v", err)
	}

	npsmap["networkpools"] = string(npsByte)
	log.Info("CreateNetworkPool", "npsmap", npsmap)

	k8sv1ClientSet, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return fmt.Errorf("Error in getting k8s clientset -%v", err)
	}

	req := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: networkpoolconfig,
		},
		Data: npsmap,
	}

	log.Info("Value of the Virtual Network req", "req", req)
	resp, err := k8sv1ClientSet.CoreV1().ConfigMaps(networkpoolns).Create(req)
	if err != nil {
		return err
	}

	log.Info("Value of the Virtual Network created", "resp", resp)

	return err

}

//UpdateNetworkPool update the config maps
func UpdateNetworkPool(np []k8sv1alpha1.NetworkPool) error {
	var err error

	for id, sn := range np {
		log.Info("CreateNetworkPool", "id", id, "poolNr", sn.PoolNr)
		log.Info("CreateNetworkPool", "id", id, "Network", sn.Network)
		log.Info("CreateNetworkPool", "id", id, "Available", sn.Available)
	}

	npsmap := make(map[string]string)
	npsByte, err := json.Marshal(np)
	if err != nil {
		return fmt.Errorf("Error in Marshalling nps -%v", err)
	}

	npsmap["networkpools"] = string(npsByte)
	log.Info("UpdateNetworkPool", "npsmap", npsmap)

	k8sv1ClientSet, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return fmt.Errorf("Error in getting k8s clientset -%v", err)
	}

	kc := k8sv1ClientSet.CoreV1()
	name := networkpoolconfig
	namespace := networkpoolns
	var cm *corev1.ConfigMap

	r := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm, err = kc.ConfigMaps(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		cm.Data = npsmap

		_, err = kc.ConfigMaps(namespace).Update(cm)
		return err
	})

	if r != nil {
		return fmt.Errorf("status update failed for configmap %s/%s: %v", cm.Namespace, cm.Name, r)
	}

	cm, err = kc.ConfigMaps(cm.Namespace).Get(cm.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	log.Info("Checking the updated config map data", "config map data", cm.Data)
	return nil
}

// CreateNetworkFromPool create the network from the pool
func CreateNetworkFromPool(ns string) error {
	isExist, err := CheckVirtualNetworkConf()
	if err != nil {
		return err
	}

	if isExist != true {
		log.Info("Can't create virtual network as the network pool config doesn't exit", "FileExist", isExist)
		return fmt.Errorf("Can't create virtual network as the network pool config doesn't exit")
	}

	k8sv1ClientSet, err := kube.GetKubeConfig()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return fmt.Errorf("Error in getting k8s clientset -%v", err)
	}

	m.Lock()
	configmaps, err := k8sv1ClientSet.CoreV1().ConfigMaps(networkpoolns).Get(networkpoolconfig, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Error in getting k8s configmaps")
		return fmt.Errorf("Error in getting k8s configmaps -%v", err)
	}

	networkPoolData := configmaps.Data

	val, ok := networkPoolData["networkpools"]
	if !ok {
		err := fmt.Errorf("Error in getting k8s configmaps")
		log.Error(err, "Error in getting the data from the configmaps")
		return err
	}

	var nps []k8sv1alpha1.NetworkPool
	err = json.Unmarshal([]byte(val), &nps)
	if err != nil {
		log.Error(err, "Error in unmarshalling networkpool data")
		return fmt.Errorf("Error in unmarshalling networkpool data -%v", err)
	}

	poollength := len(nps)
	var networkConf k8sv1alpha1.IpSubnet
	for i := 0; i < poollength; i++ {
		n := &nps[i].Network
		log.Info("nps pair: ", "field", i, "PoolNr", nps[i].PoolNr)
		log.Info("nps pair: ", "field", i, "network Name", n.Name)
		log.Info("nps pair: ", "field", i, "network Subnet", n.Subnet)
		log.Info("nps pair: ", "field", i, "network Gateway", n.Gateway)
		log.Info("nps pair: ", "field", i, "network ExcludeIps", n.ExcludeIps)
		log.Info("nps pair: ", "field", i, "Pool Available", nps[i].Available)
		if n.Name != "nil" && nps[i].Available == false {
			if i == poollength-1 {
				err := fmt.Errorf("all the network pools are taken")
				log.Error(err, "All the network pools are taken")
				return err
			}
			continue
		}

		if n.Name == "nil" && nps[i].Available == true {
			n.Name = ns
			nps[i].Available = false
			networkConf = nps[i].Network
			break
		}
	}

	//remove this block after debug
	log.Info("Available network pool to create network", "network name", ns, "network pool", networkConf)
	modifiednpsByte, err := json.Marshal(nps)
	if err != nil {
		log.Error(err, "error in Marshaling the modified network pool data")
		return fmt.Errorf("Error in Marshalling nps -%v", err)
	}

	modifiednpsmap := make(map[string]string)
	modifiednpsmap["networkpools"] = string(modifiednpsByte)
	log.Info("Modified network pools", "networkpool data", modifiednpsmap)
	//remove above block after debug

	err = CreateNetwork(networkConf)
	if err != nil {
		log.Error(err, "Error in the creating the network")
		return fmt.Errorf("Error in the creating the network -%v", err)
	}

	err = UpdateNetworkPool(nps)
	if err != nil {
		log.Error(err, "Error in the updating the config map")
		return fmt.Errorf("Error in the updating the config map -%v", err)
	}

	m.Unlock()
	return nil
}

// CreateNetwork create the k8s request for network creation
func CreateNetwork(sn k8sv1alpha1.IpSubnet) error {
	var err error

	log.Info("CreateNetwork", "name", sn.Name)
	log.Info("CreateNetwork", "Subnet", sn.Subnet)
	log.Info("CreateNetwork", "Gateway", sn.Gateway)
	log.Info("CreateNetwork", "ExcludeIps", sn.ExcludeIps)

	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return err
	}

	var (
		resp *k8sv1alpha1.Network = &k8sv1alpha1.Network{}
		w    watch.Interface
	)

	networklabel := make(map[string]string)
	networklabel["net"] = sn.Name

	req := &k8sv1alpha1.Network{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Network",
			APIVersion: "k8s.plugin.opnfv.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   sn.Name,
			Labels: networklabel,
		},
		Spec: k8sv1alpha1.NetworkSpec{
			CniType: "ovn4nfv",
			Ipv4Subnets: []k8sv1alpha1.IpSubnet{
				{
					Name:       sn.Name,
					Subnet:     sn.Subnet,
					Gateway:    sn.Gateway,
					ExcludeIps: sn.ExcludeIps,
				},
			},
		},
	}

	log.Info("Value of the Virtual Network req", "req", req)
	resp, err = k8sv1alpha1Clientset.Networks("default").Create(req)
	if err != nil {
		return err
	}

	log.Info("Value of the Virtual Network created", "resp", resp)

	status := resp.Status
	w, err = k8sv1alpha1Clientset.Networks("default").Watch(metav1.ListOptions{
		Watch:           true,
		ResourceVersion: resp.ResourceVersion,
		FieldSelector:   fields.Set{"metadata.name": sn.Name}.AsSelector().String(),
		LabelSelector:   labels.SelectorFromSet(networklabel).String(),
	})
	if err != nil {
		return err
	}

	func() {
		for {
			select {
			case events, ok := <-w.ResultChan():
				if !ok {
					return
				}
				resp = events.Object.(*k8sv1alpha1.Network)
				log.Info("Network Status", "network name", sn.Name, "network status", resp.Status.State)
				status = resp.Status
				if resp.Status.State != "" {
					w.Stop()
				}
			case <-time.After(5 * time.Second):
				log.Info("timeout to wait for Network active", "network name", sn.Name)
				w.Stop()
			}
		}
	}()
	if status.State != k8sv1alpha1.Created {
		return fmt.Errorf("Network is not created: %v", status.State)
	}

	return nil
}
