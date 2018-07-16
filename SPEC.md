# Data Models
Pipeline
spec:
  workflow:
    <Workflow>
  inputs:
    inputName: watcher|pipeline.path
  create:
    entrypoint:
    parameters:
    serviceAccount:
    instanceId:
  update:
  delete:
status:
  initialized:
  workflowState: <state from workflow>
  inputValues:
    inputName: value
  outputs:
    …

ExternalWatcher:
spec:
  outputs:
status:
  initialized:
  outputs:

# Controller Logic

## pipeline controller
if not deleting:
assert inputs initialized & labelled
get input data
assert finalizer
if input data changed, assert workflow not running, delete workflow
assert workflow
get workflow state
assert workflow complete
initialized: true
outputs: workflow outputs

If deleting:
assert other workflows not running
assert delete workflow
assert delete workflow complete, retry on error
delete finalizer & workflow


## external watcher controller
assert output = output, initialized