package topologyipsort

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/coscheduling/core"
	"sigs.k8s.io/scheduler-plugins/pkg/util"
)

const NodeRackLabel = "topology.amd.com/rack"

type rackNode struct {
	rack  string
	nodes []node
}

type node struct {
	name string
	ip   int
	pod  string
}

type nodeCache struct {
	podGroup  string
	namespace string
	nodes     []node
}

type TopologyIPSort struct {
	handle framework.Handle
	client client.Client
	logger klog.Logger
	cache  *nodeCache
}

const (
	Name              = "TopologyIPSort"
	TPCountAnnotation = scheduling.GroupName + ".tp"
	EPCountAnnotation = scheduling.GroupName + ".ep"
	CPCountAnnotation = scheduling.GroupName + ".cp"
	PPCountAnnotation = scheduling.GroupName + ".pp"

	ReplicaTypeLabel  = "training.kubeflow.org/replica-type"
	ReplicaIndexLabel = "training.kubeflow.org/replica-index"
	ReplicaMaster     = "master"
)

var _ framework.ScorePlugin = &TopologyIPSort{}
var _ framework.QueueSortPlugin = &TopologyIPSort{}
var _ framework.PermitPlugin = &TopologyIPSort{}
var _ framework.PostFilterPlugin = &TopologyIPSort{}

// New initializes a new plugin and returns it.
func New(ctx context.Context, obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	lh := klog.FromContext(ctx).WithValues("plugin", Name)
	lh.V(5).Info("creating new topologt ip sort plugin")
	scheme := runtime.NewScheme()
	_ = clientscheme.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	c, _, err := util.NewClientWithCachedReader(ctx, handle.KubeConfig(), scheme)
	if err != nil {
		return nil, err
	}
	t := &TopologyIPSort{
		handle: handle,
		client: c,
		logger: lh,
		cache:  &nodeCache{},
	}
	return t, nil
}

func (t *TopologyIPSort) Name() string {
	return Name
}

func (t *TopologyIPSort) rackNodeCache(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, pods []*corev1.Pod, lh klog.Logger) (*nodeCache, error) {
	podGroup := util.GetPodGroupLabel(pod)
	nodes, err := t.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return nil, err
	}
	rackNodes := []rackNode{}
	insertRack := func(rack string, n node) {
		insert := false
		for i := range rackNodes {
			if rackNodes[i].rack == rack {
				insert = true
				rackNodes[i].nodes = append(rackNodes[i].nodes, n)
			}
		}
		if !insert {
			rackNodes = append(rackNodes, rackNode{
				rack:  rack,
				nodes: []node{n},
			})
		}
	}
	for _, n := range nodes {
		rack := n.Node().Labels[NodeRackLabel]
		if t.handle.RunFilterPlugins(ctx, state, pod, n).IsSuccess() {
			insertRack(rack, node{
				name: n.Node().Name,
				ip:   getIPIndex(n),
			})
		}
	}
	for i := range rackNodes {
		sort.Slice(rackNodes[i].nodes, func(j, k int) bool {
			return rackNodes[i].nodes[j].ip < rackNodes[i].nodes[k].ip
		})
	}
	sort.Slice(rackNodes, func(i, j int) bool {
		if len(rackNodes[i].nodes) >= len(rackNodes[j].nodes) {
			return true
		}
		return rackNodes[i].nodes[0].ip > rackNodes[j].nodes[0].ip
	})

	lh.V(4).Info("RackNodeCache", "rackNodes", fmt.Sprintf("%+v", rackNodes))
	podCount := len(pods)
	unit := getUnit(pod)

	filters := filter(rackNodes, unit, podCount)
	for k := range filters {
		filters[k].pod = pods[k].Name
	}

	return &nodeCache{
		podGroup:  podGroup,
		namespace: pod.Namespace,
		nodes:     filters,
	}, nil
}

func (t *TopologyIPSort) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	lh := klog.FromContext(klog.NewContext(ctx, t.logger)).WithValues("ExtensionPoint", "Score")
	pg := util.GetPodGroupLabel(pod)
	pods, err := t.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).List(labels.SelectorFromSet(map[string]string{
		v1alpha1.PodGroupLabel: pg,
	}))
	if err != nil {
		return framework.MinNodeScore, framework.NewStatus(framework.Error, err.Error())
	}
	sort.Slice(pods, func(i, j int) bool {
		if l := less(pods[i], pods[j]); l != nil {
			return *l
		}
		return false
	})
	lh.Info("TopologyIPSort Score", "PodName", pod.Name)
	if t.cache.namespace != pod.Namespace || t.cache.podGroup != pg || (len(pods) > 0 && pods[0].Name == pod.Name) {
		c, err := t.rackNodeCache(ctx, state, pod, pods, lh)
		if err != nil {
			return framework.MinNodeScore, framework.NewStatus(framework.Error, err.Error())
		}
		lh.Info("RackNodeCache", "rackNodesCache", fmt.Sprintf("%+v score pod name %s", c, pod.Name))
		t.cache = c
	}

	for _, n := range t.cache.nodes {
		if n.name == nodeName && n.pod == pod.Name {
			return framework.MaxNodeScore, framework.NewStatus(framework.Success, "success")
		}
	}
	return framework.MinNodeScore, framework.NewStatus(framework.Success, "success")
}

