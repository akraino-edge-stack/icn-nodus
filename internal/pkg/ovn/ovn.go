package ovn

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	k8sv1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/config"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/network"

	"github.com/mitchellh/mapstructure"
	kapi "k8s.io/api/core/v1"
	kexec "k8s.io/utils/exec"
)

type Controller struct {
	gatewayCache map[string]string
}

type OVNNetworkConf struct {
	Subnetv4    string
	GatewayIPv4 string
	ExcludeIPs  string
	Subnetv6    string
	GatewayIPv6 string
}

const (
	ovn4nfvRouterName = "ovn4nfv-master"
	// Ovn4nfvAnnotationTag tag on already processed Pods
	Ovn4nfvAnnotationTag = "k8s.plugin.opnfv.org/ovnInterfaces"
	// OVN Default Network name
	Ovn4nfvDefaultNw           = "ovn4nfvk8s-default-nw"
	SFCnetworkIntefacePrefixes = "sn"
)

var ovnConf *OVNNetworkConf

//GetOvnNetConf return error
func GetOvnNetConf() error {
	ovnConf = &OVNNetworkConf{}

	ovnConf.Subnetv4 = os.Getenv("OVN_SUBNET")
	if ovnConf.Subnetv4 == "" {
		return fmt.Errorf("OVN subnet is not set in nfn-operator configmap env")
	}

	ovnConf.GatewayIPv4 = os.Getenv("OVN_GATEWAYIP")
	if ovnConf.GatewayIPv4 == "" {
		log.Info("No Gateway IP address provided - 1st IP address of the subnet range will be used as Gateway", "Subnet", ovnConf.Subnetv4)
	}

	ovnConf.ExcludeIPs = os.Getenv("OVN_EXCLUDEIPS")
	if ovnConf.ExcludeIPs == "" {
		log.Info("No IP addresses are excluded in the subnet range", "Subnet", ovnConf.Subnetv4)
	}

	ovnConf.Subnetv6 = os.Getenv("OVN_SUBNET_V6")
	if ovnConf.Subnetv6 == "" {
		log.Info("OVN subnet IPv6 is not set in nfn-operator configmap env")
	}

	ovnConf.GatewayIPv6 = os.Getenv("OVN_GATEWAYIP_V6")
	if ovnConf.GatewayIPv6 == "" {
		log.Info("No Gateway IPv6 address provided - 1st IP address of the subnet range will be used as Gateway", "Subnet", ovnConf.Subnetv6)
	}

	return nil
}

type NetInterface struct {
	Name           string
	Interface      string
	NetType        string
	DefaultGateway string
	IPAddress      string
	MacAddress     string
	GWIPaddress    string
}

var ovnCtl *Controller

// NewOvnController creates a new OVN controller for creating logical networks
func NewOvnController(exec kexec.Interface) (*Controller, error) {

	if exec == nil {
		exec = kexec.New()
	}
	if err := SetExec(exec); err != nil {
		log.Error(err, "Failed to initialize exec helper")
		return nil, err
	}

	if err := GetOvnNetConf(); err != nil {
		log.Error(err, "nfn-operator OVN Network configmap is not set")
		return nil, err
	}
	if err := SetupOvnUtils(); err != nil {
		log.Error(err, "Failed to initialize OVN State")
		return nil, err
	}

	ovnCtl = &Controller{
		gatewayCache: make(map[string]string),
	}
	return ovnCtl, nil
}

// GetOvnController returns OVN controller for creating logical networks
func GetOvnController() (*Controller, error) {
	if ovnCtl != nil {
		return ovnCtl, nil
	}
	return nil, fmt.Errorf("OVN Controller not initialized")
}

func (oc *Controller) AddNodeLogicalPorts(node string) (ipAddr, macAddr string, err error) {
	nodeName := strings.ToLower(node)
	portName := config.GetNodeIntfName(nodeName)

	log.V(1).Info("Creating Node logical port", "node", nodeName, "portName", portName)

	ipAddr, macmacAddr, err := oc.addNodeLogicalPortWithSwitch(Ovn4nfvDefaultNw, portName)
	if err != nil {
		return "", "", err
	}

	return ipAddr, macmacAddr, nil
}

