package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Pipeline struct {

	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the custom resource spec
	Spec PipelineSpec `json:"spec"`
	Status PipelineStatus `json:"status"`
}

type PipelineSpec struct {
	Workflow v1alpha1.Workflow `json:"workflow"`
	Inputs map[string]string `json:"inputs"`
	Create *WorkflowParameters `json:"create,omitempty"`
	Update *WorkflowParameters `json:"update,omitempty"`
	Delete *WorkflowParameters `json:"delete,omitempty"`
}

type WorkflowParameters struct {
	Entrypoint *string `json:"entrypoint,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
	ServiceAccount *string `json:"serviceAccount,omitEmpty"`
	InstanceId *string `json:"instanceId,omitEmpty"`
}

type PipelineStatus struct {
	Initialized bool `json:"initialized"`
	WorkflowState *v1alpha1.NodePhase `json:"workflowState"`
	InputValues map[string]string `json:inputValues`
	Outputs map[string]string `json:outputs`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PipelineList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []Pipeline `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ExternalWatcher struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the custom resource spec
	Spec ExternalWatcherSpec `json:"spec"`
	Status PipelineStatus `json:"status"`
}

type ExternalWatcherSpec struct {
	Outputs map[string]string `json:"outputs"`
}

type ExternalWatcherStatus struct {
	Initialized bool `json:"initialized"`
	Outputs map[string]string  `json:"outputs"`
}

func (w *ExternalWatcher) Initialized() bool {
	return w.Status.Initialized
}

func (w *ExternalWatcher) Outputs() map[string]string {
	return w.Status.Outputs
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ExternalWatcherList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []Pipeline `json:"items"`
}

type Input interface {
	Initialized() bool
	Outputs() map[string]string
}