func filter(rackNodes []rackNode, unit int, count int) []node {
	filters := []node{}
	for i := range rackNodes {
		length := len(rackNodes[i].nodes) / unit * unit
		if count == length*(i+1) {
			for j := len(rackNodes) - 1; j >= 0; j-- {
				c := len(rackNodes[i].nodes) / unit * unit
				if c >= length {
					filters = append(filters, rackNodes[j].nodes[:length]...)
				}
				if len(filters) == count {
					break
				}
			}
			break
		}
	}
	if len(filters) < count {
		filters = []node{}
		for i := range rackNodes {
			length := len(rackNodes[i].nodes) / unit * unit
			if count-len(filters) >= length {
				filters = append(filters, rackNodes[i].nodes[:length]...)
			} else {
				filters = append(filters, rackNodes[i].nodes[:count-len(filters)]...)
				break
			}
		}
	}
	if len(filters) < count {
		filters = minimizeVariance(rackNodes, count)
	}
	if len(filters) < count {
		for i := range rackNodes {
			filters = append(filters, rackNodes[i].nodes...)
		}
	}
	sort.Slice(filters, func(i, j int) bool {
		return filters[i].ip < filters[j].ip
	})
	return filters
}

func (t *TopologyIPSort) ScoreExtensions() framework.ScoreExtensions {
	return t
}

func (t *TopologyIPSort) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	return nil
}

func (t *TopologyIPSort) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
	if l := less(podInfo1.Pod, podInfo2.Pod); l != nil {
		return *l
	}
	pg1 := new(v1alpha1.PodGroup)
	err := t.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPodGroupLabel(podInfo1.Pod), Namespace: podInfo1.Pod.Namespace}, pg1)
	if err != nil {
		return true
	}
	pg2 := new(v1alpha1.PodGroup)
	err = t.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPodGroupLabel(podInfo2.Pod), Namespace: podInfo2.Pod.Namespace}, pg2)
	if err != nil {
		return true
	}
	creationTime1 := pg1.CreationTimestamp.Time
	creationTime2 := pg2.CreationTimestamp.Time
	if creationTime1.Equal(creationTime2) {
		return core.GetNamespacedName(podInfo1.Pod) < core.GetNamespacedName(podInfo2.Pod)
	}
	return creationTime1.Before(creationTime2)
}

func (t *TopologyIPSort) Permit(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (*framework.Status, time.Duration) {
	lh := klog.FromContext(klog.NewContext(ctx, t.logger)).WithValues("ExtensionPoint", "Permit")
	pg := util.GetPodGroupLabel(pod)
	if pg == "" {
		return framework.NewStatus(framework.Success, "SUCCESSED"), 0
	}

	pods, err := t.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).List(labels.SelectorFromSet(map[string]string{
		v1alpha1.PodGroupLabel: pg,
	}))
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, "SCHEDULE FAILED at plugin TopologyIPSort"), 0
	}
	for _, p := range pods {
		if !p.DeletionTimestamp.IsZero() {
			return framework.NewStatus(framework.Unschedulable, "SCHEDULE FAILED at plugin TopologyIPSort"), 0
		}
	}
	nodes, err := t.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, "SCHEDULE FAILED at plugin TopologyIPSort"), 0
	}

	scheduled := func(pre *corev1.Pod) bool {
		if pre.Spec.NodeName != "" {
			return true
		}
		for _, n := range nodes {
			for _, p := range n.Pods {
				if p.Pod.UID == pre.UID {
					return true
				}
			}
		}
		return false
	}
	sort.Slice(pods, func(i, j int) bool {
		if l := less(pods[i], pods[j]); l != nil {
			return *l
		}
		return false
	})
	for i := 1; i < len(pods); i++ {
		prePod := pods[i-1]
		if pods[i].Name == pod.Name && !scheduled(prePod) {
			lh.V(4).Info("PreviousPodUnscheduled", "Previous", prePod.Name, "Present", pod.Name)
			return framework.NewStatus(framework.Pending, "SCHEDULE FAILED at plugin TopologyIPSort, Previous pod Unscheduled"), 0
		}
	}
	allocate := map[string]struct{}{
		nodeName: {},
	}
	for _, p := range pods {
		if p.Name == pod.Name {
			continue
		}
		for _, n := range nodes {
			for _, pp := range n.Pods {
				if util.GetPodGroupLabel(pp.Pod) == pg {
					allocate[n.GetName()] = struct{}{}
				}
			}
			_, ok := allocate[n.GetName()]
			if ok {
				continue
			}
			s := t.handle.RunFilterPlugins(ctx, state, p, n)
			if s.IsSuccess() {
				allocate[n.GetName()] = struct{}{}
				break
			}
		}
	}
	podGroup := new(v1alpha1.PodGroup)
	err = t.client.Get(ctx, types.NamespacedName{Name: pg, Namespace: pod.Namespace}, podGroup)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("TopologyIPSort Coscheduler Failed %s", err.Error())), 0
	}
	if int(podGroup.Spec.MinMember) > len(allocate) {
		schedulNodes := []string{}
		for node := range allocate {
			schedulNodes = append(schedulNodes, node)
		}
		sort.Slice(schedulNodes, func(i, j int) bool {
			return schedulNodes[i] < schedulNodes[j]
		})
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("TopologyIPSort Coscheduler Failed %+v", schedulNodes)), 0
	}

	lh.Info("TopologyIPSort Scheduled", "Pod", pod.Name, "NodeName", nodeName)
	return framework.NewStatus(framework.Success, "SUCCESSED"), 0
}

