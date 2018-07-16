package pipeline_controller

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"github.com/golang/glog"
	"k8s.io/client-go/tools/cache"
	"github.com/bwarminski/brett-custom-resource/pkg/apis/demo/v1"
)

// This is the main business logic of the controller

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Pipeline resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the pipeline with this namespace/name
	pipeline, err := c.pipelinesLister.Pipelines(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("pipeline '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	if pipeline.DeletionTimestamp != nil {
		return c.syncDeletingPipeline(pipeline)
	}

	deploymentName := pipeline.Spec.DeploymentName
	if deploymentName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		runtime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	// Get the deployment with the name specified in Foo.spec
	deployment, err := c.deploymentsLister.Deployments(pipeline.Namespace).Get(deploymentName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(pipeline.Namespace).Create(newDeployment(pipeline))
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Deployment is not controlled by this Foo resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(deployment, pipeline) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(pipeline, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// If this number of the replicas on the Foo resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	if pipeline.Spec.Replicas != nil && *pipeline.Spec.Replicas != *deployment.Spec.Replicas {
		glog.V(4).Infof("Foo %s replicas: %d, deployment replicas: %d", name, *pipeline.Spec.Replicas, *deployment.Spec.Replicas)
		deployment, err = c.kubeclientset.AppsV1().Deployments(pipeline.Namespace).Update(newDeployment(pipeline))
	}

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. THis could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// Finally, we update the status block of the Foo resource to reflect the
	// current state of the world
	err = c.updateFooStatus(pipeline, deployment)
	if err != nil {
		return err
	}

	c.recorder.Event(pipeline, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) syncDeletingPipeline(pipeline *v1.Pipeline) error {

}

func (c *Controller) assertInputsInitializedLabelled(pipeline *v1.Pipeline) (*v1.Pipeline, bool, error) {
	ok := true
	pipeline.Spec.In
}