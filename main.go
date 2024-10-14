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
	"math/rand"
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

var CLUSTER_NAME = "cluster"

type NodeWithCluster struct {
	node    *corev1.Node
	cluster string
}

// getClientSet returns a clientset for a given context and kubeconfig path.
// It returns an error if it cannot build the config or create the clientset.
func getClientSet(context string, kubeConfigPath string) (NameClientset, error) {
	// Build the config given the context and kubeconfig path.
	kubeConfig, err := buildConfigWithContextFromFlags(context, kubeConfigPath)
	if err != nil {
		return NameClientset{}, fmt.Errorf("error building kubernetes config: %v\n", err)
	}
	// Create the clientset using the config.
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return NameClientset{}, fmt.Errorf("error building kubernetes clientset: %v\n", err)
	}
	// Return the clientset wrapped in a NameClientset, which holds the clientset and the context.
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

	contextControllerName := flag.String("source-context", "kind-controller", "The name of the kubeconfig context to use for source")
	controllerClientSet, err := getClientSet(*contextControllerName, kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	contextAllocatorName := flag.String("dest-context", "kind-allocator", "The name of the kubeconfig context to use for source")
	contextWk1Name := flag.String("worker-1-context", "kind-work-pool-1", "The name of the kubeconfig context to use for source")
	contextWk2Name := flag.String("worker-2-context", "kind-work-pool-2", "The name of the kubeconfig context to use for source")
	namespace := flag.String("namespace", "default", "The namespace to list pods in")
	flag.Parse()

	allocatorClientset, err := getClientSet(*contextAllocatorName, kubeConfigPath)
	if err != nil {
		fmt.Printf("error  getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	worker1Clientset, err := getClientSet(*contextWk1Name, kubeConfigPath)
	if err != nil {
		fmt.Printf("error  getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	worker2Clientset, err := getClientSet(*contextWk2Name, kubeConfigPath)
	if err != nil {
		fmt.Printf("error  getting kubernetes config: %v\n", err)
		os.Exit(1)
	}

	// stop signal for the informer
	stopper := make(chan struct{})
	defer close(stopper)

	//_, err = ListPods(*namespace, controllerClientSet)
	//if err != nil {
	//	fmt.Println(err.Error())
	//	os.Exit(1)
	//}
	//
	//_, err = ListPods(*namespace, allocatorClientset)
	//if err != nil {
	//	fmt.Println(err.Error())
	//	os.Exit(1)
	//}

	go NodeAllocationSyncer(*namespace, worker2Clientset, allocatorClientset, stopper)
	go NodeAllocationSyncer(*namespace, worker1Clientset, allocatorClientset, stopper)

	// How do fake it without adding too much load on the system.
	// We also have to honor actual issues???
	go simulateHeartbeat(allocatorClientset.clientset, "work-pool-1-worker")
	go simulateHeartbeat(allocatorClientset.clientset, "work-pool-1-worker2")
	go simulateHeartbeat(allocatorClientset.clientset, "work-pool-2-worker")
	go simulateHeartbeat(allocatorClientset.clientset, "work-pool-2-worker2")

	go PodAllocationSyncer(*namespace, controllerClientSet, allocatorClientset, stopper)

	go PodScheduler(*namespace, controllerClientSet, allocatorClientset, worker1Clientset, worker2Clientset, stopper)

	go RunScheduledPod(*namespace, allocatorClientset, worker1Clientset, stopper)

	go RunScheduledPod(*namespace, allocatorClientset, worker2Clientset, stopper)

	//go PodAllocationSyncer(*namespace, allocatorClientset, stopper)

	<-stopper
}

func RunScheduledPod(
	namespace string,
	allocatorClient NameClientset,
	workerClientset NameClientset,
	stopper chan struct{}) {
	// create shared informers for resources in all known API group versions with a reSync period and namespace
	workerFactory := informers.NewSharedInformerFactoryWithOptions(workerClientset.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	workerPodInformer := workerFactory.Core().V1().Pods().Informer()
	workerNodeInformer := workerFactory.Core().V1().Nodes().Informer()

	allocatorFactory := informers.NewSharedInformerFactoryWithOptions(allocatorClient.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	allocatorPodInformer := allocatorFactory.Core().V1().Pods().Informer()

	defer runtime.HandleCrash()

	go workerFactory.Start(stopper)
	go allocatorFactory.Start(stopper)

	allocatorPodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			fmt.Printf("TODO: handle update pod scheduler")
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			//oldPod := oldObj.(*corev1.Pod)
			newPod := newObj.(*corev1.Pod)

			if newPod.Spec.NodeName != "" {

				_, exists, err := workerNodeInformer.GetIndexer().GetByKey(newPod.Spec.NodeName)
				if err != nil {
					return
				}
				if !exists {
					fmt.Printf("Cluster(%s) doesn't have node:%s\n", workerClientset.name, newPod.Spec.NodeName)
					return
				}

				_, podExists, err := workerPodInformer.GetIndexer().GetByKey(newPod.Name)
				if err != nil {
					fmt.Printf("error getting pod in destination cluster: %v\n", err)
					return
				}
				if !podExists {
					podCopy := newPod.DeepCopy()
					podCopy.ObjectMeta.OwnerReferences = nil
					// newPod.ObjectMeta.Finalizers = []string{"kubernetes.io/pod-termination-protection"}
					podCopy.ResourceVersion = "" // Clear the resource version to avoid conflicts
					podCopy.Status = corev1.PodStatus{}
					//newPod.Spec.SchedulerName = "custom-scheduler"
					tenMin := int64(600)
					podCopy.Spec.TerminationGracePeriodSeconds = &tenMin
					podCopy.Spec.RestartPolicy = corev1.RestartPolicyAlways
					p, err := workerClientset.clientset.CoreV1().Pods(podCopy.Namespace).Create(context.TODO(), podCopy, metav1.CreateOptions{})
					if err != nil {
						fmt.Printf("error creating pod(%s) in destination cluster(%s): %v\n", p.Name, workerClientset.name, err)
						return
					}
					fmt.Printf("Pod(%s) created in destination cluster: %s\n", p.Name, workerClientset.name)
					return
				} else {
					fmt.Printf("Handle update for POD in worker node!!!\n")
				}
			}

		},
		DeleteFunc: func(obj interface{}) {
			fmt.Printf("TODO: handle delete pod scheduler\n")
		},
	})
}

func PodScheduler(
	namespace string,
	controllerClientSet NameClientset,
	allocatorClient NameClientset,
	worker1Clientset NameClientset,
	worker2Clientset NameClientset,
	stopper chan struct{}) {

	// create shared informers for resources in all known API group versions with a reSync period and namespace
	worker1Factory := informers.NewSharedInformerFactoryWithOptions(worker1Clientset.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	worker1NodeInformer := worker1Factory.Core().V1().Nodes().Informer()

	// create shared informers for resources in all known API group versions with a reSync period and namespace
	worker2Factory := informers.NewSharedInformerFactoryWithOptions(worker2Clientset.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	worker2NodeInformer := worker2Factory.Core().V1().Nodes().Informer()

	allocatorFactory := informers.NewSharedInformerFactoryWithOptions(allocatorClient.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	allocatorPodInformer := allocatorFactory.Core().V1().Pods().Informer()

	defer runtime.HandleCrash()

	go worker1Factory.Start(stopper)
	go worker2Factory.Start(stopper)
	go allocatorFactory.Start(stopper)

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, worker1NodeInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for worker1NodeInformer to sync\n"))
		return
	}

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, worker2NodeInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for worker2NodeInformer to sync\n"))
		return
	}

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, allocatorPodInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync\n"))
		return
	}

	nodes := make([]NodeWithCluster, 0)
	n := worker1NodeInformer.GetIndexer().List()
	for _, node := range n {
		n := node.(*corev1.Node)
		_, exists := n.Labels["node-role.kubernetes.io/control-plane"]
		if exists {
			continue
		}
		nodes = append(nodes, NodeWithCluster{n, worker1Clientset.name})
	}

	n = worker2NodeInformer.GetIndexer().List()
	for _, node := range n {
		n := node.(*corev1.Node)
		_, exists := n.Labels["node-role.kubernetes.io/control-plane"]
		if exists {
			continue
		}
		nodes = append(nodes, NodeWithCluster{n, worker1Clientset.name})
	}

	allocatorPodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			onAddPodScheduler(obj, allocatorClient, allocatorPodInformer, nodes)
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			fmt.Printf("TODO: handle update pod scheduler\n")
		},
		DeleteFunc: func(obj interface{}) {
			fmt.Printf("TODO: handle delete pod scheduler\n")
		},
	})

}