func (t *TopologyIPSort) PostFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod,
	filteredNodeStatusReader framework.NodeToStatusReader) (*framework.PostFilterResult, *framework.Status) {
	lh := klog.FromContext(klog.NewContext(ctx, t.logger)).WithValues("ExtensionPoint", "PostFilter")
	pg := util.GetPodGroupLabel(pod)
	if pg == "" {
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable, "dose not exits pod group")
	}
	nodes, err := t.handle.SnapshotSharedLister().NodeInfos().List()
	schedulNodes := []string{}
	if err != nil {

	}
	for _, node := range nodes {
		s := t.handle.RunFilterPlugins(ctx, state, pod, node)
		if s.Code() != framework.Success {
			continue
		}
		schedulNodes = append(schedulNodes, node.GetName())
	}
	sort.Slice(schedulNodes, func(i, j int) bool {
		return schedulNodes[i] < schedulNodes[j]
	})
	reason := fmt.Sprintf("podgroup %s, %d nodes can be scheduled%+v.", pg, len(schedulNodes), schedulNodes)
	lh.Info(reason)
	return &framework.PostFilterResult{}, framework.NewStatus(framework.Pending, reason).WithPlugin(t.Name())
}

func getIPIndex(node *framework.NodeInfo) int {
	for _, v := range node.Node().Status.Addresses {
		if v.Type == corev1.NodeInternalIP {
			ips := strings.Split(v.Address, ".")
			if len(ips) < 4 {
				continue
			}
			pow := 0
			for i, vv := range ips {
				vvv, err := strconv.Atoi(vv)
				if err != nil {
					continue
				}
				vvv = vvv << (24 - i*8)
				pow += vvv
			}
			return pow
		}
	}
	return 0
}

func less(pod1, pod2 *corev1.Pod) *bool {
	prio1 := corev1helpers.PodPriority(pod1)
	prio2 := corev1helpers.PodPriority(pod2)
	if prio1 != prio2 {
		return pointer.Bool(prio1 > prio2)
	}

	pg1 := util.GetPodGroupLabel(pod1)
	pg2 := util.GetPodGroupLabel(pod2)
	if pg1 == pg2 {
		replicaType1 := pod1.Labels[ReplicaTypeLabel]
		replicaType2 := pod2.Labels[ReplicaTypeLabel]
		if replicaType1 == replicaType2 {
			replicaIndex1 := atoi(pod1.Labels[ReplicaIndexLabel])
			replicaIndex2 := atoi(pod2.Labels[ReplicaIndexLabel])
			return pointer.Bool(replicaIndex1 < replicaIndex2)

		}
		if replicaType1 == ReplicaMaster {
			return pointer.Bool(true)
		}
		return pointer.Bool(false)
	}
	return nil
}

func getUnit(pod *corev1.Pod) int {
	tp := getPCount(pod, TPCountAnnotation)
	ep := getPCount(pod, EPCountAnnotation)
	cp := getPCount(pod, CPCountAnnotation)
	pp := getPCount(pod, PPCountAnnotation)
	return (tp*ep*cp*pp + 7) / 8
}

