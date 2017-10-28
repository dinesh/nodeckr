package kube

import (
	"context"
	"fmt"

	"github.com/ericchiang/k8s"
	apiv1 "github.com/ericchiang/k8s/api/v1"
	"github.com/rs/zerolog/log"
)

type KubeService struct {
	*k8s.Client
}

func NewService(kubeConfigPath string) (*KubeService, error) {
	client, err := loadK8sClient(kubeConfigPath)
	if err != nil {
		return nil, err
	}

	return &KubeService{client}, nil
}

//GetNode return the node object from given name
func (kc *KubeService) GetNode(name string) (node *apiv1.Node, err error) {
	node, err = kc.Client.CoreV1().GetNode(context.Background(), name)
	return
}

func (kc *KubeService) SetUnschedulableState(name string, unschedulable bool) (err error) {
	node, err := kc.GetNode(name)

	if err != nil {
		err = fmt.Errorf("Error getting node information before setting unschedulable state:\n%v", err)
		return
	}

	node.Spec.Unschedulable = &unschedulable

	_, err = kc.Client.CoreV1().UpdateNode(context.Background(), node)
	return
}

func (kc *KubeService) DrainNode(name string) error {
	fieldSelector := k8s.QueryParam("fieldSelector", "spec.nodeName="+name+",metadata.namespace!=kube-system")
	podList, err := kc.Client.CoreV1().ListPods(context.Background(), k8s.AllNamespaces, fieldSelector)
	if err != nil {
		return err
	}

	filteredPodList := filterOutPodByOwnerReferenceKind(podList.Items, "DaemonSet")
	log.Info().Str("node", name).Msgf("%s pods found", len(filteredPodList))

	for _, pod := range filteredPodList {
		log.Info().Str("node", name).Msgf("deleteting pod %s", *pod.Metadata.Name)

		if err := kc.Client.CoreV1().DeletePod(
			context.Background(),
			*pod.Metadata.Name,
			*pod.Metadata.Namespace,
		); err != nil {
			return fmt.Errorf("Error draining pod %s: %v", *pod.Metadata.Name, err)
		}
	}

	return nil
}
