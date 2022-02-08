package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"

	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/generated/clientset/versioned/typed/k8s/v1alpha1"

	kapi "k8s.io/api/core/v1"
	kapisnetworking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

const kubeconfigmap = "kubeadm-config"
const kubesystemNamespace = "kube-system"
const nodusKubeConfigFile = "/etc/cni/net.d/ovn4nfv-k8s.d/ovn4nfv-k8s.kubeconfig"

// Interface represents the exported methods for dealing with getting/setting
// kubernetes resources
type Interface interface {
	SetAnnotationOnPod(pod *kapi.Pod, key, value string) error
	SetAnnotationOnNode(node *kapi.Node, key, value string) error
	SetAnnotationOnNamespace(ns *kapi.Namespace, key, value string) error
	GetAnnotationsOnPod(namespace, name string) (map[string]string, error)
	GetPod(namespace, name string) (*kapi.Pod, error)
	GetPods(namespace string) (*kapi.PodList, error)
	GetPodsByLabels(namespace string, selector labels.Selector) (*kapi.PodList, error)
	GetNodes() (*kapi.NodeList, error)
	GetNode(name string) (*kapi.Node, error)
	GetService(namespace, name string) (*kapi.Service, error)
	GetEndpoints(namespace string) (*kapi.EndpointsList, error)
	GetNamespace(name string) (*kapi.Namespace, error)
	GetNamespaces() (*kapi.NamespaceList, error)
	GetNetworkPolicies(namespace string) (*kapisnetworking.NetworkPolicyList, error)
}

// Kube is the structure object upon which the Interface is implemented
type Kube struct {
	KClient kubernetes.Interface
}

// GetKubev1alpha1Config return k8s v1alpha1 Clientset
func GetKubev1alpha1Config() (*k8sv1alpha1.K8sV1alpha1Client, error) {
	var k *k8sv1alpha1.K8sV1alpha1Client
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	k, err = k8sv1alpha1.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Error building Kuberenetes clientset")
		return nil, err
	}

	return k, nil
}

// GetKubeConfig return kubernetes Clientset
func GetKubeConfig() (*kubernetes.Clientset, error) {
	var k *kubernetes.Clientset
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	k, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Error building Kuberenetes clientset")
		return nil, err
	}

	return k, nil
}

// GetKubeConfigfromFile return kubernetes Clientset
func GetKubeConfigfromFile() (*kubernetes.Clientset, error) {
	var k *kubernetes.Clientset

	cfg, err := clientcmd.BuildConfigFromFlags("", nodusKubeConfigFile)
	if err != nil {
		log.Errorf("Error in getting the context for the kubeconfig - %v : %v", nodusKubeConfigFile, err)
		return nil, err
	}

	k, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Error building Kuberenetes clientset")
		return nil, err
	}

	return k, nil
}

// SetAnnotationOnPod takes the pod object and key/value string pair to set it as an annotation
func (k *Kube) SetAnnotationOnPod(pod *kapi.Pod, key, value string) error {
	logrus.Infof("Setting annotations %s=%s on pod %s", key, value, pod.Name)
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, value)
	_, err := k.KClient.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})
	if err != nil {
		logrus.Errorf("Error in setting annotation on pod %s/%s: %v", pod.Name, pod.Namespace, err)
	}
	return err
}

// AppendAnnotationOnPod takes the pod object and key/value string pair to set it as an annotation
func (k *Kube) AppendAnnotationOnPod(pod *kapi.Pod, key, value string) error {
	var err error
	logrus.Infof("Appending Annotation %s=%s on pod %s", key, value, pod.Name)
	if len(pod.Annotations) == 0 {
		pod.Annotations = make(map[string]string)
	}

	//Start code from here
	currrentnfnannotation := pod.Annotations[key]
	if len(currrentnfnannotation) == 0 {
		//create a new annotations
		logrus.Infof(" create a new Annotation")
	}

	var nfnannotationmaps, newnfnannotationmaps []map[string]interface{}

	err = json.Unmarshal([]byte(currrentnfnannotation), &nfnannotationmaps)
	if err != nil {
		return fmt.Errorf("error in unmarshalling current pod annotation - %v", err)
	}

	err = json.Unmarshal([]byte(strings.ReplaceAll(value, "\\", "")), &newnfnannotationmaps)
	if err != nil {
		return fmt.Errorf("error in unmarshalling new annotation value created - %v", err)
	}

	newnfnannotationmaps = append(newnfnannotationmaps, nfnannotationmaps...)

	nfnrawbytes, err := json.Marshal(newnfnannotationmaps)
	if err != nil {
		return fmt.Errorf("error in marshalling new append annotation - %v", err)
	}

	newnfnannotation := string(nfnrawbytes)
	logrus.Infof("Appending Annotation %s=%s on pod %s", key, newnfnannotation, pod.Name)

	kc := k.KClient.CoreV1()
	name := pod.Name
	namespace := pod.Namespace

	r := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod, err = kc.Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		pod.Annotations[key] = newnfnannotation

		_, err = kc.Pods(namespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
		return err
	})

	if r != nil {
		return fmt.Errorf("status update failed for pod %s/%s: %v", pod.Namespace, pod.Name, r)
	}

	pod, err = kc.Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	logrus.Infof("checking the appended Annotation %s=%s on pod %s", key, pod.Annotations[key], pod.Name)
	return nil
}

