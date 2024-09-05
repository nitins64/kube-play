package main

import (
	"context"
	"flag"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
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
		fmt.Printf("error getting kubernetes config: %v\n", err)
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

}

func WatchPods(namespace string, ncs NameClientset) {

}

func ListPods(namespace string, ncs NameClientset) (*v1.PodList, error) {
	fmt.Printf("Get Kubernetes Pods from namespace:%s for context: %s\n", namespace, ncs.name)
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