func bestFitNode(nodes []NodeWithCluster, pod *corev1.Pod) NodeWithCluster {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Generate a random index
	randomIndex := rand.Intn(len(nodes))

	// Return the node at the random index
	return nodes[randomIndex]
}

func onAddPodScheduler(obj interface{}, allocatorClient NameClientset,
	allocatorPodInformer cache.SharedIndexInformer, nodes []NodeWithCluster) {
	pod := obj.(*corev1.Pod)

	if pod.Spec.NodeName != "" {
		fmt.Printf("Pod already scheduled: %s\n", pod.Name)
		return
	}

	node := bestFitNode(nodes, pod)

	//newPod := pod.DeepCopy()
	//newPod.Spec.NodeName = node.node.Name
	//if newPod.Annotations == nil {
	//	newPod.Annotations = make(map[string]string)
	//}
	//newPod.Annotations[CLUSTER_NAME] = node.cluster

	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID},
		Target:     v1.ObjectReference{Kind: "Node", Name: node.node.Name},
	}

	err := allocatorClient.clientset.CoreV1().Pods(pod.Namespace).Bind(context.TODO(), binding, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("error creating pod in destination cluster: %v\n", err)
		return
	}
	fmt.Printf("Pod (%s) scheduled on node(%s)\n", pod.Name, node.node.Name)
}

