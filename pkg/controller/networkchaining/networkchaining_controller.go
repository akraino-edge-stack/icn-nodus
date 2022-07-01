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

package networkchaining

import (
	"context"
	"fmt"
	"reflect"

	"github.com/akraino-edge-stack/icn-nodus/pkg/utils"

	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"

	chaining "github.com/akraino-edge-stack/icn-nodus/internal/pkg/utils"

	notif "github.com/akraino-edge-stack/icn-nodus/internal/pkg/nfnNotify"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_networkchaining")

// Add creates a new NetworkChaining Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNetworkChaining{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("networkchaining-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	log.V(1).Info("Adding the networking chainings")
	// Define Predicates On Create and Update function
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// updates are ingored, if they are already in the "created"
			obj, ok := e.ObjectNew.(*k8sv1alpha1.NetworkChaining)

			if !ok {
				return false
			}

			oldObj, ok := e.ObjectOld.(*k8sv1alpha1.NetworkChaining)
			if !ok {
				return false
			}

			if (obj.Status.State == k8sv1alpha1.CreateInternalError) || (obj.Status.State == k8sv1alpha1.DeleteInternalError) {
				log.V(1).Info("obj.status.Status is internal error", "obj.Status.State", obj.Status.State)
				return true
			}

			if obj.GetGeneration() == oldObj.GetGeneration() && reflect.DeepEqual(oldObj.GetFinalizers(), obj.GetFinalizers()) {
				return false
			}

			log.V(1).Info("value of e.MetaNew.GetGeneration()", "e.MetaNew.GetGeneration()", obj.GetGeneration())
			log.V(1).Info("value of e.MetaOld.GetGeneration()", "e.MetaOld.GetGeneration()", oldObj.GetGeneration())
			log.V(1).Info("value of e.MetaOld.GetFinalizers()", "e.MetaOld.GetFinalizers()", obj.GetFinalizers())
			log.V(1).Info("value of e.MetaNew.GetFinalizers()", "e.MetaNew.GetFinalizers()", oldObj.GetFinalizers())
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log.V(1).Info("create event return true")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.V(1).Info("create delete return true")
			return true
		},
	}

	// Watch for changes to primary resource NetworkChaining
	err = c.Watch(&source.Kind{Type: &k8sv1alpha1.NetworkChaining{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileNetworkChaining implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNetworkChaining{}

// ReconcileNetworkChaining reconciles a NetworkChaining object
type ReconcileNetworkChaining struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}
type reconcileFun func(instance *k8sv1alpha1.NetworkChaining, reqLogger logr.Logger) error

// Reconcile reads that state of the cluster for a NetworkChaining object and makes changes based on the state read
// and what is in the NetworkChaining.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNetworkChaining) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NetworkChaining")
	log.V(1).Info("Entering the reconile")
	// Fetch the NetworkChaining instance
	instance := &k8sv1alpha1.NetworkChaining{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.V(1).Info("Request object not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.V(1).Info("Request object error in reading")
		return reconcile.Result{}, err
	}
	for _, fun := range []reconcileFun{
		r.reconcileFinalizers,
		r.createChain,
	} {
		if err = fun(instance, reqLogger); err != nil {
			log.V(1).Info("err in fun")
			return reconcile.Result{}, err
		}
	}
	log.V(1).Info("return nothing")
	return reconcile.Result{}, nil
}

const (
	nfnNetworkChainFinalizer = "nfnCleanUpNetworkChain"
)

func (r *ReconcileNetworkChaining) checkChain(cr *k8sv1alpha1.NetworkChaining) error {

	err := chaining.CheckNetForNetPool(cr)
	if err != nil {
		return err
	}

	podStatus, err := chaining.CheckSFCPodLabelStatus(cr)
	if err != nil {
		return err
	}

	if podStatus != true {
		cr.Status.State = k8sv1alpha1.Pending
	} else {
		cr.Status.State = k8sv1alpha1.Creating
	}

	err = r.client.Status().Update(context.TODO(), cr)
	if err != nil {
		return err
	}

	if cr.Status.State == k8sv1alpha1.Pending {
		for {
			ps, err := chaining.CheckSFCPodLabelStatus(cr)
			if err != nil {
				return err
			}
			if ps {
				cr.Status.State = k8sv1alpha1.Creating
				err = r.client.Status().Update(context.TODO(), cr)
				if err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}

func (r *ReconcileNetworkChaining) createChain(cr *k8sv1alpha1.NetworkChaining, reqLogger logr.Logger) error {
	log.V(1).Info("Entering the createchain")

	if !cr.DeletionTimestamp.IsZero() {
		// Marked for deletion
		log.V(1).Info("Marked for deletion")
		return nil
	}

	if cr.Status.State == k8sv1alpha1.Created {
		// Already created CR
		log.V(1).Info("Already created chain")
		return nil
	}

	switch {
	case cr.Spec.ChainType == "Routing":
		err := r.checkChain(cr)
		if err != nil {
			log.V(1).Error(err, "Error on chainCheck")
			return err
		}

		//updateStatus, UpdatedChain, err := chaining.CheckForOnlyNFLabel(cr)
		//if err != nil {
		//	reqLogger.Error(err, "Error updating the chain")
		//}

		//if updateStatus == true {
		//	cr.Spec.RoutingSpec.NetworkChain = UpdatedChain
		//}

		log.V(1).Info("Value of networkchain in chain creation", "cr.Spec.RoutingSpec.NetworkChain", cr.Spec.RoutingSpec.NetworkChain)

		podnetworkList, routeList, err := chaining.CalculateRoutes(cr, false, false)
		if err != nil {
			return err
		}

		err = notif.SendRouteNotif(routeList, "create")
		if err != nil {
			cr.Status.State = k8sv1alpha1.CreateInternalError
			reqLogger.Error(err, "Error Sending route Message")
		} else {
			cr.Status.State = k8sv1alpha1.Created
		}

		log.V(1).Info("length of the podnetworkList", "len(podnetworkList)", len(podnetworkList))
		log.V(1).Info("value of the podnetworkList", "podnetworkList", podnetworkList)
		log.V(1).Info("value of the cr.Status.State", "cr.Status.State", cr.Status.State)

		if cr.Status.State != k8sv1alpha1.CreateInternalError {
			err = notif.SendPodNetworkNotif(podnetworkList, "create")
			if err != nil {
				cr.Status.State = k8sv1alpha1.CreateInternalError
				reqLogger.Error(err, "Error Sending pod network Message")
			} else {
				cr.Status.State = k8sv1alpha1.Created
			}
		}

		err = r.client.Status().Update(context.TODO(), cr)
		if err != nil {
			return err
		}
		return nil
		// Add other Chaining types here
	}
	reqLogger.Info("Chaining type not supported", "name", cr.Spec.ChainType)
	return fmt.Errorf("Chaining type not supported")
}

func (r *ReconcileNetworkChaining) deleteChain(cr *k8sv1alpha1.NetworkChaining, reqLogger logr.Logger) error {
	log.V(1).Info("Entering the deletechain")

	if cr.Status.State == k8sv1alpha1.Deleted {
		// Already deleted CR
		log.V(1).Info("Already deleted chain")
		return nil
	}

	switch {
	case cr.Spec.ChainType == "Routing":
		//updateStatus, UpdatedChain, err := chaining.CheckForOnlyNFLabel(cr)
		//if err != nil {
		//	reqLogger.Error(err, "Error updating the chain")
		//}

		//if updateStatus == true {
		//	cr.Spec.RoutingSpec.NetworkChain = UpdatedChain
		//}

		log.Info("Value of networkchain in chain deletion", "cr.Spec.RoutingSpec.NetworkChain", cr.Spec.RoutingSpec.NetworkChain)
		podnetworkList, routeList, err := chaining.CalculateRoutes(cr, true, false)
		if err != nil {
			return err
		}
		err = notif.SendDeleteRouteNotif(routeList, "delete")
		if err != nil {
			cr.Status.State = k8sv1alpha1.DeleteInternalError
			reqLogger.Error(err, "Error Sending route Message")
		} else {
			cr.Status.State = k8sv1alpha1.Deleted
		}

		log.Info("length of the podnetworkList", "len(podnetworkList)", len(podnetworkList))
		log.Info("value of the podnetworkList", "podnetworkList", podnetworkList)
		log.Info("value of the cr.Status.State", "cr.Status.State", cr.Status.State)

		if cr.Status.State != k8sv1alpha1.CreateInternalError {
			err = notif.SendDeletePodNetworkNotif(podnetworkList, "delete")
			if err != nil {
				cr.Status.State = k8sv1alpha1.DeleteInternalError
				reqLogger.Error(err, "Error Sending pod network Message")
			} else {
				cr.Status.State = k8sv1alpha1.Deleted
			}
		}

		err = r.client.Status().Update(context.TODO(), cr)
		if err != nil {
			return err
		}
		return nil
		// Add other Chaining types here
	}
	reqLogger.Info("Chaining type not supported", "name", cr.Spec.ChainType)
	return fmt.Errorf("Chaining type not supported")
}

func (r *ReconcileNetworkChaining) reconcileFinalizers(instance *k8sv1alpha1.NetworkChaining, reqLogger logr.Logger) (err error) {
	log.V(1).Info("Entering the reconcileFinalizers")
	if !instance.DeletionTimestamp.IsZero() {
		// Instance marked for deletion
		if utils.Contains(instance.ObjectMeta.Finalizers, nfnNetworkChainFinalizer) {
			reqLogger.V(1).Info("Finalizer found - delete chain")
			if err = r.deleteChain(instance, reqLogger); err != nil {
				reqLogger.Error(err, "Delete chain")
			}
			// Remove the finalizer even if Delete Network fails. Fatal error retry will not resolve
			instance.ObjectMeta.Finalizers = utils.Remove(instance.ObjectMeta.Finalizers, nfnNetworkChainFinalizer)
			if err = r.client.Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "Removing Finalizer")
				return err
			}
		}

	} else {
		// If finalizer doesn't exist add it
		if !utils.Contains(instance.GetFinalizers(), nfnNetworkChainFinalizer) {
			instance.SetFinalizers(append(instance.GetFinalizers(), nfnNetworkChainFinalizer))
			if err = r.client.Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "Adding Finalizer")
				return err
			}
			reqLogger.V(1).Info("Finalizer added")
		}
	}
	return nil
}
