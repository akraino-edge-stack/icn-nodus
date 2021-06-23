package network

import (
	"fmt"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	net "github.com/akraino-edge-stack/icn-nodus/internal/pkg/network"
	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//CreateVirutalNetwork returns
func CreateVirutalNetwork() error {

	isExist, err := net.CheckVirutalNetworkConf()
	if err != nil {
		return err
	}

	if isExist != true {
		log.Info("No need to create virutal network", "FileExist", isExist)
		return nil
	}

	nets, err := net.CalculateVirutalNetworkSubnet()
	if err != nil {
		return err
	}

	log.Info("Value of the virutal Networks", "network-controller value of len(networks)", len(nets))
	log.Info("Value of the virutal Networks", "network-controller value of networks", nets)

	for k, n := range nets {
		err := CreateNetwork(k, n)
		if err != nil {
			return fmt.Errorf("Error in creating virutal Network - %v", err)
		}
	}
	return nil
}

// CreateNetwork create the k8s request for network creation
func CreateNetwork(id int, sn k8sv1alpha1.IpSubnet) error {
	var err error

	log.Info("CreateNetwork", "id", id, "n.name", sn.Name)
	log.Info("CreateNetwork", "id", id, "n.name", sn.Subnet)
	log.Info("CreateNetwork", "id", id, "n.name", sn.Gateway)
	log.Info("CreateNetwork", "id", id, "n.name", sn.ExcludeIps)

	k8sv1alpha1Clientset, err := kube.GetKubev1alpha1Config()
	if err != nil {
		log.Error(err, "Error in getting k8s v1alpha1 clientset")
		return err
	}

	var (
		resp *k8sv1alpha1.Network = &k8sv1alpha1.Network{}
	//	w    watch.Interface
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

	log.Info("Value of the Virutal Network req", "req", req)
	resp, err = k8sv1alpha1Clientset.Networks("default").Create(req)
	if err != nil {
		return err
	}

	log.Info("Value of the Virutal Network created", "resp", resp)

	//To do check the status of the Network creation error

	return nil
}
