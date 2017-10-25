package kube

import (
	"fmt"
	"io/ioutil"

	"github.com/ericchiang/k8s"
	apiv1 "github.com/ericchiang/k8s/api/v1"
	yaml "gopkg.in/yaml.v2"
)

// filterOutPodByOwnerReferenceKind filter out a list of pods by its owner references kind
func filterOutPodByOwnerReferenceKind(podList []*apiv1.Pod, kind string) (output []*apiv1.Pod) {
	for _, pod := range podList {
		for _, ownerReference := range pod.Metadata.OwnerReferences {
			if *ownerReference.Kind != kind {
				output = append(output, pod)
			}
		}
	}

	return
}

// loadK8sClient parses a kubeconfig from a file and returns a Kubernetes
// client. It does not support extensions or client auth providers.
func loadK8sClient(kubeconfigPath string) (*k8s.Client, error) {
	data, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("Read kubeconfig error:\n%v", err)
	}

	// Unmarshal YAML into a Kubernetes config object.
	var config k8s.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("Unmarshal kubeconfig error:\n%v", err)
	}

	return k8s.NewClient(&config)
}