func NodeAllocationSyncer(namespace string, workerClientset NameClientset,
	allocatorClient NameClientset,
	stopper chan struct{}) {

	// create shared informers for resources in all known API group versions with a reSync period and namespace
	workerFactory := informers.NewSharedInformerFactoryWithOptions(workerClientset.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	workerNodeInformer := workerFactory.Core().V1().Nodes().Informer()

	allocatorFactory := informers.NewSharedInformerFactoryWithOptions(allocatorClient.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	allocatorNodeInformer := allocatorFactory.Core().V1().Nodes().Informer()

	defer runtime.HandleCrash()

	go workerFactory.Start(stopper)
	go allocatorFactory.Start(stopper)

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, workerNodeInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, allocatorNodeInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	// register event handlers on podInformer
	// - AddFunc:   called when a pod is created
	// - UpdateFunc: called when a pod is updated
	// - DeleteFunc: called when a pod is deleted
	workerNodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			onAddNode(obj, allocatorClient, allocatorNodeInformer)
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			onUpdateNode(oldObj, newObj, allocatorClient, allocatorNodeInformer)
		},
		DeleteFunc: func(obj interface{}) {
			onDeleteNode(obj, allocatorClient, allocatorNodeInformer)
		},
	})
}

// PodAllocationSyncer sets up a watcher on the pods in the given namespace.
// It prints to stdout when a pod is created, updated or deleted.
// It stops when the channel `stopper` is closed.
func PodAllocationSyncer(namespace string, controllerClient NameClientset, allocatorClient NameClientset,
	stopper chan struct{}) {

	// create shared informers for resources in all known API group versions with a reSync period and namespace
	controllerFactory := informers.NewSharedInformerFactoryWithOptions(controllerClient.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	controllerPodInformer := controllerFactory.Core().V1().Pods().Informer()

	allocatorFactory := informers.NewSharedInformerFactoryWithOptions(allocatorClient.clientset, 10*time.Minute, informers.WithNamespace(namespace))
	allocatorPodInformer := allocatorFactory.Core().V1().Pods().Informer()

	defer runtime.HandleCrash()

	// start informer ->
	go controllerFactory.Start(stopper)
	go allocatorFactory.Start(stopper)

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, controllerPodInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, allocatorPodInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	// register event handlers on podInformer
	// - AddFunc:   called when a pod is created
	// - UpdateFunc: called when a pod is updated
	// - DeleteFunc: called when a pod is deleted
	controllerPodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			onAdd(obj, allocatorClient, allocatorPodInformer)
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			onUpdate(oldObj, newObj, allocatorClient, allocatorPodInformer)
		},
		DeleteFunc: func(obj interface{}) {
			onDelete(obj, allocatorClient, allocatorPodInformer)
		},
	})
}

