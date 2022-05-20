package openshift

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	operv1 "github.com/openshift/api/operator/v1"
	confv1 "github.com/openshift/api/config"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	openshiftConfigName      = "cluster"
	openshiftConfigNamespace = "openshift-network-operator"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme will be used to register OpenShift's Network type
	AddToScheme   = schemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	schemeGroupVersion := schema.GroupVersion{Group: confv1.GroupName, Version: "v1"}
    scheme.AddKnownTypes(schemeGroupVersion,
		&operv1.Network{},
    )

    metav1.AddToGroupVersion(scheme, schemeGroupVersion)
    return nil
}

// OpenShift is a type to handle communication with OpenShiftApi
type OpenShift struct {
	kube.Kube
}

// GetOpenShiftClient can be used to obtain a pointer to OpenShift client
func GetOpenShiftClient() (*OpenShift, error) {
	clientset, err := kube.GetKubeConfig()
	if err != nil {
		return nil, err
	}

	kubecli := &kube.Kube{KClient: clientset}
	return &OpenShift{*kubecli}, nil
}

// GetControlPlaneServiceIPRange return the service IP
func (os *OpenShift) GetControlPlaneServiceIPRange() (kubeadmtypes.Networking, error) {
	config, err := config.GetConfig()
	if err != nil {
		return kubeadmtypes.Networking{},  fmt.Errorf("Error in getting kubernetes config: %v", err)
	}

	crcli, err := crclient.New(config, crclient.Options{})
	if err != nil {
		return kubeadmtypes.Networking{},  fmt.Errorf("Error while creating controller-runtime client: %v", err)
	}

	netConf := &operv1.Network{TypeMeta: metav1.TypeMeta{APIVersion: confv1.GroupName, Kind: "Network"}}
	if err = crcli.Get(context.TODO(), types.NamespacedName{Name: openshiftConfigName}, netConf); err != nil {
		return kubeadmtypes.Networking{},  fmt.Errorf("Error while getting OpenShift cluster configuration: %v", err)
	}

	// return if OpenShift is being used
	return kubeadmtypes.Networking{
		PodSubnet: netConf.Spec.ClusterNetwork[0].CIDR,
		ServiceSubnet: netConf.Spec.ServiceNetwork[0],
	}, nil
}
