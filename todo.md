



**Storage:**
*     Delos: https://research.facebook.com/file/421830459717012/Log-structured-Protocols-in-Delos.pdf
*     Tango: https://github.com/derekelkins/tangohs
*     Similar to Delo: https://github.com/ut-osa/boki
*     https://github.com/corfudb


**Virtual clusters:**

    * https://github.com/karmada-io/karmada/tree/master/operator

    * vCluster vs Karmada -- cost of keeping the upgrading clusters
    https://www.loft.sh/blog/comparing-multi-tenancy-options-in-kubernetes#:~:text=Karmada's%20architecture%20is%20similar%20to%20vcluster.&text=You%20usually%20deploy%20it%20in,them%20to%20the%20local%20cluster.

    * Good read on concurrency in kubernetes: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#types-kinds
    kub design principles: https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/principles.md#control-logic

**Isolation:**

    cgroupv2 : https://facebookmicrosites.github.io/cgroup2/docs/overview

**Kubernetes Good read:**

*   Kubernetes: https://github.com/vmware-archive/tgik/blob/master/episodes/004/README.md

* https://multicluster.sigs.k8s.io/concepts/work-api/

* https://www.cncf.io/blog/2022/09/26/karmada-and-open-cluster-management-two-new-approaches-to-the-multicluster-fleet-management-challenge/

*     kubebuilder : https://book.kubebuilder.io/introduction

*     Controller runtime architecture: https://book.kubebuilder.io/architecture.html
    
*     controller-runtime : https://nakamasato.medium.com/kubernetes-operator-series-2-overview-of-controller-runtime-f8454522a539
    
*     ListWatch: https://www.mgasch.com/2021/01/listwatch-prologue/
    
*     Scaling kub: https://openai.com/index/scaling-kubernetes-to-7500-nodes/
    
* interesting plugins: https://github.com/kubernetes-sigs/scheduler-plugin    
  * Capacity Scheduling
  * Coscheduling
  * Node Resources
  * Node Resource Topology
  *   Preemption Toleration
  *   Trimaran (Load-Aware Scheduling)
  *   Network-Aware Scheduling