// AddLogicalPorts adds ports to the Pod
func (oc *Controller) AddLogicalPorts(pod *kapi.Pod, ovnNetObjs []map[string]interface{}, IsExtraInterfaces bool) (key, value string) {

	if pod.Spec.HostNetwork {
		return
	}

	if !IsExtraInterfaces {
		if _, ok := pod.Annotations[Ovn4nfvAnnotationTag]; ok {
			log.V(1).Info("AddLogicalPorts : Pod annotation found")
			return
		}
	}

	var ovnString, outStr string
	var defaultInterface bool

	ovnString = "["
	var ns NetInterface
	for _, net := range ovnNetObjs {
		err := mapstructure.Decode(net, &ns)
		if err != nil {
			log.Error(err, "mapstruct error", "network", net)
			return
		}

		if !oc.FindLogicalSwitch(ns.Name) && IsExtraInterfaces == false {
			log.Info("Logical Switch not found, create the network")
			err := network.CreateNetworkFromPool(ns.Name)
			if err != nil {
				log.Error(err, "Error in creating networkpool or network")
				return
			}
		}

		if !oc.FindLogicalSwitch(ns.Name) {
			log.Info("Logical Switch not found")
			return
		}
		if ns.Name == Ovn4nfvDefaultNw {
			defaultInterface = true
		}
		if ns.Interface == "" && ns.Name != Ovn4nfvDefaultNw {
			log.Info("Interface name must be provided")
			return
		}
		if ns.DefaultGateway == "" {
			ns.DefaultGateway = "false"
		}
		var portName string
		if ns.Interface != "" {
			portName = fmt.Sprintf("%s_%s_%s", pod.Namespace, pod.Name, ns.Interface)
		} else {
			portName = fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
			ns.Interface = "*"
		}
		outStr = oc.addLogicalPortWithSwitch(pod, ns.Name, ns.IPAddress, ns.MacAddress, ns.GWIPaddress, portName)
		if outStr == "" {
			return
		}
		last := len(outStr) - 1
		tmpString := outStr[:last]
		tmpString += "," + "\\\"defaultGateway\\\":" + "\\\"" + ns.DefaultGateway + "\\\""
		tmpString += "," + "\\\"interface\\\":" + "\\\"" + ns.Interface + "\\\"}"
		ovnString += tmpString
		ovnString += ","
	}
	var last int
	if defaultInterface == false && !IsExtraInterfaces {
		// Add Default interface
		portName := fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
		outStr = oc.addLogicalPortWithSwitch(pod, Ovn4nfvDefaultNw, "", "", "", portName)
		if outStr == "" {
			return
		}
		last := len(outStr) - 1
		tmpString := outStr[:last]
		tmpString += "," + "\\\"interface\\\":" + "\\\"" + "*" + "\\\"}"
		ovnString += tmpString
		ovnString += ","
	}
	last = len(ovnString) - 1
	ovnString = ovnString[:last]
	ovnString += "]"
	key = Ovn4nfvAnnotationTag
	value = ovnString
	return key, value
}

// DeleteLogicalPorts deletes the OVN ports for the pod
func (oc *Controller) DeleteLogicalPorts(name, namespace string) {

	log.Info("DeleteLogicalPorts")
	logicalPort := fmt.Sprintf("%s_%s", namespace, name)

	// get the list of logical ports from OVN
	stdout, stderr, err := RunOVNNbctl("--data=bare", "--no-heading",
		"--columns=name", "find", "logical_switch_port", "external_ids:pod=true")
	if err != nil {
		log.Error(err, "Error in obtaining list of logical ports ", "stdout", stdout, "stderr", stderr)
		return
	}
	existingLogicalPorts := strings.Fields(stdout)
	for _, existingPort := range existingLogicalPorts {
		if strings.Contains(existingPort, logicalPort) {
			// found, delete this logical port
			log.Info("Deleting", "Port", existingPort)
			stdout, stderr, err := RunOVNNbctl("--if-exists", "lsp-del",
				existingPort)
			if err != nil {
				log.Error(err, "Error in deleting pod's logical port ", "stdout", stdout, "stderr", stderr)
			}
		}
	}
	return
}

