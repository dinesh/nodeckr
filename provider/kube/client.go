package kube

import (
	"context"
	"fmt"

	"github.com/ericchiang/k8s"
)

type KubeService struct {
	*k8s.Client
}

func NewService(kubeConfigPath string) (*KubeService, error) {
	return nil, nil

	client, err := loadK8sClient(kubeConfigPath)
	if err != nil {
		return nil, err
	}

	return &KubeService{client}, nil
}

func (kc *KubeService) DrainNode(name string) error {
	fieldSelector := k8s.QueryParam("fieldSelector", "spec.nodeName="+name+",metadata.namespace!=kube-system")
	podList, err := kc.Client.CoreV1().ListPods(context.Background(), k8s.AllNamespaces, fieldSelector)
	if err != nil {
		return nil
	}

	filteredPodList := filterOutPodByOwnerReferenceKind(podList.Items, "DaemonSet")

	fmt.Printf("%d pods found in %s\n", len(filteredPodList), name)

	for _, pod := range filteredPodList {
		fmt.Printf("host: %s deleting pod %s", name, *pod.Metadata.Name)

		if err := kc.Client.CoreV1().DeletePod(
			context.Background(),
			*pod.Metadata.Name,
			*pod.Metadata.Namespace,
		); err != nil {
			fmt.Printf("Error draining pod %s: %v", *pod.Metadata.Name, err)
		}
	}

	fmt.Printf("Done Draining Node host:%s\n", name)
	return nil
}
