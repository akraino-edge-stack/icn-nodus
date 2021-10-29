package networkpolicy

import (
	"context"
	"fmt"

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
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
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
			log.V(1).Info("Network policy - UPDATE\n")

			log.V(1).Info("Old policy:")

			ok := printPolicyObject(e.ObjectOld)

			if !ok {
				return false
			}

			log.V(1).Info("New policy:")
			return printPolicyObject(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log.V(1).Info("Network policy - CREATE\n")

			printPolicyObject(e.Object)

			policy, ok := e.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			isIngressPolicy := false
			isEgressPolicy := false

			for _, t := range(policy.Spec.PolicyTypes) {
				if t == networkingv1.PolicyTypeIngress {
					log.V(1).Info("isIngressPolicy = true")
					isIngressPolicy = true
				} else if t == networkingv1.PolicyTypeEgress {
					log.V(1).Info("isEgressPolicy = true")
					isEgressPolicy = true
				}
			}

			if !isIngressPolicy && !isEgressPolicy {
				isIngressPolicy = true
			}

			list, err := listPods(&mgr, policy, policy.Namespace)
			if err != nil {
				log.V(1).Info("listPods error")
				log.V(1).Info(err.Error())
				return false
			}

			ports := getPorts(list)

			for _, port := range(ports) {
				log.V(1).Info(port)
				log.V(1).Info("\n")
			}

			pgName := policy.Namespace + "_" + policy.Name

			if err = ovn.AddDenyPG(pgName, isIngressPolicy, isEgressPolicy); err != nil {
				return false
			}

			stdout, err := ovn.PGSetPorts(pgName + "_Deny", ports)
			if err != nil {
				log.V(1).Info("PGSetPorts")
				log.V(1).Info(stdout)
				log.V(1).Info(err.Error())
				return false
			}

			ovn.PGAddWithPorts(pgName, ports)

			var ingressPorts PolicyPorts
			var ipBlockMatch string
			var ingressPortsMatch string
			var ingressPortsMatchDst string
			var ingressPortsMatchSrc string
			var peerIPAddressMatch string
			var aclRule string

			var rule ovn.ACL
			rule.Entity = pgName
			rule.Priority = 1000
			rule.Verdict = "allow"

			rule.Direction = ovn.Ingress
			rule.Match = ""

			var ingressACL []string

			for _, ingressRule := range(policy.Spec.Ingress) {
				log.V(1).Info("THERE IS A RULE")
				if len(ingressRule.Ports) > 0 {
					ingressPorts = getNetworkPorts(ingressRule.Ports)
					ingressPortsMatchDst = ingressPorts.createPortsMatch("dst")
					ingressPortsMatchSrc = ingressPorts.createPortsMatch("src")
					ingressPortsMatch = addBraces(concatenate(ingressPortsMatchDst, ingressPortsMatchSrc, "||"))
				}
				if len(ingressRule.From) > 0 {
					for _, peer := range(ingressRule.From) {
						ipBlockMatch = ""
						peerIPAddressMatch = ""
						log.V(1).Info("FROM RULE")
						if peer.IPBlock != nil {
							ipBlockMatch = createIPBlockMatch(peer.IPBlock, "ip4", "src")
						}
						if peer.NamespaceSelector != nil || peer.PodSelector != nil {
							peerIPAddressMatch, err = createPeerMatch(&mgr, &peer, policy.Namespace, ovn.Ingress)
							if err != nil {
								log.V(1).Info(err.Error())
								return false
							}
						}

						log.V(1).Info("peerIPAddressMatch")
						log.V(1).Info(peerIPAddressMatch)

						log.V(1).Info("ipBlockMatch")
						log.V(1).Info(ipBlockMatch)

						var tmpMatch string
						if peerIPAddressMatch != "" && ipBlockMatch != "" {
							tmpMatch = concatenate(addBraces(peerIPAddressMatch), ipBlockMatch, "||")
						} else if peerIPAddressMatch != "" {
							tmpMatch = peerIPAddressMatch
						} else if ipBlockMatch != "" {
							tmpMatch = ipBlockMatch
						}
						
						if tmpMatch != "" && ingressPortsMatch != "" {
							aclRule = ingressPortsMatch + " && " + addBraces(tmpMatch)
						} else if tmpMatch != "" {
							aclRule = addBraces(tmpMatch)
						}

						ingressACL = append(ingressACL, aclRule)

						log.V(1).Info("Created rule:")
						log.V(1).Info(aclRule)
					}
				} else {
					aclRule := "(tcp || udp || icmp)"
					if ingressPortsMatch != "" {
						aclRule = ingressPortsMatch
					}
					ingressACL = append(ingressACL, aclRule)
				}

				for _, acl := range(ingressACL) {
					rule.Priority = 1000
					if acl == "(tcp || udp || icmp)" {
						rule.Priority = 2000
					}
					rule.Match = acl
					_, err = ovn.ACLAdd(rule, "", "", "", "", true, false)
					if err != nil {
						log.V(1).Info(err.Error())
						return false
					}
				}
			}

			var egressPorts PolicyPorts
			var egressPortsMatch string
			var egressPortsMatchDst string
			var egressPortsMatchSrc string

			rule.Direction = ovn.Egress
			rule.Match = ""
			aclRule = ""

			var egressACL []string

			if isEgressPolicy {
				for _, egressRule := range(policy.Spec.Egress) {
					if len(egressRule.Ports) > 0 {
						egressPorts = getNetworkPorts(egressRule.Ports)
						egressPortsMatchDst = egressPorts.createPortsMatch("dst")
						egressPortsMatchSrc = egressPorts.createPortsMatch("src")
						egressPortsMatch = addBraces(concatenate(egressPortsMatchDst, egressPortsMatchSrc, "||"))
					}
					if len(egressRule.To) > 0 {
						for _, peer := range(egressRule.To) {
							ipBlockMatch = ""
							peerIPAddressMatch = ""
							log.V(1).Info("TO RULE")
							if peer.IPBlock != nil {
								ipBlockMatch = createIPBlockMatch(peer.IPBlock, "ip4", "dst")
							}
							if peer.NamespaceSelector != nil || peer.PodSelector != nil {
								peerIPAddressMatch, err = createPeerMatch(&mgr, &peer, policy.Namespace, ovn.Egress)
								if err != nil {
									log.V(1).Info(err.Error())
									return false
								}
							}
							

							var tmpMatch string
							if peerIPAddressMatch != "" && ipBlockMatch != "" {
								tmpMatch = concatenate(addBraces(peerIPAddressMatch), ipBlockMatch, "||")
							} else if peerIPAddressMatch != "" {
								tmpMatch = peerIPAddressMatch
							} else if ipBlockMatch != "" {
								tmpMatch = ipBlockMatch
							}
							
							if tmpMatch != "" && egressPortsMatch != "" {
								aclRule = ingressPortsMatch + " && " + addBraces(tmpMatch)
							} else if tmpMatch != "" {
								aclRule = addBraces(tmpMatch)
							}

							egressACL = append(egressACL, aclRule)

							log.V(1).Info("Created rule:")
							log.V(1).Info(aclRule)

							_, err = ovn.ACLAdd(rule, "", "", "", "", true, false)
							if err != nil {
								log.V(1).Info(err.Error())
								return false
							}
						}
					} else {
						aclRule := "(tcp || udp || icmp)"
						if ingressPortsMatch != "" {
							aclRule = ingressPortsMatch
						}
						egressACL = append(egressACL, aclRule)
					}
					for _, acl := range(egressACL) {
						rule.Priority = 1000
						if acl == "(tcp || udp || icmp)" {
							rule.Priority = 2000
						}
						rule.Match = acl
						_, err = ovn.ACLAdd(rule, "", "", "", "", true, false)
						if err != nil {
							log.V(1).Info(err.Error())
							return false
						}
					}
				}
			}

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.V(1).Info("Network policy - DELETE\n")

			printPolicyObject(e.Object)

			policy, ok := e.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			pgName := policy.Namespace + "_" + policy.Name

			_, err := ovn.PGDel(pgName)
			if err != nil {
				log.V(1).Info(err.Error())
				return false
			}

			_, err = ovn.PGDel(pgName + "_Deny")
			if err != nil {
				log.V(1).Info(err.Error())
				return false
			}

			return true
		},
	}

	// if err = ovn.AddDefaultPG(ovn.Ingress); err != nil {
	// 	return err
	// }

	// if err = ovn.AddDefaultPG(ovn.Egress); err != nil {
	// 	return err
	// }

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
func (r *ReconcileNetworkPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Enter Reconciling Network Policy")

	// Fetch the Pod instance
	instance := &networkingv1.NetworkPolicy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

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


func printPolicyObject(obj runtime.Object) bool {
			policy, ok := obj.(*networkingv1.NetworkPolicy)
			if !ok {
				return false
			}

			log.V(1).Info("Name:")
			log.V(1).Info(policy.Name)
			log.V(1).Info("Namespace:")
			log.V(1).Info(policy.Namespace)
			log.V(1).Info("Spec:")
			log.V(1).Info(policy.Spec.String())
		
			return true
}

func listPods(mgr *manager.Manager, policy *networkingv1.NetworkPolicy, namespace string) (*corev1.PodList, error) {
	c := (*mgr).GetClient()

	options := &client.ListOptions{}
	options.Namespace = policy.Namespace
	options.LabelSelector = labels.SelectorFromSet(policy.Spec.PodSelector.MatchLabels)

	podList := &corev1.PodList{}

	err := c.List(context.TODO(), podList, options)
	if err != nil {
		log.V(1).Info("listing error")
		log.V(1).Info(err.Error())
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

func concatenate(A, B, operand string) string {
	return A + " " + operand + " " + B
}

func addBraces(A string) string {
	return "(" + A + ")"
}

// PolicyPorts describes internal port definition
type PolicyPorts struct {
	TCPPorts []int
	UDPPorts []int
	SCTPPorts []int //support needs to be checked according to: https://github.com/ovn-org/ovn-kubernetes/issues/1120
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
			continue //for now I do not know how to handle named ports
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

// func (p *Port) getPortACL() string {
// 	return concatenate(p.port, p.protocol, "&&")
// }

// func createPortMatch(ports []Port) string {
// 	var match string
// 	for i, port := range(ports) {
// 		if i == 0 {
// 			match = port.getPortACL()
// 		} else {
// 			match = concatenate(match, port.getPortACL(), "||")
// 		}
// 	}
// 	return match
// }

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
	tcp := getPortsMatch(ports.TCPPorts, "tcp", direction)
	if tcp != "" {
		tcp = addBraces(tcp)
	}

	udp := getPortsMatch(ports.UDPPorts, "udp", direction)
	if udp != "" {
		udp = addBraces(udp)
	}

	var match string
	if tcp != "" && udp != "" {
		match = concatenate(tcp, udp, "||")
	} else if tcp != "" {
		match = tcp
	} else if udp != ""{
		match = udp
	} else {
		return ""
	}

	return addBraces(match)
}

func createIPBlockMatch(block *networkingv1.IPBlock, ipversion, direction string) string {
	tag := ipversion + "." + direction
	match := concatenate(tag, block.CIDR, "==")
	for _, exception := range(block.Except) {
		match = concatenate(match, concatenate(tag, exception, "!="), "&&")
	}
	return addBraces(match)
}

func createPeerMatch(mgr *manager.Manager, peer *networkingv1.NetworkPolicyPeer, policyNamespace string, direction ovn.PolicyDirection) (string, error) {
	log.V(1).Info("createPeerMatch")
	peerIPAddress, err := getPeerIPs(mgr, peer, policyNamespace)
	if err != nil {
		return "", err
	}
	var keyword string

	if direction == ovn.Ingress {
		keyword = "ip4.src"
	} else {
		keyword = "ip4.dst"
	}

	return sliceToACL(peerIPAddress, keyword, "==", "||"), nil
}

func getPeerIPs(mgr *manager.Manager, peer *networkingv1.NetworkPolicyPeer, policyNamespace string) ([]string, error) {
	c := (*mgr).GetClient()

	namespaceOptions := &client.ListOptions{}
	if peer.NamespaceSelector != nil {
		log.V(1).Info("namespaceOptions.LabelSelector created")
		namespaceOptions.LabelSelector = labels.SelectorFromSet(peer.NamespaceSelector.MatchLabels)
	}
	podOptions := &client.ListOptions{}
	if peer.PodSelector != nil {
		log.V(1).Info("podOptions.LabelSelector created")
		podOptions.LabelSelector = labels.SelectorFromSet(peer.PodSelector.MatchLabels)
	}

	podList := &corev1.PodList{}

	err := c.List(context.TODO(), podList, namespaceOptions, podOptions)
	if err != nil {
		log.V(1).Info("getPeerIPs")
		log.V(1).Info(err.Error())
		return nil, err
	}
	log.V(1).Info("Num of pods:")
	log.V(1).Info(fmt.Sprintf("%d", len(podList.Items)))
	ipAddresses := getIPs(podList)

	return ipAddresses, nil
}

func sliceToACL(data []string, keyword, comparator, operand string) string {
	if len(data) == 0 {
		return ""
	}
	acl := concatenate(keyword, data[0], comparator)
	for i := 1; i < len(data); i++ {
		tmp := concatenate(keyword, data[i], comparator)
		acl = concatenate(acl, tmp, operand)
	}
	return acl
}