// SetAnnotationOnNode takes the node object and key/value string pair to set it as an annotation
func (k *Kube) SetAnnotationOnNode(node *kapi.Node, key, value string) error {
	logrus.Infof("Setting annotations %s=%s on node %s", key, value, node.Name)
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, value)
	_, err := k.KClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})
	if err != nil {
		logrus.Errorf("Error in setting annotation on node %s: %v", node.Name, err)
	}
	return err
}

// SetAnnotationOnNamespace takes the Namespace object and key/value pair
// to set it as an annotation
func (k *Kube) SetAnnotationOnNamespace(ns *kapi.Namespace, key,
	value string) error {
	logrus.Infof("Setting annotations %s=%s on namespace %s", key, value,
		ns.Name)
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key,
		value)
	_, err := k.KClient.CoreV1().Namespaces().Patch(context.TODO(), ns.Name,
		types.MergePatchType, []byte(patchData), metav1.PatchOptions{})
	if err != nil {
		logrus.Errorf("Error in setting annotation on namespace %s: %v",
			ns.Name, err)
	}
	return err
}

// GetControlPlaneServiceIPRange return the service IP
func (k *Kube) GetControlPlaneServiceIPRange() (kubeadmtypes.Networking, error) {

	configmap, err := k.KClient.CoreV1().ConfigMaps(kubesystemNamespace).Get(context.TODO(), kubeconfigmap, metav1.GetOptions{})
	if err != nil {
		return kubeadmtypes.Networking{}, fmt.Errorf("Error in gettin the config %s on the namespace %s - %v", kubeconfigmap, kubesystemNamespace, err)
	}
	var clusterconf kubeadmtypes.ClusterConfiguration

	//fmt.Printf("Value of the config Data - %v\n", configmap.Data)
	//fmt.Printf("Value of the config Data[ClusterConfiguration] as string - %v\n", configmap.Data["ClusterConfiguration"])

	j1, err := yaml.YAMLToJSON([]byte(configmap.Data["ClusterConfiguration"]))
	if err != nil {
		return kubeadmtypes.Networking{}, fmt.Errorf("YAMLToJSON err: %v", err)
	}

	//fmt.Printf("Value of j1 =%s\n", string(j1))

	err = json.Unmarshal(j1, &clusterconf)
	if err != nil {
		return kubeadmtypes.Networking{}, fmt.Errorf("Error in un marshalling the config %s on the namespace %s - %v", kubeconfigmap, kubesystemNamespace, err)
	}

	if clusterconf.Networking == (kubeadmtypes.Networking{}) {
		return kubeadmtypes.Networking{}, fmt.Errorf(" %s in the namespace %s - kubeadm network value is empty", kubeconfigmap, kubesystemNamespace)
	}

	//fmt.Printf("Value of the clusterconf Networking ServiceSubnet=%+v\n", clusterconf)
	//svcCidr = controlplaneNetwork.ServiceSubnet
	//podCidr = controlplaneNetwork.PodSubnet

	return clusterconf.Networking, nil

}

// GetAnnotationsOnPod obtains the pod annotations from kubernetes apiserver, given the name and namespace
func (k *Kube) GetAnnotationsOnPod(namespace, name string) (map[string]string, error) {
	pod, err := k.KClient.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.ObjectMeta.Annotations, nil
}

// GetPod obtains the Pod resource from kubernetes apiserver, given the name and namespace
func (k *Kube) GetPod(namespace, name string) (*kapi.Pod, error) {
	return k.KClient.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// GetPods obtains the Pod resource from kubernetes apiserver, given the name and namespace
func (k *Kube) GetPods(namespace string) (*kapi.PodList, error) {
	return k.KClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetPodsByLabels obtains the Pod resources from kubernetes apiserver,
// given the namespace and label
func (k *Kube) GetPodsByLabels(namespace string, selector labels.Selector) (*kapi.PodList, error) {
	options := metav1.ListOptions{}
	options.LabelSelector = selector.String()
	return k.KClient.CoreV1().Pods(namespace).List(context.TODO(), options)
}

// GetNodes returns the list of all Node objects from kubernetes
func (k *Kube) GetNodes() (*kapi.NodeList, error) {
	return k.KClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
}

// GetNode returns the Node resource from kubernetes apiserver, given its name
func (k *Kube) GetNode(name string) (*kapi.Node, error) {
	return k.KClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
}

// GetService returns the Service resource from kubernetes apiserver, given its name and namespace
func (k *Kube) GetService(namespace, name string) (*kapi.Service, error) {
	return k.KClient.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// GetEndpoints returns all the Endpoint resources from kubernetes
// apiserver, given namespace
func (k *Kube) GetEndpoints(namespace string) (*kapi.EndpointsList, error) {
	return k.KClient.CoreV1().Endpoints(namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetNamespace returns the Namespace resource from kubernetes apiserver,
// given its name
func (k *Kube) GetNamespace(name string) (*kapi.Namespace, error) {
	return k.KClient.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
}

// GetNamespaces returns all Namespace resource from kubernetes apiserver
func (k *Kube) GetNamespaces() (*kapi.NamespaceList, error) {
	return k.KClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
}

// GetNetworkPolicies returns all network policy objects from kubernetes
func (k *Kube) GetNetworkPolicies(namespace string) (*kapisnetworking.NetworkPolicyList, error) {
	return k.KClient.NetworkingV1().NetworkPolicies(namespace).List(context.TODO(), metav1.ListOptions{})
}
