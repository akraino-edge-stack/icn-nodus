package providernetwork

import (
	"context"
	"fmt"
	"reflect"

	notif "github.com/akraino-edge-stack/icn-nodus/internal/pkg/nfnNotify"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	"github.com/akraino-edge-stack/icn-nodus/pkg/utils"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_providernetwork")

// Add creates a new ProviderNetwork Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileProviderNetwork{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("providernetwork-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ProviderNetwork
	err = c.Watch(&source.Kind{Type: &k8sv1alpha1.ProviderNetwork{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileProviderNetwork implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileProviderNetwork{}

// ReconcileProviderNetwork reconciles a ProviderNetwork object
type ReconcileProviderNetwork struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}
type reconcileFun func(instance *k8sv1alpha1.ProviderNetwork, reqLogger logr.Logger) error

// Reconcile reads that state of the cluster for a ProviderNetwork object and makes changes based on the state read
// and what is in the ProviderNetwork.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileProviderNetwork) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ProviderNetwork")

	// Fetch the ProviderNetwork instance
	instance := &k8sv1alpha1.ProviderNetwork{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	for _, fun := range []reconcileFun{
		r.reconcileFinalizers,
		r.createNetwork,
	} {
		if err = fun(instance, reqLogger); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

const (
	nfnProviderNetworkFinalizer = "nfnCleanUpProviderNetwork"
)

func (r *ReconcileProviderNetwork) createNetwork(cr *k8sv1alpha1.ProviderNetwork, reqLogger logr.Logger) error {

	if !cr.DeletionTimestamp.IsZero() {
		// Marked for deletion
		return nil
	}
	switch {
	case cr.Spec.CniType == "ovn4nfv":
		ovnCtl, err := ovn.GetOvnController()
		if err != nil {
			return err
		}
		err = ovnCtl.CreateProviderNetwork(cr)
		if err != nil && !reflect.DeepEqual(err, fmt.Errorf("LS exists")) {
			// Log the error
			reqLogger.Error(err, "Error Creating Network")
			cr.Status.State = k8sv1alpha1.CreateInternalError
		} else {
			err := notif.SendNotif(cr, "create", "")
			if err != nil {
				cr.Status.State = k8sv1alpha1.CreateInternalError
				reqLogger.Error(err, "Error Sending Message")
			} else {
				cr.Status.State = k8sv1alpha1.Created
			}
			err = r.client.Status().Update(context.TODO(), cr)
			if err != nil {
				return err
			}
		}
		// If OVN internal error don't requeue
		return nil
		// Add other CNI types here
	}
	reqLogger.Info("CNI type not supported", "name", cr.Spec.CniType)
	return fmt.Errorf("CNI type not supported")
}

func (r *ReconcileProviderNetwork) deleteNetwork(cr *k8sv1alpha1.ProviderNetwork, reqLogger logr.Logger) error {

	switch {
	case cr.Spec.CniType == "ovn4nfv":
		ovnCtl, err := ovn.GetOvnController()
		if err != nil {
			return err
		}

		notif.SendNotif(cr, "delete", "")

		err = ovnCtl.DeleteProviderNetwork(cr)
		if err != nil {
			// Log the error
			reqLogger.Error(err, "Error Delete Network")
			cr.Status.State = k8sv1alpha1.DeleteInternalError
			err = r.client.Status().Update(context.TODO(), cr)
			if err != nil {
				return err
			}
		}
		// If OVN internal error don't requeue
		return nil
		// Add other CNI types here
	}
	reqLogger.Info("CNI type not supported", "name", cr.Spec.CniType)
	return fmt.Errorf("CNI type not supported")
}

func (r *ReconcileProviderNetwork) reconcileFinalizers(instance *k8sv1alpha1.ProviderNetwork, reqLogger logr.Logger) (err error) {

	if !instance.DeletionTimestamp.IsZero() {
		// Instance marked for deletion
		if utils.Contains(instance.ObjectMeta.Finalizers, nfnProviderNetworkFinalizer) {
			reqLogger.V(1).Info("Finalizer found - delete network")
			if err = r.deleteNetwork(instance, reqLogger); err != nil {
				reqLogger.Error(err, "Delete network")
			}
			// Remove the finalizer even if Delete Network fails. Fatal error retry will not resolve
			instance.ObjectMeta.Finalizers = utils.Remove(instance.ObjectMeta.Finalizers, nfnProviderNetworkFinalizer)
			if err = r.client.Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "Removing Finalize")
				return err
			}
		}

	} else {
		// If finalizer doesn't exist add it
		if !utils.Contains(instance.GetFinalizers(), nfnProviderNetworkFinalizer) {
			instance.SetFinalizers(append(instance.GetFinalizers(), nfnProviderNetworkFinalizer))
			if err = r.client.Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "Adding Finalize")
				return err
			}
			reqLogger.V(1).Info("Finalizer added")
		}
	}
	return nil
}
