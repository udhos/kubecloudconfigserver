package main

import (
	"context"
	"errors"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubeClient struct {
	clientset *kubernetes.Clientset
	inCluster bool
	podCache  *podInfo
}

type podInfo struct {
	namespace   string
	listOptions metav1.ListOptions
}

func newKubeClient() (kubeClient, error) {

	kc := kubeClient{}

	config, errConfig := rest.InClusterConfig()
	if errConfig != nil {
		log.Printf("running OUT-OF-CLUSTER: %v", errConfig)
		return kc, nil
	}

	log.Printf("running IN-CLUSTER")
	kc.inCluster = true

	clientset, errClientset := kubernetes.NewForConfig(config)
	if errClientset != nil {
		log.Fatalf("kube clientset error: %v", errClientset)
		return kc, errClientset
	}

	kc.clientset = clientset

	return kc, nil
}

func (k *kubeClient) getPodName() string {
	host, errHost := os.Hostname()
	if errHost != nil {
		log.Printf("getPodName: hostname: %v", errHost)
	}
	return host
}

func (k *kubeClient) getPod() (*corev1.Pod, error) {
	podName := k.getPodName()
	if podName == "" {
		return nil, errors.New("missing pod name")
	}

	namespace := "" // search pod across all namespaces
	pod, errPod := k.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if errPod != nil {
		log.Printf("getPod: could not find pod name='%s': %v", podName, errPod)
	}

	return pod, errPod
}

func isPodReady(pod *v1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func (k *kubeClient) getPodInfo() (*podInfo, error) {
	if k.podCache != nil {
		return k.podCache, nil
	}

	// get my pod
	pod, errPod := k.getPod()
	if errPod != nil {
		log.Printf("getPodInfo: could not find pod: %v", errPod)
		return nil, errPod
	}

	// get namespace from my pod
	namespace := pod.ObjectMeta.Namespace

	// get label from my pod
	labelKey := "app"
	labelValue := pod.ObjectMeta.Labels[labelKey]

	// search other pods using label from my pod
	listOptions := metav1.ListOptions{LabelSelector: labelKey + "=" + labelValue}

	k.podCache = &podInfo{
		namespace:   namespace,
		listOptions: listOptions,
	}

	return k.podCache, nil
}

func (k *kubeClient) listPodsAddresses() ([]string, error) {

	if !k.inCluster {
		return []string{findMyAddr()}, nil
	}

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		log.Printf("listPodsAddresses: pod info: %v", errInfo)
		return nil, errInfo
	}

	pods, errList := k.clientset.CoreV1().Pods(podInfo.namespace).List(context.TODO(), podInfo.listOptions)
	if errList != nil {
		log.Printf("listPodsAddresses: list pods: %v", errList)
		return nil, errList
	}

	var podList []string

	for _, p := range pods.Items {
		if isPodReady(&p) {
			addr := p.Status.PodIP
			podList = append(podList, addr)
		}
	}

	return podList, nil
}

func (k *kubeClient) watchPodsAddresses(out chan<- podAddress) error {

	if !k.inCluster {
		close(out) // notify readers
		return nil // nothing to do
	}

	podInfo, errInfo := k.getPodInfo()
	if errInfo != nil {
		log.Printf("watchPodsAddresses: pod info: %v", errInfo)
		return errInfo
	}

	watch, errWatch := k.clientset.CoreV1().Pods(podInfo.namespace).Watch(context.TODO(), podInfo.listOptions)
	if errWatch != nil {
		log.Printf("watchPodsAddresses: watch: %v", errWatch)
		return errWatch
	}

	in := watch.ResultChan()
	for event := range in {
		pod, ok := event.Object.(*v1.Pod)
		if !ok {
			log.Printf("watchPodsAddresses: unexpected event object: %v", event.Object)
			continue
		}

		addr := pod.Status.PodIP
		ready := isPodReady(pod)

		log.Printf("watchPodsAddresses: event=%s pod=%s addr=%s ready=%t",
			event.Type, pod.Name, addr, ready)

		if event.Type != "MODIFIED" {
			continue
		}

		out <- podAddress{address: addr, added: ready}
	}

	errClosed := errors.New("watchPodsAddresses: input channel has been closed")
	log.Fatalf(errClosed.Error())
	return errClosed
}

type podAddress struct {
	address string
	added   bool
}
