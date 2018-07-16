package pipeline_controller

import (
	//"fmt"
	//"time"
	//
	//"github.com/golang/glog"
	//appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/runtime/schema"
	//"k8s.io/apimachinery/pkg/util/runtime"
	//"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	//
	//samplev1alpha1 "k8s.io/sample-controller/pkg/apis/samplecontroller/v1alpha1"
	clientset "github.com/bwarminski/brett-custom-resource/pkg/client/clientset/versioned"
	//samplescheme "k8s.io/sample-controller/pkg/client/clientset/versioned/scheme"
	demoscheme "github.com/bwarminski/brett-custom-resource/pkg/client/clientset/versioned/scheme"
	informers "github.com/bwarminski/brett-custom-resource/pkg/client/informers/externalversions/demo/v1"
	listers "github.com/bwarminski/brett-custom-resource/pkg/client/listers/demo/v1"
	argolisters "github.com/argoproj/argo/pkg/client/listers/workflow/v1alpha1"
	argoinformers "github.com/argoproj/argo/pkg/client/informers/externalversions/workflow/v1alpha1"
	"github.com/golang/glog"
	argo "github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"fmt"
	"time"
	"k8s.io/apimachinery/pkg/util/wait"
	"github.com/bwarminski/brett-custom-resource/pkg/apis/demo/v1"
	"strings"
)

const (
	InputLabelPrefix = "demo.input."
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// democlientset is a clientset for our own API group
	democlientset clientset.Interface

	workflowsLister        argolisters.WorkflowLister
	workflowsSynced        cache.InformerSynced
	pipelinesLister        listers.PipelineLister
	pipelinesSynced        cache.InformerSynced
	externalWatchersLister listers.ExternalWatcherLister
	externalWatchersSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

func NewController(
	kubeclientset kubernetes.Interface,
	democlientset clientset.Interface,
	workflowInformer argoinformers.WorkflowInformer,
	pipelineInformer informers.PipelineInformer,
    externalWatcherInformer informers.ExternalWatcherInformer) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	demoscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "demo-pipeline-controller"})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		democlientset:     democlientset,
		workflowsLister:   workflowInformer.Lister(),
		workflowsSynced:   workflowInformer.Informer().HasSynced,
		pipelinesLister:        pipelineInformer.Lister(),
		pipelinesSynced:        pipelineInformer.Informer().HasSynced,
		externalWatchersLister: externalWatcherInformer.Lister(),
		externalWatchersSynced: externalWatcherInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Pipelines"),
		recorder:          recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when Pipeline resources change
	pipelineInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueuePipeline,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueuePipeline(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Foo resource will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	workflowInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleWorkflow,
		UpdateFunc: func(old, new interface{}) {
			newWorkflow := new.(*argo.Workflow)
			oldWorkflow := old.(*argo.Workflow)
			if newWorkflow.ResourceVersion == oldWorkflow.ResourceVersion {
				// Periodic resync will send update events for all known workflows.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleWorkflow(new)
		},
		DeleteFunc: controller.handleWorkflow,
	})

	externalWatcherInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleWatcher,
		UpdateFunc: func(old, new interface{}) {
			newWatcher := new.(*v1.ExternalWatcher)
			oldWatcher := old.(*v1.ExternalWatcher)
			if newWatcher.ResourceVersion == oldWatcher.ResourceVersion {
				// Periodic resync will send update events for all known workflows.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleWatcher(new)
		},
		DeleteFunc: controller.handleWatcher,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting Foo controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.workflowsSynced, c.pipelinesSynced, c.externalWatchersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

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
		defer c.workqueue.Done(obj)
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
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// enqueuePipeline takes a Pipeline resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Pipeline.
func (c *Controller) enqueuePipeline(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) handleWorkflow(obj interface{}) {
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
		// If this object is not owned by a Pipeline, we should not do anything more
		// with it.
		if ownerRef.Kind != "Pipeline" {
			return
		}

		pipeline, err := c.pipelinesLister.Pipelines(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			glog.V(4).Infof("ignoring orphaned object '%s' of pipeline '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueuePipeline(pipeline)
		return
	}
}

func (c *Controller) handleWatcher(obj interface{}) {
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

	labels := object.GetLabels()
	if labels != nil {
		for label := range labels {
			if strings.HasPrefix(label, InputLabelPrefix) {
				pipeline, err := c.pipelinesLister.Pipelines(object.GetNamespace()).Get(label)
				if err != nil {
					glog.V(4).Infof("ignoring orphaned watcher input '%s' of pipeline '%s'", object.GetSelfLink(), label)
					continue
				}

				c.enqueuePipeline(pipeline)
			}
		}
	}
	return
}