// CreateNetwork in OVN controller
func (oc *Controller) CreateNetwork(cr *k8sv1alpha1.Network) error {
	// Currently only these fields are supported
	name := cr.Name

	if len(cr.Spec.Ipv4Subnets) > 0 {
		subnet := cr.Spec.Ipv4Subnets[0].Subnet
		gatewayIP := cr.Spec.Ipv4Subnets[0].Gateway
		excludeIps := cr.Spec.Ipv4Subnets[0].ExcludeIps

		logicalSwitchName := getIPv4LogicalSwitchName(name)
		logicalRouterPortName := getIPv4LogicalRouterPortName(name)

		err := createNetwork(logicalSwitchName, subnet, gatewayIP, excludeIps, logicalRouterPortName)
		if err != nil {
			return err
		}
	}

	if len(cr.Spec.Ipv6Subnets) > 0 {
		subnet := cr.Spec.Ipv6Subnets[0].Subnet
		gatewayIP := cr.Spec.Ipv6Subnets[0].Gateway
		excludeIps := cr.Spec.Ipv6Subnets[0].ExcludeIps

		logicalSwitchName := getIPv6LogicalSwitchName(name)
		logicalRouterPortName := getIPv6LogicalRouterPortName(name)

		err := createNetwork(logicalSwitchName, subnet, gatewayIP, excludeIps, logicalRouterPortName)
		if err != nil {
			return err
		}
	}

	return nil
}

func createNetwork(name, subnet, gatewayIP, excludeIps, logicalRouterPortName string) error {
	var stdout, stderr string
	gatewayIPMask, _, err := createOvnLS(name, subnet, gatewayIP, excludeIps, "", "")
	if err != nil {
		return err
	}

	routerMac, stderr, err := RunOVNNbctl("--if-exist", "get", "logical_router_port", logicalRouterPortName, "mac")
	if err != nil {
		log.Error(err, "Failed to get logical router port", "stderr", stderr)
		return err
	}
	if routerMac == "" {
		prefix := "00:00:00"
		newRand := rand.New(rand.NewSource(time.Now().UnixNano()))
		routerMac = fmt.Sprintf("%s:%02x:%02x:%02x", prefix, newRand.Intn(255), newRand.Intn(255), newRand.Intn(255))
	}

	_, stderr, err = RunOVNNbctl("--wait=hv", "--may-exist", "lrp-add", ovn4nfvRouterName, logicalRouterPortName, routerMac, gatewayIPMask)
	if err != nil {
		log.Error(err, "Failed to add logical port to router", "stderr", stderr)
		return err
	}

	// Connect the switch to the router.
	stdout, stderr, err = RunOVNNbctl("--wait=hv", "--", "--may-exist", "lsp-add", name, "stor-"+name, "--", "set", "logical_switch_port", "stor-"+name, "type=router", "options:router-port="+logicalRouterPortName, "addresses="+"\""+routerMac+"\"")
	if err != nil {
		log.Error(err, "Failed to add logical port to switch", "stderr", stderr, "stdout", stdout)
		return err
	}

	return nil
}

// DeleteNetwork in OVN controller
func (oc *Controller) DeleteNetwork(cr *k8sv1alpha1.Network) error {

	name := cr.Name

	err := deleteLogicalRouterPort(getIPv4LogicalRouterPortName(name))
	if err != nil {
		return err
	}
	err = deleteLogicalRouterPort(getIPv6LogicalRouterPortName(name))
	if err != nil {
		return err
	}

	err = deleteLogicalSwitch(getIPv4LogicalSwitchName(name))
	if err != nil {
		return err
	}
	err = deleteLogicalSwitch(getIPv6LogicalSwitchName(name))
	if err != nil {
		return err
	}

	return nil
}

func deleteLogicalSwitch(name string) error {
	stdout, stderr, err := RunOVNNbctl("--if-exist", "--wait=hv", "ls-del", name)
	if err != nil {
		log.Error(err, "Failed to delete switch", "name", name, "stdout", stdout, "stderr", stderr)
		return err
	}
	return nil
}

func deleteLogicalRouterPort(name string) error {
	stdout, stderr, err := RunOVNNbctl("--if-exist", "--wait=hv", "lrp-del", name)
	if err != nil {
		log.Error(err, "Failed to delete router port", "name", name, "stdout", stdout, "stderr", stderr)
		return err
	}
	return nil
}

