package networkpolicy

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
)

const (
	// NodusNetworkPolicyAnnotationTag will be used to annotate pod that has already been processed by Network Policy update 
	NodusNetworkPolicyAnnotationTag = "NodusNetworkPolicyUpdated"
	ipv4Delimeter = "."
)

var log = logf.Log.WithName("controller_networkpolicy")

// Add creates a new Network Policy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNetworkPolicy{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Create a new Controller that will call the provided Reconciler function in response
	// to events.
	c, err := controller.New("networkpolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Define Predicates On Create and Update function
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPolicy, ok := e.ObjectOld.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			newPolicy, ok := e.ObjectOld.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			return updatePolicy(&mgr, oldPolicy, newPolicy)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			policy, ok := e.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			logPolicyInfo("Creating policy", policy)
			return createPolicy(&mgr, policy)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			policy, ok := e.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			logPolicyInfo("Deleting policy", policy)
			return deletePolicy(&mgr, policy)
		},
	}

	// Watch for NetworkPolicy create / update / delete events and call Reconcile
	err = c.Watch(&source.Kind{Type: &networkingv1.NetworkPolicy{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcuilePod implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNetworkPolicy{}

// ReconcileNetworkPolicy reconciles a ProviderNetwork object
type ReconcileNetworkPolicy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile function
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNetworkPolicy) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Enter Reconciling Network Policy")

	// Fetch the Network Policy instance
	instance := &networkingv1.NetworkPolicy{}
	err := r.client.Get(ctx, request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Delete Network Policy", "request", request)
			reqLogger.Info("Exit Reconciling Network Policy")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if instance.Name == "" || instance.Namespace == "" {
		return reconcile.Result{}, nil
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Pod is marked delete, process delete allowing appropriate
		// cleanup of ports from OVN CNI
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Exit Reconciling Network Policy")
	return reconcile.Result{}, nil
}

// XgressRule will hold data from k8s NetworkPolicyIngress/EgressRule structs
type XgressRule struct {
	Ports []networkingv1.NetworkPolicyPort
	Peer []networkingv1.NetworkPolicyPeer
	Type ovn.PolicyDirection
}


// fromIngress translates k8s Ingress struct into Xgress
func fromIngress(rules []networkingv1.NetworkPolicyIngressRule) []XgressRule {
	var xgressRules []XgressRule
	for _, rule := range(rules) {
		var xgressRule XgressRule
		xgressRule.Ports = rule.Ports
		xgressRule.Peer = rule.From
		xgressRule.Type = ovn.Ingress
		xgressRules = append(xgressRules, xgressRule)
	}
	return xgressRules
}

// fromEgress translates k8s Egress struct into Xgress
func fromEgress(rules []networkingv1.NetworkPolicyEgressRule) []XgressRule {
	var xgressRules []XgressRule
	for _, rule := range(rules) {
		var xgressRule XgressRule
		xgressRule.Ports = rule.Ports
		xgressRule.Peer = rule.To
		xgressRule.Type = ovn.Egress
		xgressRules = append(xgressRules, xgressRule)
	}
	return xgressRules
}

func listPods(mgr *manager.Manager, policy *networkingv1.NetworkPolicy, namespace string) (*corev1.PodList, error) {
	c := (*mgr).GetClient()

	options := &client.ListOptions{}
	options.Namespace = policy.Namespace
	options.LabelSelector = labels.SelectorFromSet(policy.Spec.PodSelector.MatchLabels)

	podList := &corev1.PodList{}

	err := c.List(context.TODO(), podList, options)
	if err != nil {
		log.Error(err, "Error occurred while listing pods")
		return nil, err
	}

	return podList, nil
}

func getPortName(pod *corev1.Pod) string {
	return pod.Namespace + "_" + pod.Name
}

func getPorts(podList *corev1.PodList) []string {
	var ports []string
	for _, pod := range(podList.Items) {
		ports = append(ports, getPortName(&pod))
	}
	return ports
}

func getIPs(podList *corev1.PodList) []string {
	var ips []string
	for _, pod := range(podList.Items) {
		ips = append(ips, pod.Status.PodIP)
	}
	return ips
}

// PolicyPorts describes internal port definition
type PolicyPorts struct {
	TCPPorts []int
	UDPPorts []int
	SCTPPorts []int // support might need to be checked
}

func (ports *PolicyPorts) addPort(p *networkingv1.NetworkPolicyPort) {
	switch *p.Protocol {
	case corev1.ProtocolUDP:
		ports.UDPPorts = append(ports.UDPPorts, p.Port.IntValue())
	case corev1.ProtocolSCTP:
		ports.SCTPPorts = append(ports.SCTPPorts, p.Port.IntValue())
	default:
		ports.TCPPorts = append(ports.TCPPorts, p.Port.IntValue())
	}
}

func getNetworkPorts(ports []networkingv1.NetworkPolicyPort) PolicyPorts{
	var policyPorts PolicyPorts
	for _, port := range(ports) {
		if port.Port.Type == intstr.Int {
			policyPorts.addPort(&port)
		} else {
			continue // not sure how to handle named ports
		}
	}
	return policyPorts
}

func protocolToString(protocol corev1.Protocol) string {
	switch protocol {
	case corev1.ProtocolUDP:
		return "udp"
	case corev1.ProtocolSCTP:
		return "sctp"
	default:
		return "tcp"
	}
}

func getPortsMatch(ports []int, protocol, direction string) string {
	var match string
	for i, port := range(ports) {
		if i == 0 {
			match = fmt.Sprintf("%s.%s == %d",protocol, direction, port)
		} else {
			match = concatenate(match, fmt.Sprintf("%s.%s == %d", protocol, direction, port), "||")
		}
	}
	return match
}

func (ports *PolicyPorts) createPortsMatch(direction string) string {
	rules := []string{}

	tcp := getPortsMatch(ports.TCPPorts, "tcp", direction)
	if tcp != "" {
		tcp = addBraces(tcp)
		rules = append(rules, tcp)
	}

	udp := getPortsMatch(ports.UDPPorts, "udp", direction)
	if udp != "" {
		udp = addBraces(udp)
		rules = append(rules, udp)
	}

	sctp := getPortsMatch(ports.SCTPPorts, "sctp", direction)
	if sctp != "" {
		sctp = addBraces(udp)
		rules = append(rules, sctp)
	}

	match := ""
	for i, rule := range rules {
		if i != 0 {
			match += " || "
		}
		match += rule
	}

	return addBraces(match)
}

func createIPBlockMatch(block *networkingv1.IPBlock, direction string) string {
	tag := getIPVersion(block.CIDR) + "." + direction
	match := concatenate(tag, block.CIDR, "==")
	for _, exception := range(block.Except) {
		match = concatenate(match, concatenate(tag, exception, "!="), "&&")
	}
	return addBraces(match)
}

func createPeerMatch(mgr *manager.Manager, peer *networkingv1.NetworkPolicyPeer, policyNamespace string, direction ovn.PolicyDirection) (string, error) {
	peerIPAddress, err := getPeerIPs(mgr, peer, policyNamespace)
	if err != nil {
		return "", err
	}

	return ipSliceToACL(peerIPAddress, direction, "==", "||"), nil
}

func getPeerIPs(mgr *manager.Manager, peer *networkingv1.NetworkPolicyPeer, policyNamespace string) ([]string, error) {
	c := (*mgr).GetClient()

	namespaceOptions := &client.ListOptions{}
	if peer.NamespaceSelector != nil {	
		namespaceOptions.LabelSelector = labels.SelectorFromSet(peer.NamespaceSelector.MatchLabels)
	}
	podOptions := &client.ListOptions{}
	if peer.PodSelector != nil {
		podOptions.LabelSelector = labels.SelectorFromSet(peer.PodSelector.MatchLabels)
	}

	podList := &corev1.PodList{}

	err := c.List(context.TODO(), podList, namespaceOptions, podOptions)
	if err != nil {
		log.Error(err, "Failed to list pods")
		return nil, err
	}

	ipAddresses := getIPs(podList)

	return ipAddresses, nil
}

func ipSliceToACL(data []string, direction ovn.PolicyDirection, comparator, operand string) string {
	if len(data) == 0 {
		return ""
	}

	dir := ".dst"
	if direction == ovn.Ingress {
		dir = ".src"
	} 
	
	keyword := getIPVersion(data[0]) + dir
	acl := concatenate(keyword, data[0], comparator)
	for i := 1; i < len(data); i++ {
		keyword = getIPVersion(data[i]) + dir
		tmp := concatenate(keyword, data[i], comparator)
		acl = concatenate(acl, tmp, operand)
	}

	return acl
}

func addACLs(mgr *manager.Manager, policy *networkingv1.NetworkPolicy, xgressRules []XgressRule) error {
	var ports PolicyPorts
	var ipBlockMatch string
	var portsMatch string
	var peerIPAddressMatch string
	var aclRule string

	// create ACL template
	rule := ovn.ACL{
		Entity: getPortGroupName(policy),
		Priority: 1000,
		Verdict: "allow",
		Match: "",
	}

	var ovnRules []ovn.ACL
	var err error

	for _, xgressRule := range(xgressRules) {
		rule.Direction = xgressRule.Type

		// select if it's inport or outport
		matchPort := "inport"
		if rule.Direction == ovn.Ingress {
			matchPort = "outport"
		}

		// process policy ports
		if len(xgressRule.Ports) > 0 {
			ports = getNetworkPorts(xgressRule.Ports)
			portsMatchDst := ports.createPortsMatch("dst")
			portsMatchSrc := ports.createPortsMatch("src")
			portsMatch = addBraces(concatenate(portsMatchDst, portsMatchSrc, "||"))
		}

		// process policy peer rules
		if len(xgressRule.Peer) > 0 {
			for _, peer := range(xgressRule.Peer) {
				ipBlockMatch = ""
				peerIPAddressMatch = ""

				// add IPBlock rules
				if peer.IPBlock != nil {
					direction := "src"
					if xgressRule.Type == ovn.Egress {
						direction = "dst"
					}
					ipBlockMatch = createIPBlockMatch(peer.IPBlock, direction)
				}

				// add Namespace/Pod selectors rules
				if peer.NamespaceSelector != nil || peer.PodSelector != nil {
					peerIPAddressMatch, err = createPeerMatch(mgr, &peer, policy.Namespace, xgressRule.Type)
					if err != nil {
						log.Error(err, "Error creating peer matches")
						return err
					}
				}

				// join peer and selector rules
				var tmpMatch string
				if peerIPAddressMatch != "" && ipBlockMatch != "" {
					tmpMatch = concatenate(addBraces(peerIPAddressMatch), ipBlockMatch, "||")
				} else if peerIPAddressMatch != "" {
					tmpMatch = peerIPAddressMatch
				} else if ipBlockMatch != "" {
					tmpMatch = ipBlockMatch
				}
				
				// add port rules to the ACL
				if tmpMatch != "" && portsMatch != "" {
					aclRule = portsMatch + " && " + addBraces(tmpMatch)
				} else if tmpMatch != "" {
					aclRule = addBraces(tmpMatch)
				}

				// add inport/outport to the rule
				port := concatenate(matchPort, "@" + rule.Entity, "==")
				rule.Match = concatenate(port, aclRule, "&&")

				// add rule to the slice to process with ovn-nbctl later
				if rule.Match != "" {
					ovnRules = append(ovnRules, rule)
				}
			}
		} else {
			port := concatenate(matchPort, "@" + rule.Entity, "==")
			aclRule := ""
			if portsMatch != "" {
				// if there was no peer rules but the rules for ports have been provided
				// all traffic on those ports should be allowed
				aclRule = concatenate(port, portsMatch, "&&")
			} else {
				// if no rules were provided in the network policy at all
				// then it's an allow-all policy
				aclRule = concatenate(port, "(tcp || udp || icmp || sctp)", "&&")
				rule.Priority = 2000
			}

			rule.Match = aclRule

			// add rule to the slice to process with ovn-nbctl later
			ovnRules = append(ovnRules, rule)
		}

		// apply the created ACLs
		for _, rule := range(ovnRules) {
			_, err = ovn.ACLAdd(rule, "", "", "", "", true, false)
			if err != nil {
				log.Error(err, "Error while adding ACLs")
				return err
			}
		}
	}

	return nil
}

func getPortGroupName(policy *networkingv1.NetworkPolicy) string {
	// it turend out that port group can't contain some characters, e.g. "-"
	// so we need to hash the name so it can be represented as a numerical string
	// however, port group's name can not start with a number, hence we added the "pg"
	return "pg" + ovn.Hash(policy.Namespace + "_" + policy.Name)
}

func processPolicyRules(mgr *manager.Manager, policy *networkingv1.NetworkPolicy) error {
	// as Ingress/Egress policies are the same except for the name of one field (To/From)
	// we translate thos to common 'interface' XgressRule so we can process those easily later 
	// using the same function
	ingress := fromIngress(policy.Spec.Ingress)
	egress := fromEgress(policy.Spec.Egress)

	// add ingress ACLs
	if err := addACLs(mgr, policy, ingress); err != nil {
		return err
	}

	// add egress ACLs
	if err := addACLs(mgr, policy, egress); err != nil {
		return err
	}

	return nil
}

// createPolicy - creates policy with ACLs
func createPolicy(mgr *manager.Manager, policy *networkingv1.NetworkPolicy) bool {
	isIngressPolicy := false
	isEgressPolicy := false

	// check if policy is ingress, egress or both
	for _, t := range(policy.Spec.PolicyTypes) {
		if t == networkingv1.PolicyTypeIngress {
			isIngressPolicy = true
		} else if t == networkingv1.PolicyTypeEgress {
			isEgressPolicy = true
		}
	}

	// if no type is specified in yaml, then it's iingress only policy
	if !isIngressPolicy && !isEgressPolicy {
		isIngressPolicy = true
	}

	// list pods that should be affected by policy
	list, err := listPods(mgr, policy, policy.Namespace)
	if err != nil {
		log.Error(err, "Error occurred while listing pods")
		return false
	}

	// find OVS ports for the pods
	ports := getPorts(list)

	// get the hash of the port name
	pgName := getPortGroupName(policy)

	// add port with drop rules to filter all the traffic but allowed by the policy
	if err = ovn.AddDenyPG(pgName, isIngressPolicy, isEgressPolicy); err != nil {
		return false
	}

	// add ports to the port group
	if len(ports) > 0 {
		stdout, err := ovn.PGSetPorts(pgName, ports)
		if err != nil {
			log.Error(err, stdout)
			return false
		}
	}

	// translate the policy into ACLs and apply
	if err = processPolicyRules(mgr, policy); err != nil {
		return false
	}

	return true
}

// deletePolicy - deletes selected policy
func deletePolicy(mgr *manager.Manager,policy *networkingv1.NetworkPolicy) bool {
	_, err := ovn.PGDel(getPortGroupName(policy))
	if err != nil {
		log.Error(err, "Error while deleting policy")
		return false
	}

	return true
}

// updatePolicy - updates selected policy
func updatePolicy(mgr *manager.Manager, oldPolicy, newPolicy *networkingv1.NetworkPolicy) bool {
	logPolicyInfo("Updating policy", oldPolicy)

	status := deletePolicy(mgr, oldPolicy)
	if !status {
		return false
	}
	status = createPolicy(mgr, newPolicy)
	return status
}

// ListNetworkPolicies - gets all network policies
func ListNetworkPolicies(mgr *manager.Manager) (*networkingv1.NetworkPolicyList, error) {
	c := (*mgr).GetClient()

	options := &client.ListOptions{}

	npList := &networkingv1.NetworkPolicyList{}

	err := c.List(context.TODO(), npList, options)
	if err != nil {
		return nil, err
	}

	return npList, nil
}

// RefreshNetworkPolicies updates all currently deployed netwokr policies
func RefreshNetworkPolicies(mgr *manager.Manager) error {
	l, err := ListNetworkPolicies(mgr)
	if err != nil {
		return err
	}
	for _, np := range l.Items {
		if policyUpdated := updatePolicy(mgr, &np, &np); !policyUpdated {
			return fmt.Errorf("Unable to update policy: %v", np)
		}
		
	}
	return nil
}

func concatenate(A, B, operand string) string {
	return A + " " + operand + " " + B
}

func addBraces(A string) string {
	return "(" + A + ")"
}

func logPolicyInfo(msg string, policy *networkingv1.NetworkPolicy) {
	log.V(1).Info(msg + ": " + policy.Namespace + "_" + policy.Name + " (" + getPortGroupName(policy) + ")")
}

func getIPVersion(ip string) string {
	if strings.Contains(ip,ipv4Delimeter) {
		return "ip4"
	}
	return "ip6"
}