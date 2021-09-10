package networkpolicy

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
)

var log = logf.Log.WithName("controller_networkpolicy")

// Add creates a new Network Policy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNetworkPolicy{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Create a new Controller that will call the provided Reconciler function in response
	// to events.
	c, err := controller.New("networkpolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Define Predicates On Create and Update function
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.V(1).Info("Network policy - UPDATE\n")

			log.V(1).Info("Old policy:")

			ok := printPolicyObject(e.ObjectOld)

			if !ok {
				return false
			}

			log.V(1).Info("New policy:")
			return printPolicyObject(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log.V(1).Info("Network policy - CREATE\n")

			printPolicyObject(e.Object)

			policy, ok := e.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			list, err := listPods(&mgr, policy)
			if err != nil {
				log.V(1).Info(err.Error())
				return false
			}

			ports := getPorts(list)

			for _, port := range(ports) {
				log.V(1).Info(port)
				log.V(1).Info("\n")
			}

			ovn.PGSetPorts(ovn.DefaultDenyIngress, ports)
			ovn.PGSetPorts(ovn.DefaultDenyEgress, ports)

			ovn.PGAddWithPorts(policy.Namespace + "_" + policy.Name, ports)

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.V(1).Info("Network policy - DELETE\n")

			return printPolicyObject(e.Object)
		},
	}

	// Watch for NetworkPolicy create / update / delete events and call Reconcile
	err = c.Watch(&source.Kind{Type: &networkingv1.NetworkPolicy{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}

	if err = ovn.AddDefaultPG(ovn.Ingress); err != nil {
		return err
	}

	if err = ovn.AddDefaultPG(ovn.Egress); err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcuilePod implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNetworkPolicy{}

// ReconcileNetworkPolicy reconciles a ProviderNetwork object
type ReconcileNetworkPolicy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile function
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNetworkPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Enter Reconciling Network Policy")

	// Fetch the Pod instance
	instance := &networkingv1.NetworkPolicy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Delete Network Policy", "request", request)
			reqLogger.Info("Exit Reconciling Network Policy")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if instance.Name == "" || instance.Namespace == "" {
		return reconcile.Result{}, nil
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Pod is marked delete, process delete allowing appropriate
		// cleanup of ports from OVN CNI
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Exit Reconciling Network Policy")
	return reconcile.Result{}, nil
}


func printPolicyObject(obj runtime.Object) bool {
			policy, ok := obj.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			log.V(1).Info("Name:")
			log.V(1).Info(policy.Name)
			log.V(1).Info("Namespace:")
			log.V(1).Info(policy.Namespace)
			log.V(1).Info("Spec:")
			log.V(1).Info(policy.Spec.String())
		
			return true
}

func listPods(mgr *manager.Manager, policy *networkingv1.NetworkPolicy) (*corev1.PodList, error) {
	c := (*mgr).GetClient()

	options := &client.ListOptions{}
	options.Namespace = policy.Namespace
	options.LabelSelector = labels.SelectorFromSet(policy.Spec.PodSelector.MatchLabels)

	var podList *corev1.PodList

	err := c.List(context.TODO(), podList, options)
	if err != nil {
		return nil, err
	}

	return podList, nil
}

func getPortName(pod *corev1.Pod) string {
	return pod.Namespace + "_" + pod.Name
}

func getPorts(podList *corev1.PodList) []string {
	var ports []string
	for _, pod := range(podList.Items) {
		ports = append(ports, getPortName(&pod))
	}
	return ports
}
