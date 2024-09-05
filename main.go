package main

import (
	"context"
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"time"
)

func getConfigPath() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("error getting user home dir: %v\n", err)
		os.Exit(1)
	}
	kubeConfigPath := filepath.Join(userHomeDir, "kube-play", "allocator")
	fmt.Printf("Using kubeconfig: %s\n", kubeConfigPath)
	return kubeConfigPath
}

type NameClientset struct {
	clientset *kubernetes.Clientset
	name      string
}

func getClientSet(context string, kubeConfigPath string) (NameClientset, error) {
	kubeConfig, err := buildConfigWithContextFromFlags(context, kubeConfigPath)
	if err != nil {
		return NameClientset{}, fmt.Errorf("error building kubernetes config: %v\n", err)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return NameClientset{}, fmt.Errorf("error building kubernetes clientset: %v\n", err)
	}
	return NameClientset{clientset, context}, nil
}

func buildConfigWithContextFromFlags(context string, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

func main() {
	kubeConfigPath := getConfigPath()

	contextSourceName := flag.String("source-context", "kind-controller", "The name of the kubeconfig context to use for source")
	contextDestName := flag.String("dest-context", "kind-allocator", "The name of the kubeconfig context to use for source")
	namespace := flag.String("namespace", "default", "The namespace to list pods in")
	flag.Parse()

	clientSourceset, err := getClientSet(*contextSourceName, kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	clientDestset, err := getClientSet(*contextDestName, kubeConfigPath)
	if err != nil {
		fmt.Printf("error  getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	// stop signal for the informer
	stopper := make(chan struct{})
	defer close(stopper)

	_, err = ListPods(*namespace, clientSourceset)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	_, err = ListPods(*namespace, clientDestset)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	go WatchPods(*namespace, clientSourceset, stopper)
	go WatchPods(*namespace, clientDestset, stopper)

	<-stopper
}

func WatchPods(namespace string, ncs NameClientset, stopper chan struct{}) {

	// create shared informers for resources in all known API group versions with a reSync period and namespace
	factory := informers.NewSharedInformerFactoryWithOptions(ncs.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	podInformer := factory.Core().V1().Pods().Informer()

	defer runtime.HandleCrash()

	// start informer ->
	go factory.Start(stopper)

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, podInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    onAdd, // register add eventhandler
		UpdateFunc: onUpdate,
		DeleteFunc: onDelete,
	})

}

func onAdd(obj interface{}) {
	pod := obj.(*corev1.Pod)
	fmt.Printf("Time:%s POD CREATED: %s/%s \n", time.Now().Format(time.RFC850), pod.Namespace, pod.Name)
}

func onUpdate(oldObj interface{}, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)
	fmt.Printf(
		"Time:%s POD UPDATED. %s/%s %s oldDelete:%s newDelete:%s \n", time.Now().Format(time.RFC850),
		oldPod.Namespace, oldPod.Name, newPod.Status.Phase, oldPod.DeletionTimestamp, newPod.DeletionTimestamp,
	)
}

func onDelete(obj interface{}) {
	pod := obj.(*corev1.Pod)
	fmt.Printf("Time:%s POD DELETED: %s/%s \n", time.Now().Format(time.RFC850), pod.Namespace, pod.Name)
}

func ListPods(namespace string, ncs NameClientset) (*v1.PodList, error) {
	fmt.Printf("Get Kubernetes Pods from namespace:%s for context: %s\n\n", namespace, ncs.name)
	pods, err := ncs.clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		err = fmt.Errorf("error getting pods: %v\n", err)
		return nil, err
	}
	for _, pod := range pods.Items {
		fmt.Printf("Pod name: %v\n", pod.Name)
	}
	var message string
	if namespace == "" {
		message = "Total Pods in all namespaces"
	} else {
		message = fmt.Sprintf("Total Pods in namespace `%s`", namespace)
	}
	fmt.Printf("%s %d\n", message, len(pods.Items))
	return pods, nil
}

//https://github.com/karmada-io/karmada/tree/master/operator
// https://www.cncf.io/blog/2022/09/26/karmada-and-open-cluster-management-two-new-approaches-to-the-multicluster-fleet-management-challenge/