func getIPv4LogicalSwitchName(prefix string) string {
	return prefix
}

func getIPv6LogicalSwitchName(prefix string) string {
	return prefix + "v6"
}

func getIPv4LogicalRouterPortName(suffix string) string {
	return "rtos-" + suffix
}

func getIPv6LogicalRouterPortName(suffix string) string {
	return "rtosv6-" + suffix
}

// CreateProviderNetwork in OVN controller
func (oc *Controller) CreateProviderNetwork(cr *k8sv1alpha1.ProviderNetwork) error {
	// Currently only these fields are supported
	name := cr.Name

	if len(cr.Spec.Ipv4Subnets) > 0 {
		subnet := cr.Spec.Ipv4Subnets[0].Subnet
		gatewayIP := cr.Spec.Ipv4Subnets[0].Gateway
		excludeIps := cr.Spec.Ipv4Subnets[0].ExcludeIps

		logicalSwitchName := getIPv4LogicalSwitchName(name)

		err := createProviderNetwork(logicalSwitchName, subnet, gatewayIP, excludeIps)
		if err != nil {
			return err
		}
	}

	if len(cr.Spec.Ipv6Subnets) > 0 {
		subnet := cr.Spec.Ipv6Subnets[0].Subnet
		gatewayIP := cr.Spec.Ipv6Subnets[0].Gateway
		excludeIps := cr.Spec.Ipv6Subnets[0].ExcludeIps

		logicalSwitchName := getIPv6LogicalSwitchName(name)

		err := createProviderNetwork(logicalSwitchName, subnet, gatewayIP, excludeIps)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteProviderNetwork in OVN controller
func (oc *Controller) DeleteProviderNetwork(cr *k8sv1alpha1.ProviderNetwork) error {

	name := cr.Name

	err := deleteLogicalSwitch(getIPv4LogicalSwitchName(name))
	if err != nil {
		return err
	}
	err = deleteLogicalSwitch(getIPv6LogicalSwitchName(name))
	if err != nil {
		return err
	}

	return nil
}

func createProviderNetwork(name, subnet, gatewayIP, excludeIps string) error {
	var stdout, stderr string

	_, _, err := createOvnLS(name, subnet, gatewayIP, excludeIps, "", "")
	if err != nil {
		return err
	}

	// Add localnet port.
	stdout, stderr, err = RunOVNNbctl("--wait=hv", "--", "--may-exist", "lsp-add", name, "server-localnet_"+name, "--",
		"lsp-set-addresses", "server-localnet_"+name, "unknown", "--",
		"lsp-set-type", "server-localnet_"+name, "localnet", "--",
		"lsp-set-options", "server-localnet_"+name, "network_name=nw_"+name)
	if err != nil {
		log.Error(err, "Failed to add logical port to switch", "stderr", stderr, "stdout", stdout)
		return err
	}
	return nil
}

// FindLogicalSwitch returns true if switch exists
func (oc *Controller) FindLogicalSwitch(name string) bool {
	// get logical switch from OVN
	output, stderr, err := RunOVNNbctl("--data=bare", "--no-heading",
		"--columns=name", "find", "logical_switch", "name="+name)
	if err != nil {
		log.Error(err, "Error in obtaining list of logical switch", "stderr", stderr)
		return false
	}
	if strings.Compare(name, output) == 0 {
		return true
	}
	return false
}

func (oc *Controller) getGatewayFromSwitch(logicalSwitch string) (string, string, error) {
	var gatewayIPMaskStr, stderr string
	var ok bool
	var err error
	if gatewayIPMaskStr, ok = oc.gatewayCache[logicalSwitch]; !ok {
		gatewayIPMaskStr, stderr, err = RunOVNNbctl("--if-exists",
			"get", "logical_switch", logicalSwitch,
			"external_ids:gateway_ip")
		if err != nil {
			log.Error(err, "Failed to get gateway IP", "stderr", stderr, "gatewayIPMaskStr", gatewayIPMaskStr)
			return "", "", err
		}
		if gatewayIPMaskStr == "" {
			return "", "", fmt.Errorf("Empty gateway IP in logical switch %s",
				logicalSwitch)
		}
		oc.gatewayCache[logicalSwitch] = gatewayIPMaskStr
	}
	gatewayIPMask := strings.Split(gatewayIPMaskStr, "/")
	if len(gatewayIPMask) != 2 {
		return "", "", fmt.Errorf("Failed to get IP and Mask from gateway CIDR:  %s",
			gatewayIPMaskStr)
	}
	gatewayIP := gatewayIPMask[0]
	mask := gatewayIPMask[1]
	return gatewayIP, mask, nil
}

func (oc *Controller) addNodeLogicalPortWithSwitch(logicalSwitch, portName string) (ipAddr, macAddr string, r error) {
	var out, stderr string
	var err error

	log.V(1).Info("Creating Node logical port for on switch", "portName", portName, "logicalSwitch", logicalSwitch)

	out, stderr, err = RunOVNNbctl("--wait=sb", "--",
		"--may-exist", "lsp-add", logicalSwitch, portName,
		"--", "lsp-set-addresses",
		portName, "dynamic")
	if err != nil {
		log.Error(err, "Error while creating logical port %s ", "portName", portName, "stdout", out, "stderr", stderr)
		return "", "", err
	}

	count := 30
	for count > 0 {
		out, stderr, err = RunOVNNbctl("get",
			"logical_switch_port", portName, "dynamic_addresses")

		if err == nil && out != "[]" {
			break
		}
		if err != nil {
			log.Error(err, "Error while obtaining addresses for", "portName", portName)
			return "", "", err
		}
		time.Sleep(time.Second)
		count--
	}
	if count == 0 {
		log.Error(err, "Error while obtaining addresses for", "portName", portName, "stdout", out, "stderr", stderr)
		return "", "", err
	}

	// static addresses have format ["0a:00:00:00:00:01 192.168.1.3"], while
	// dynamic addresses have format "0a:00:00:00:00:01 192.168.1.3".
	outStr := strings.TrimLeft(out, `[`)
	outStr = strings.TrimRight(outStr, `]`)
	outStr = strings.Trim(outStr, `"`)
	addresses := strings.Split(outStr, " ")
	if len(addresses) != 2 {
		log.Info("Error while obtaining addresses for", "portName", portName)
		return "", "", err
	}

	_, mask, err := oc.getGatewayFromSwitch(logicalSwitch)
	if err != nil {
		log.Error(err, "Error obtaining gateway address for switch", "logicalSwitch", logicalSwitch)
		return "", "", err
	}

	ipAddr = fmt.Sprintf("%s/%s", addresses[1], mask)
	macAddr = fmt.Sprintf("%s", addresses[0])

	return ipAddr, macAddr, nil
}

func (oc *Controller) getNodeLogicalPortIPAddr(pod *kapi.Pod) (ipAddress string, r error) {
	var out, stderr, nodeName, portName string
	var err error

	nodeName = strings.ToLower(pod.Spec.NodeName)
	portName = config.GetNodeIntfName(nodeName)

	log.V(1).Info("Get Node logical port", "pod", pod.GetName(), "node", nodeName, "portName", portName)

	count := 30
	for count > 0 {
		out, stderr, err = RunOVNNbctl("get",
			"logical_switch_port", portName, "dynamic_addresses")

		if err == nil && out != "[]" {
			break
		}
		if err != nil {
			log.Error(err, "Error while obtaining addresses for", "portName", portName)
			return "", err
		}
		time.Sleep(time.Second)
		count--
	}
	if count == 0 {
		log.Error(err, "Error while obtaining addresses for", "portName", portName, "stdout", out, "stderr", stderr)
		return "", err
	}

	// static addresses have format ["0a:00:00:00:00:01 192.168.1.3"], while
	// dynamic addresses have format "0a:00:00:00:00:01 192.168.1.3".
	outStr := strings.TrimLeft(out, `[`)
	outStr = strings.TrimRight(outStr, `]`)
	outStr = strings.Trim(outStr, `"`)
	addresses := strings.Split(outStr, " ")
	if len(addresses) != 2 {
		log.Info("Error while obtaining addresses for", "portName", portName)
		return "", err
	}

	ipAddr := fmt.Sprintf("%s", addresses[1])
	log.V(1).Info("Get Node logical port", "pod", pod.GetName(), "node", nodeName, "portName", portName, "Node port IP", ipAddr)

	return ipAddr, nil
}

func (oc *Controller) addLogicalPortWithSwitch(pod *kapi.Pod, logicalSwitch, ipAddress, macAddress, gwipAddress, portName string) (annotation string) {
	var out, stderr string
	var err error
	var isStaticIP bool
	if pod.Spec.HostNetwork {
		return
	}

	log.V(1).Info("Creating logical port for on switch", "portName", portName, "logicalSwitch", logicalSwitch)

	if ipAddress != "" && macAddress != "" {
		isStaticIP = true
	}
	if ipAddress != "" && macAddress == "" {
		macAddress = generateMac()
		isStaticIP = true
	}

	if isStaticIP {
		out, stderr, err = RunOVNNbctl("--may-exist", "lsp-add",
			logicalSwitch, portName, "--", "lsp-set-addresses", portName,
			fmt.Sprintf("%s %s", macAddress, ipAddress), "--", "--if-exists",
			"clear", "logical_switch_port", portName, "dynamic_addresses", "--", "set",
			"logical_switch_port", portName,
			"external-ids:namespace="+pod.Namespace,
			"external-ids:logical_switch="+logicalSwitch,
			"external-ids:pod=true")
		if err != nil {
			log.Error(err, "Failed to add logical port to switch", "out", out, "stderr", stderr)
			return
		}
	} else {
		out, stderr, err = RunOVNNbctl("--wait=sb", "--",
			"--may-exist", "lsp-add", logicalSwitch, portName,
			"--", "lsp-set-addresses",
			portName, "dynamic", "--", "set",
			"logical_switch_port", portName,
			"external-ids:namespace="+pod.Namespace,
			"external-ids:logical_switch="+logicalSwitch,
			"external-ids:pod=true")
		if err != nil {
			log.Error(err, "Error while creating logical port %s ", "portName", portName, "stdout", out, "stderr", stderr)
			return
		}
	}

	count := 30
	for count > 0 {
		if isStaticIP {
			out, stderr, err = RunOVNNbctl("get",
				"logical_switch_port", portName, "addresses")
		} else {
			out, stderr, err = RunOVNNbctl("get",
				"logical_switch_port", portName, "dynamic_addresses")
		}
		if err == nil && out != "[]" {
			break
		}
		if err != nil {
			log.Error(err, "Error while obtaining addresses for", "portName", portName)
			return
		}
		time.Sleep(time.Second)
		count--
	}
	if count == 0 {
		log.Error(err, "Error while obtaining addresses for", "portName", portName, "stdout", out, "stderr", stderr)
		return
	}

	// static addresses have format ["0a:00:00:00:00:01 192.168.1.3"], while
	// dynamic addresses have format "0a:00:00:00:00:01 192.168.1.3".
	outStr := strings.TrimLeft(out, `[`)
	outStr = strings.TrimRight(outStr, `]`)
	outStr = strings.Trim(outStr, `"`)
	addresses := strings.Split(outStr, " ")
	if len(addresses) != 2 {
		log.Info("Error while obtaining addresses for", "portName", portName)
		return
	}

	_, mask, err := oc.getGatewayFromSwitch(logicalSwitch)
	if err != nil {
		log.Error(err, "Error obtaining gateway address for switch", "logicalSwitch", logicalSwitch)
		return
	}

	var gatewayIP string
	if gwipAddress != "" {
		gatewayIP = gwipAddress
	} else {
		gatewayIP, err = oc.getNodeLogicalPortIPAddr(pod)
		if err != nil {
			log.Error(err, "Error obtaining gateway address for switch", "logicalSwitch", logicalSwitch)
			return
		}
	}

	annotation = fmt.Sprintf(`{\"ip_address\":[\"%s/%s\"], \"mac_address\":\"%s\", \"gateway_ip\": [\"%s\"]}`, addresses[1], mask, addresses[0], gatewayIP)

	return annotation
}

func GetSFCNetworkIfname() (f func() string) {
	var interfaceIndex int
	f = func() string {
		ifname := fmt.Sprintf("%s%d", SFCnetworkIntefacePrefixes, interfaceIndex)
		interfaceIndex++
		return ifname
	}

	return
}