func onAddNode(obj interface{}, allocatorClient NameClientset, allocatorNodeInformer cache.SharedIndexInformer) {
	node := obj.(*corev1.Node)
	fmt.Printf("Time:%s NODE CREATED: %s/%s \n", time.Now().Format(time.RFC850), node.Namespace, node.Name)

	// Only add node for worker Role?
	_, exists := node.Labels["node-role.kubernetes.io/control-plane"]
	if exists {
		fmt.Printf("Node %s is a control-plane node\n", node.Name)
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(node)
	if err != nil {
		fmt.Printf("error getting key for node: %v\n", err)
		return
	}
	descObj, exists, err := allocatorNodeInformer.GetIndexer().GetByKey(key)
	if err != nil {
		fmt.Printf("error getting node from cache: %v\n", err)
		return
	}
	if !exists {
		fmt.Printf("node %s not found in cache\n", key)
		newNode := node.DeepCopy()
		newNode.ObjectMeta.OwnerReferences = nil
		// newPod.ObjectMeta.Finalizers = []string{"kubernetes.io/pod-termination-protection"}
		newNode.ResourceVersion = "" // Clear the resource version to avoid conflicts
		p, err := allocatorClient.clientset.CoreV1().Nodes().Create(context.TODO(), newNode, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("error creating node in destination cluster: %v\n", err)
			return
		}
		fmt.Printf("Node created in destination cluster: %s\n", p.Name)
		return
	}

	// Handel update
	descNode := descObj.(*corev1.Node)
	fmt.Printf("updating node %s/%s in dest", descNode.Namespace, descNode.Name)
	//TODO :: Better handling
}

func onUpdateNode(oldObj interface{}, newObj interface{}, allocatorClient NameClientset, allocatorNodeInformer cache.SharedIndexInformer) {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)
	fmt.Printf(
		"Time:%s Node UPDATED. %s/%s %s oldDelete:%s newDelete:%s \n", time.Now().Format(time.RFC850),
		oldNode.Namespace, oldNode.Name, newNode.Status.Phase, newNode.DeletionTimestamp, newNode.DeletionTimestamp,
	)
}

func onDeleteNode(obj interface{}, allocatorClient NameClientset, allocatorNodeInformer cache.SharedIndexInformer) {
	node := obj.(*corev1.Node)
	fmt.Printf("Time:%s NOD DELETED: %s/%s \n", time.Now().Format(time.RFC850), node.Namespace, node.Name)
}

func onAdd(obj interface{}, allocatorClient NameClientset, allocatorPodInformer cache.SharedIndexInformer) {
	pod := obj.(*corev1.Pod)
	fmt.Printf("Time:%s POD CREATED: %s/%s \n", time.Now().Format(time.RFC850), pod.Namespace, pod.Name)

	key, err := cache.MetaNamespaceKeyFunc(pod)
	if err != nil {
		fmt.Printf("error getting key for pod: %v\n", err)
		return
	}
	descObj, exists, err := allocatorPodInformer.GetIndexer().GetByKey(key)
	if err != nil {
		fmt.Printf("error getting pod from cache: %v\n", err)
		return
	}
	if !exists {
		fmt.Printf("pod %s not found in cache\n", key)
		newPod := pod.DeepCopy()
		newPod.ObjectMeta.OwnerReferences = nil
		// newPod.ObjectMeta.Finalizers = []string{"kubernetes.io/pod-termination-protection"}
		newPod.ResourceVersion = "" // Clear the resource version to avoid conflicts
		newPod.Status = corev1.PodStatus{}
		//newPod.Spec.SchedulerName = "custom-scheduler"
		tenMin := int64(600)
		newPod.Spec.TerminationGracePeriodSeconds = &tenMin
		newPod.Spec.RestartPolicy = corev1.RestartPolicyAlways
		p, err := allocatorClient.clientset.CoreV1().Pods(newPod.Namespace).Create(context.TODO(), newPod, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("error creating pod in destination cluster: %v\n", err)
			return
		}
		fmt.Printf("Pod created in destination cluster: %s\n", p.Name)
		return
	}

	// Handel update
	descPod := descObj.(*corev1.Pod)
	fmt.Printf("updating pod %s/%s in dest", descPod.Namespace, descPod.Name)
	//TODO :: Better handling
}

func onUpdate(oldObj interface{}, newObj interface{},
	allocatorClient NameClientset, allocatorPodInformer cache.SharedIndexInformer) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)
	fmt.Printf(
		"Time:%s POD UPDATED. %s/%s %s oldDelete:%s newDelete:%s \n", time.Now().Format(time.RFC850),
		oldPod.Namespace, oldPod.Name, newPod.Status.Phase, oldPod.DeletionTimestamp, newPod.DeletionTimestamp,
	)
}

func onDelete(obj interface{},
	allocatorClient NameClientset, allocatorPodInformer cache.SharedIndexInformer) {
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

// Simulate node heartbeats
func simulateHeartbeat(clientset *kubernetes.Clientset, nodeName string) {
	ticker := time.NewTicker(5 * time.Second) // Simulate heartbeat every 5 seconds
	defer ticker.Stop()

	for range ticker.C {
		// Fetch the node object
		node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("Failed to get node: %v\n", err)
			continue
		}

		// Update the node's status (e.g., NodeReady condition)
		node.Status.Conditions = []v1.NodeCondition{
			{
				Type:               v1.NodeReady,
				Status:             v1.ConditionTrue,
				LastHeartbeatTime:  metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
		}

		_, err = clientset.CoreV1().Nodes().UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("Failed to update node status: %v\n", err)
		}
	}
}
