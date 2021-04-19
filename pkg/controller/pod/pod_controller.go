package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"ovn4nfv-k8s-plugin/internal/pkg/kube"
	"ovn4nfv-k8s-plugin/internal/pkg/ovn"
	"strings"

	"github.com/mitchellh/mapstructure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_pod")

const (
	nfnNetworkAnnotation = "k8s.plugin.opnfv.org/nfn-network"
)

type nfnNetwork struct {
	Type      string                   "json:\"type\""
	Interface []map[string]interface{} "json:\"interface\""
}

var enableOvnDefaultIntf bool = true

// Add creates a new Pod Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePod{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Create a new Controller that will call the provided Reconciler function in response
	// to events.
	c, err := controller.New("pod-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Define Predicates On Create and Update function
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			annotaion := e.MetaNew.GetAnnotations()
			// The object doesn't contain annotation ,nfnNetworkAnnotation so the event will be
			// ignored.
			if _, ok := annotaion[nfnNetworkAnnotation]; !ok {
				return false
			}
			// If pod is already processed by OVN don't add event
			if _, ok := annotaion[ovn.Ovn4nfvAnnotationTag]; ok {
				return false
			}
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// The object doesn't contain annotation ,nfnNetworkAnnotation so the event will be
			// ignored.
			/*annotaion := e.Meta.GetAnnotations()
			if _, ok := annotaion[nfnNetworkAnnotation]; !ok {
				return false
			}*/
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// The object doesn't contain annotation ,nfnNetworkAnnotation so the event will be
			// ignored.
			/*annotaion := e.Meta.GetAnnotations()
			if _, ok := annotaion[nfnNetworkAnnotation]; !ok {
				return false
			}*/
			return true
		},
	}

	// Watch for Pod create / update / delete events and call Reconcile
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcuilePod implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePod{}

// ReconcilePod reconciles a ProviderNetwork object
type ReconcilePod struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile function
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePod) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Enter Reconciling Pod")

	// Fetch the Pod instance
	instance := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Delete Pod", "request", request)
			r.deleteLogicalPorts(request.Name, request.Namespace)
			reqLogger.Info("Exit Reconciling Pod")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if instance.Name == "" || instance.Namespace == "" {
		return reconcile.Result{}, nil
	}
	if instance.Spec.HostNetwork {
		return reconcile.Result{}, nil
	}

	if instance.Spec.NodeName == "" {
		return reconcile.Result{
			Requeue: true,
		}, nil
	}

	err = r.addLogicalPorts(instance)
	if err != nil && err.Error() == "Failed to add ports" {
		// Requeue the object
		return reconcile.Result{}, err
	}
	reqLogger.Info("Exit Reconciling Pod")
	return reconcile.Result{}, nil
}

// annotatePod annotates pod with the given annotations
func (r *ReconcilePod) setPodAnnotation(pod *corev1.Pod, key, value string) error {

	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, key, value)
	err := r.client.Patch(context.TODO(), pod, client.ConstantPatch(types.MergePatchType, []byte(patchData)))
	if err != nil {
		log.Error(err, "Updating pod failed", "pod", pod, "key", key, "value", value)
		return err
	}
	return nil
}

func (r *ReconcilePod) addLogicalPorts(pod *corev1.Pod) error {

	nfn, err := r.readPodAnnotation(pod)
	if err != nil {
		// No annotation for multiple interfaces
		nfn = &nfnNetwork{Interface: nil}
		if enableOvnDefaultIntf == true {
			nfn.Type = "ovn4nfv"
		} else {
			return err
		}
	}

	switch {
	case nfn.Type == "ovn4nfv":
		ovnCtl, err := ovn.GetOvnController()
		if err != nil {
			return err
		}
		if _, ok := pod.Annotations[ovn.Ovn4nfvAnnotationTag]; ok {
			return fmt.Errorf("Pod annotation found")
		}
		key, value := ovnCtl.AddLogicalPorts(pod, nfn.Interface, false)
		if len(key) > 0 {
			return r.setPodAnnotation(pod, key, value)
		}
		return fmt.Errorf("Failed to add ports")
	default:
		return fmt.Errorf("Unsupported Networking type %s", nfn.Type)
		// Add other types here
	}
}

func (r *ReconcilePod) deleteLogicalPorts(name, namesapce string) error {

	// Run delete for all controllers; pod annonations inaccessible
	ovnCtl, err := ovn.GetOvnController()
	if err != nil {
		return err
	}
	log.Info("Calling DeleteLogicalPorts")
	ovnCtl.DeleteLogicalPorts(name, namesapce)
	return nil
	// Add other types here
}

func (r *ReconcilePod) readPodAnnotation(pod *corev1.Pod) (*nfnNetwork, error) {
	annotaion, ok := pod.Annotations[nfnNetworkAnnotation]
	if !ok {
		return nil, fmt.Errorf("Invalid annotations")
	}
	var nfn nfnNetwork
	err := json.Unmarshal([]byte(annotaion), &nfn)
	if err != nil {
		log.Error(err, "Invalid nfn annotaion", "annotaiton", annotaion)
		return nil, err
	}
	return &nfn, nil
}

//IsPodNetwork return ...
func IsPodNetwork(pod corev1.Pod, networkname string) (bool, error) {
	log.Info("checking the pod network %s on pod %s", networkname, pod.GetName())
	annotations := pod.GetAnnotations()
	annotationsValue, result := annotations[nfnNetworkAnnotation]
	if !result {
		return false, nil
	}

	var nfn nfnNetwork
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

func buildNfnAnnotations(pod corev1.Pod, ifname, networkname string) (string, error) {
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
	nfn := &nfnNetwork{
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

	//netinformation already appended into the pod annotation
	appendednetinfo := strings.ReplaceAll(value, "\\", "")

	return appendednetinfo, nil
}

//AddPodNetworkAnnotations returns ...
func AddPodNetworkAnnotations(pod corev1.Pod, networkname string) (string, error) {
	log.Info("checking the pod network %s on pod %s", networkname, pod.GetName())
	annotations := pod.GetAnnotations()
	sfcIfname := ovn.GetSFCNetworkIfname()
	inet := sfcIfname()
	annotationsValue, result := annotations[nfnNetworkAnnotation]
	if !result {
		// no nfn-network annotations, create a new one
		networkInfo, err := buildNfnAnnotations(pod, inet, networkname)
		if err != nil {
			return "", err
		}
		return networkInfo, nil
	}

	// nfn-network annotations exist, but have to find the interface names to
	// avoid the conflict with the inteface name
	var nfn nfnNetwork
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
	networkInfo, err := buildNfnAnnotations(pod, inet, networkname)
	if err != nil {
		return "", err
	}
	return networkInfo, nil
}