func getPCount(obj metav1.Object, key string) int {
	n := atoi(getAnnotation(obj, key))
	if n == 0 {
		n = 1
	}
	return n
}

func atoi(str string) int {
	if str == "" {
		return 0
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return n
}

func getAnnotation(obj metav1.Object, key string) string {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return ""
	}
	val, ok := obj.GetAnnotations()[key]
	if !ok {
		return ""
	}
	return val
}

func variance(arr []int) float64 {
	n := len(arr)
	if n == 0 {
		return 0.0
	}
	var sum, sqSum float64
	for _, v := range arr {
		sum += float64(v)
		sqSum += float64(v * v)
	}
	mean := sum / float64(n)
	return sqSum/float64(n) - mean*mean
}

// Allocate count nodes from rackNodes, limiting each allocation to not exceed nodes[i], minimizing variance
func minimizeVariance(rackNodes []rackNode, count int) []node {
	// Build result
	result := []node{}

	// If no nodes need to be allocated, return directly
	if count <= 0 {
		return result
	}

	// If no rackNodes available, return empty
	if len(rackNodes) == 0 {
		return result
	}

	// Calculate available node count for each rack
	rackCounts := make([]int, len(rackNodes))
	for i, rack := range rackNodes {
		rackCounts[i] = len(rack.nodes)
	}

	// Use greedy algorithm to find allocation scheme with minimum variance
	allocation := findMinVarianceAllocation(rackCounts, count)

	// Build result according to allocation scheme
	for i, alloc := range allocation {
		if alloc > 0 && i < len(rackNodes) {
			// Take alloc nodes from this rack
			nodesToTake := alloc
			if nodesToTake > len(rackNodes[i].nodes) {
				nodesToTake = len(rackNodes[i].nodes)
			}
			result = append(result, rackNodes[i].nodes[:nodesToTake]...)
		}
	}

	// Check if allocation was successful - if we couldn't allocate the requested count, return empty
	if len(result) < count {
		return []node{}
	}

	return result
}

// findMinVarianceAllocation uses greedy algorithm to find allocation scheme with minimum variance
func findMinVarianceAllocation(rackCounts []int, totalCount int) []int {
	n := len(rackCounts)

	// Initialize allocation scheme
	allocation := make([]int, n)

	// If no racks available or total demand is 0, return directly
	if n == 0 || totalCount == 0 {
		return allocation
	}

	// Calculate maximum available nodes for each rack
	maxAlloc := make([]int, n)
	for i := range maxAlloc {
		maxAlloc[i] = min(rackCounts[i], totalCount)
	}

	// Strategy: Try to distribute nodes equally across racks
	// First, calculate how many racks we need to use
	racksNeeded := 0
	totalCapacity := 0
	for i := 0; i < n; i++ {
		totalCapacity += maxAlloc[i]
		if totalCapacity >= totalCount {
			racksNeeded = i + 1
			break
		}
	}
	if racksNeeded == 0 {
		racksNeeded = n
	}

	// Calculate allocation per rack
	if racksNeeded > 0 {
		avgPerRack := totalCount / racksNeeded
		remainder := totalCount % racksNeeded

		for i := 0; i < racksNeeded; i++ {
			allocation[i] = avgPerRack
			if i < remainder {
				allocation[i]++
			}
			// Ensure not exceeding maximum capacity
			if allocation[i] > maxAlloc[i] {
				allocation[i] = maxAlloc[i]
			}
		}
	}

	// If we still have remaining nodes, distribute them optimally
	allocated := 0
	for _, alloc := range allocation {
		allocated += alloc
	}

	if allocated < totalCount {
		remaining := totalCount - allocated
		for remaining > 0 {
			bestRack := -1
			bestVariance := math.Inf(1)

			// Try adding one node to each rack, find the scheme with minimum variance
			for i := range allocation {
				if allocation[i] < maxAlloc[i] {
					// Temporarily add one node
					allocation[i]++

					// Calculate variance
					variance := calculateVariance(allocation)

					// If variance is smaller, record this rack
					if variance < bestVariance {
						bestVariance = variance
						bestRack = i
					}

					// Restore original state
					allocation[i]--
				}
			}

			// If found the best rack, add one node
			if bestRack != -1 {
				allocation[bestRack]++
				remaining--
			} else {
				// Cannot allocate more, exit
				break
			}
		}
	}

	return allocation
}

// calculateVariance calculates variance of allocation scheme
func calculateVariance(allocation []int) float64 {
	// Filter out zero values, only calculate racks with allocation
	nonZero := []int{}
	for _, val := range allocation {
		if val > 0 {
			nonZero = append(nonZero, val)
		}
	}

	if len(nonZero) == 0 {
		return 0
	}

	// Calculate variance
	return variance(nonZero)
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
