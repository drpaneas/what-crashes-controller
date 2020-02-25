package main

import (
	"context"
	"os"

	"github.com/go-logr/zapr"
	"github.com/prometheus/common/log"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var restartList map[string]int32

type ReconcilePod struct {
	client client.Client // it reads objects from the cache and writes to teh apiserver
}

func (r *ReconcilePod) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	pod := &v1.Pod{} // Fetch the pod object
	err := r.client.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Error("Pod Not Found. Could have been deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error("Error fetching pod. Going to requeue")
		return reconcile.Result{Requeue: true}, err
	}

	// Write the business logic here
	for i := range pod.Status.ContainerStatuses {
		container := pod.Status.ContainerStatuses[i].Name
		restartCount := pod.Status.ContainerStatuses[i].RestartCount
		identifier := pod.Name + pod.Status.ContainerStatuses[i].Name
		if _, ok := restartList[identifier]; !ok {
			restartList[identifier] = restartCount
		} else if restartList[identifier] < restartCount {
			log.Info("Reconciling container: " + container)
			log.Info(container, restartCount)
			restartList[identifier] = restartCount
		}
	}
	return reconcile.Result{}, nil
}

func main() {
	// Setup the Logger
	log := zapr.NewLogger(zap.NewExample()).WithName("pod-what-crashes")

	restartList = make(map[string]int32)

	// Create a Manager, passing the configuration for KUBECONFIG
	// To watch all namespaces leave the namespace option empty: ""
	log.Info("Setting up the Manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{Namespace: ""})
	if err != nil {
		log.Error(err, "Unable to setup manager. Please check if KUBECONFIG is available")
		os.Exit(1)
	}

	ctrl, err := controller.New("pod-what-crashes", mgr, controller.Options{
		Reconciler: &ReconcilePod{client: mgr.GetClient()},
	})
	if err != nil {
		log.Error(err, "Failed to setup controller")
		os.Exit(1)
	}

	if err := ctrl.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		log.Error(err, "Failed to watch pods")
		os.Exit(1)
	}

	log.Info("Starting up the Manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Failed to start manager")
		os.Exit(1)
	}
}
