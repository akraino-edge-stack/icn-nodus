package kube

import (
	"fmt"
	"strings"

	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"

	kapi "k8s.io/api/core/v1"
	kapisnetworking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const apiserverNamespace = "kube-system"
const apiserverPodName = "kube-apiserver-master"
const controlplaneOption = "--service-cluster-ip-range"

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

// SetAnnotationOnPod takes the pod object and key/value string pair to set it as an annotation
func (k *Kube) SetAnnotationOnPod(pod *kapi.Pod, key, value string) error {
	logrus.Infof("Setting annotations %s=%s on pod %s", key, value, pod.Name)
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, value)
	_, err := k.KClient.CoreV1().Pods(pod.Namespace).Patch(pod.Name, types.MergePatchType, []byte(patchData))
	if err != nil {
		logrus.Errorf("Error in setting annotation on pod %s/%s: %v", pod.Name, pod.Namespace, err)
	}
	return err
}

// SetAnnotationOnNode takes the node object and key/value string pair to set it as an annotation
func (k *Kube) SetAnnotationOnNode(node *kapi.Node, key, value string) error {
	logrus.Infof("Setting annotations %s=%s on node %s", key, value, node.Name)
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, value)
	_, err := k.KClient.CoreV1().Nodes().Patch(node.Name, types.MergePatchType, []byte(patchData))
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
	_, err := k.KClient.CoreV1().Namespaces().Patch(ns.Name,
		types.MergePatchType, []byte(patchData))
	if err != nil {
		logrus.Errorf("Error in setting annotation on namespace %s: %v",
			ns.Name, err)
	}
	return err
}

func findServiceIP(a []string, key string) string {
	var clusterCidr string
	for _, n := range a {
		parts := strings.Split(n, "=")
		if len(parts) == 1 {
			continue
		}
		if strings.Compare(parts[0], key) == 0 {
			clusterCidr = parts[1]
			break
		}
	}
	return clusterCidr
}

// GetControlPlaneServiceIPRange return the service IP
func (k *Kube) GetControlPlaneServiceIPRange() (string, error) {
	var clusterIP string
	pod, err := k.GetPod(apiserverNamespace, apiserverPodName)
	if err != nil {
		logrus.Errorf("Error in getting the pod spec details of kube-apiserver-master: %v", err)
	}
	clusterIP = findServiceIP(pod.Spec.Containers[0].Command, controlplaneOption)
	return clusterIP, nil
}

// GetAnnotationsOnPod obtains the pod annotations from kubernetes apiserver, given the name and namespace
func (k *Kube) GetAnnotationsOnPod(namespace, name string) (map[string]string, error) {
	pod, err := k.KClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.ObjectMeta.Annotations, nil
}

// GetPod obtains the Pod resource from kubernetes apiserver, given the name and namespace
func (k *Kube) GetPod(namespace, name string) (*kapi.Pod, error) {
	return k.KClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
}

// GetPods obtains the Pod resource from kubernetes apiserver, given the name and namespace
func (k *Kube) GetPods(namespace string) (*kapi.PodList, error) {
	return k.KClient.CoreV1().Pods(namespace).List(metav1.ListOptions{})
}

// GetPodsByLabels obtains the Pod resources from kubernetes apiserver,
// given the namespace and label
func (k *Kube) GetPodsByLabels(namespace string, selector labels.Selector) (*kapi.PodList, error) {
	options := metav1.ListOptions{}
	options.LabelSelector = selector.String()
	return k.KClient.CoreV1().Pods(namespace).List(options)
}

// GetNodes returns the list of all Node objects from kubernetes
func (k *Kube) GetNodes() (*kapi.NodeList, error) {
	return k.KClient.CoreV1().Nodes().List(metav1.ListOptions{})
}

// GetNode returns the Node resource from kubernetes apiserver, given its name
func (k *Kube) GetNode(name string) (*kapi.Node, error) {
	return k.KClient.CoreV1().Nodes().Get(name, metav1.GetOptions{})
}

// GetService returns the Service resource from kubernetes apiserver, given its name and namespace
func (k *Kube) GetService(namespace, name string) (*kapi.Service, error) {
	return k.KClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
}

// GetEndpoints returns all the Endpoint resources from kubernetes
// apiserver, given namespace
func (k *Kube) GetEndpoints(namespace string) (*kapi.EndpointsList, error) {
	return k.KClient.CoreV1().Endpoints(namespace).List(metav1.ListOptions{})
}

// GetNamespace returns the Namespace resource from kubernetes apiserver,
// given its name
func (k *Kube) GetNamespace(name string) (*kapi.Namespace, error) {
	return k.KClient.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
}

// GetNamespaces returns all Namespace resource from kubernetes apiserver
func (k *Kube) GetNamespaces() (*kapi.NamespaceList, error) {
	return k.KClient.CoreV1().Namespaces().List(metav1.ListOptions{})
}

// GetNetworkPolicies returns all network policy objects from kubernetes
func (k *Kube) GetNetworkPolicies(namespace string) (*kapisnetworking.NetworkPolicyList, error) {
	return k.KClient.NetworkingV1().NetworkPolicies(namespace).List(metav1.ListOptions{})
}
