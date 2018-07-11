package main

import (
	"time"
	"k8s.io/client-go/util/workqueue"

	deploymentClientset "github.com/bwarminski/brett-custom-resource/deployment/pkg/client/clientset/versioned"
	deploymentInformers "github.com/bwarminski/brett-custom-resource/deployment/pkg/client/informers/externalversions/brettdeployment/v1"
	deploymentListers "github.com/bwarminski/brett-custom-resource/deployment/pkg/client/listers/brettdeployment/v1"
	argoClientset "github.com/argoproj/argo/pkg/client/clientset/versioned"
	argoInformers "github.com/argoproj/argo/pkg/client/informers/externalversions/workflow/v1alpha1"
	"github.com/golang/glog"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"fmt"
	"github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
)

type Controller struct {
	deploymentClient deploymentClientset.Interface
	argoClient argoClientset.Interface
	deploymentInformer deploymentInformers.BrettDeploymentInformer
	argoInformer argoInformers.WorkflowInformer
	queue workqueue.RateLimitingInterface
	deploymentLister deploymentListers.BrettDeploymentLister
}

func NewController(
	deploymentClient deploymentClientset.Interface,
	argoClient argoClientset.Interface,
	deploymentInformer deploymentInformers.BrettDeploymentInformer,
	argoInformer argoInformers.WorkflowInformer) *Controller {
	controller := &Controller{
		deploymentClient: deploymentClient,
		argoClient: argoClient,
		deploymentInformer: deploymentInformer,
		argoInformer: argoInformer,
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Deployments"),
	}

	glog.Info("Setting up event handlers")

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDeployment,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDeployment(new)
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)

			if err != nil {
				runtime.HandleError(err)
				return
			}

			controller.queue.AddRateLimited(key)

		},
	})

	argoInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueWorkflow,
		UpdateFunc: func(old, new interface{}) {
			newWf := new.(*v1alpha1.Workflow)
			oldWf := old.(*v1alpha1.Workflow)
			if newWf.ResourceVersion == oldWf.ResourceVersion {
				return
			}
			controller.enqueueWorkflow(new)
		},
		DeleteFunc: controller.enqueueWorkflow,
	})

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <- chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	glog.Info("Starting Deployment controller")

	glog.Info("Waiting for chaches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.deploymentInformer.Informer().HasSynced, c.argoInformer.Informer().HasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

func (c *Controller) enqueueDeployment(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.AddRateLimited(key)
}

func (c *Controller) enqueueWorkflow(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	glog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Foo, we should not do anything more
		// with it.
		if ownerRef.Kind != "Foo" {
			return
		}

		foo, err := c.deploymentLister.BrettDeployments(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			glog.V(4).Infof("ignoring orphaned object '%s' of deployment '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueDeployment(foo)
		return
	}
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {

	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.queue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.queue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.queue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.handle(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}