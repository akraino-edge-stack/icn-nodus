//go:build linux
// +build linux

package app

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/config"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/kube"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/network"
	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/ovn"
	"github.com/coreos/go-iptables/iptables"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const primaryiface = "eth0"

func renameLink(curName, newName string) error {
	link, err := netlink.LinkByName(curName)
	if err != nil {
		return err
	}

	if err := netlink.LinkSetDown(link); err != nil {
		return err
	}
	if err := netlink.LinkSetName(link, newName); err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}

	return nil
}

//Todo Comments
func CreateNodeOVSInternalPort(nodeintfipaddr, nodeintfipv6addr, nodeintfmacaddr, node string) error {
	nodeName := strings.ToLower(node)
	nodeOVSInternalIntfName := config.GetNodeIntfName(nodeName)

	hwAddr, err := net.ParseMAC(nodeintfmacaddr)
	if err != nil {
		logrus.Errorf("Error is converting %q to net hwaddr: %v", nodeOVSInternalIntfName, err)
		return fmt.Errorf("Error is converting %q to net hwaddr: %v", nodeOVSInternalIntfName, err)
	}

	ovsArgs := []string{
		"--", "--may-exist", "add-port", "br-int", nodeOVSInternalIntfName, "--", "set",
		"interface", nodeOVSInternalIntfName, "type=internal",
		fmt.Sprintf("mac_in_use=%s", strings.ReplaceAll(hwAddr.String(), ":", "\\:")),
		fmt.Sprintf("mac=%s", strings.ReplaceAll(hwAddr.String(), ":", "\\:")),
		fmt.Sprintf("external_ids:iface-id=%s", nodeOVSInternalIntfName),
	}
	logrus.Infof("ovs-vsctl args - %v", ovsArgs)

	//var out []byte
	out, err := exec.Command("ovs-vsctl", ovsArgs...).CombinedOutput()
	if err != nil {
		logrus.Errorf("failure in creating Node OVS internal port - %s: %v - %q", nodeOVSInternalIntfName, err, string(out))
		return fmt.Errorf("failure in creating Node OVS internal port - %s: %v - %q", nodeOVSInternalIntfName, err, string(out))
	}
	logrus.Infof("ovs-vsctl args - %v output:%v", ovsArgs, string(out))

	link, err := netlink.LinkByName(nodeOVSInternalIntfName)
	if err != nil {
		logrus.Errorf("failed to get netlink for Node OVS internal port %s: %v", nodeOVSInternalIntfName, err)
		return fmt.Errorf("failed to get netlink for Node OVS internal port %s: %v", nodeOVSInternalIntfName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		logrus.Errorf("failed to set up netlink for Node OVS internal port %s: %v", nodeOVSInternalIntfName, err)
		return fmt.Errorf("failed to set up netlink for Node OVS internal port %s: %v", nodeOVSInternalIntfName, err)
	}

	ipv4Rules, ipv6Rules := network.MasqRules(nodeOVSInternalIntfName)

	if nodeintfipaddr != "" {
		addr, err := netlink.ParseAddr(nodeintfipaddr)
		if err != nil {
			logrus.Errorf("failed to parse IP addr %s: %v", nodeintfipaddr, err)
			return fmt.Errorf("failed to parse IP addr %s: %v", nodeintfipaddr, err)
		}

		err = netlink.AddrAdd(link, addr)
		if err != nil {
			logrus.Errorf("failed to parse IP addr %s: %v", nodeintfipaddr, err)
			return fmt.Errorf("failed to add IP addr %s to %s: %v", nodeintfipaddr, nodeOVSInternalIntfName, err)
		}

		if ipv4Rules != nil {
			err = network.SetupAndEnsureIPTables(ipv4Rules, iptables.ProtocolIPv4)
			if err != nil {
				logrus.Errorf("failed to apply snat rule for %s: %v", nodeOVSInternalIntfName, err)
				return fmt.Errorf("failed to apply snat rule for %s: %v", nodeOVSInternalIntfName, err)
			}
		}
	}

	if nodeintfipv6addr != "" {
		addr, err := netlink.ParseAddr(nodeintfipv6addr)
		if err != nil {
			logrus.Errorf("failed to parse IP addr %s: %v", nodeintfipv6addr, err)
			return fmt.Errorf("failed to parse IP addr %s: %v", nodeintfipv6addr, err)
		}

		err = netlink.AddrAdd(link, addr)
		if err != nil {
			logrus.Errorf("failed to parse IP addr %s: %v", nodeintfipv6addr, err)
			return fmt.Errorf("failed to add IP addr %s to %s: %v", nodeintfipv6addr, nodeOVSInternalIntfName, err)
		}

		if ipv6Rules != nil {
			err = network.SetupAndEnsureIPTables(ipv6Rules, iptables.ProtocolIPv6)
			if err != nil {
				logrus.Errorf("failed to apply snat rule for %s: %v", nodeOVSInternalIntfName, err)
				return fmt.Errorf("failed to apply snat rule for %s: %v", nodeOVSInternalIntfName, err)
			}
		}
	}

	return nil
}

//GetPrimaryInterface return the eth0 link if it is created in the container ns
func GetPrimaryInterface() (netlink.Link, error) {

	link, err := netlink.LinkByName(primaryiface)
	if err != nil {
		return nil, err
	}

	return link, err
}

func setGateway(link netlink.Link, gatewayIP string) error {
	gw := net.ParseIP(gatewayIP)
	if gw == nil {
		return fmt.Errorf("parse ip of gateway failed")
	}
	err := ip.AddRoute(nil, gw, link)
	if err != nil {
		logrus.Errorf("ip.AddRoute failed %v gw %v link %v", err, gw, link)
		return err
	}

	return nil
}

func setpodGWRoutes(hostNet, serviceSubnet, podSubnet, gatewayIP string) error {
	podGW, err := network.GetDefaultGateway()
	if err != nil {
		logrus.Error(err, "Failed to get pod default gateway")
		return err
	}

	stdout, stderr, err := ovn.RunIP("route", "add", hostNet, "via", podGW)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	stdout, stderr, err = ovn.RunIP("route", "add", serviceSubnet, "via", podGW)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	stdout, stderr, err = ovn.RunIP("route", "add", podSubnet, "via", podGW)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	stdout, stderr, err = ovn.RunIP("route", "replace", "default", "via", gatewayIP)
	if err != nil {
		logrus.Error(err, "Failed to ip route replace", "stdout", stdout, "stderr", stderr)
		return err
	}

	return nil
}

func setExtraRoutes(hostNet, serviceSubnet, podSubnet, gatewayIP string) error {
	ipVersionArg := "-4"
	if strings.Contains(hostNet, ":") {
		ipVersionArg = "-6"
	}

	stdout, stderr, err := ovn.RunIP(ipVersionArg, "route", "add", hostNet, "via", gatewayIP)
	fmt.Printf("setExtraRoutes - hostNet - stdout: %s\n", stdout)
	fmt.Printf("setExtraRoutes - hostNet - stderr: %s\n", stderr)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	stdout, stderr, err = ovn.RunIP(ipVersionArg, "route", "add", serviceSubnet, "via", gatewayIP)
	fmt.Printf("setExtraRoutes - serviceSubnet - stdout: %s\n", stdout)
	fmt.Printf("setExtraRoutes - serviceSubnet - stderr: %s\n", stderr)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	stdout, stderr, err = ovn.RunIP(ipVersionArg, "route", "add", podSubnet, "via", gatewayIP)
	fmt.Printf("setExtraRoutes - gatewayIP - stdout: %s\n", stdout)
	fmt.Printf("setExtraRoutes - gatewayIP - stderr: %s\n", stderr)
	if err != nil && !strings.Contains(stderr, "RTNETLINK answers: File exists") {
		logrus.Error(err, "Failed to ip route add", "stdout", stdout, "stderr", stderr)
		return err
	}

	return nil
}

func setupInterface(netns ns.NetNS, containerID, ifName, macAddress string, ipAddress, gatewayIP []string, defaultGateway string, idx, mtu int, isDefaultGW bool) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}
	var hostNet []string
	var serviceSubnet string
	var podSubnet string
	var err error

	for _, gw := range gatewayIP {
		afInetVersion := syscall.AF_INET
		if strings.Contains(gw, ":") {
			afInetVersion = syscall.AF_INET6
		}

		hostNetwork, err := network.GetHostNetwork(afInetVersion)
		if err != nil {
			logrus.Error(err, "Failed to get host network")
			return nil, nil, fmt.Errorf("failed to get host network: %v", err)
		}

		hostNet = append(hostNet, hostNetwork)
	}

	// hostNet, err = network.GetHostNetwork()
	// if err != nil {
	// 	logrus.Error(err, "Failed to get host network")
	// 	return nil, nil, fmt.Errorf("failed to get host network: %v", err)
	// }

	k, err := kube.GetKubeConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("Error in kubeclientset:%v", err)
	}

	kubecli := &kube.Kube{KClient: k}
	kn, err := kubecli.GetControlPlaneServiceIPRange()
	if err != nil {
		return nil, nil, fmt.Errorf("Error in getting svc cidr range")
	}
	serviceSubnet = kn.ServiceSubnet
	podSubnet = kn.PodSubnet

	var oldHostVethName string
	err = netns.Do(func(hostNS ns.NetNS) error {
		// create the veth pair in the container and move host end into host netns
		hostVeth, containerVeth, err := ip.SetupVeth(ifName, mtu, hostNS)
		fmt.Printf("hostVeth: %v\n", hostVeth)
		fmt.Printf("containerVeth: %v\n", containerVeth)
		if err != nil {
			fmt.Printf("failed to setup veth %s: %v\n", ifName, err)
			return fmt.Errorf("failed to setup veth %s: %v", ifName, err)
			//return err
		}
		hostIface.Mac = hostVeth.HardwareAddr.String()
		contIface.Name = containerVeth.Name

		link, err := netlink.LinkByName(contIface.Name)
		if err != nil {
			fmt.Printf("failed to lookup %s: %v\n", contIface.Name, err)
			return fmt.Errorf("failed to lookup %s: %v", contIface.Name, err)
		}

		hwAddr, err := net.ParseMAC(macAddress)
		if err != nil {
			fmt.Printf("failed to parse mac address for %s: %v\n", contIface.Name, err)
			return fmt.Errorf("failed to parse mac address for %s: %v", contIface.Name, err)
		}
		err = netlink.LinkSetHardwareAddr(link, hwAddr)
		if err != nil {
			fmt.Printf("failed to add mac address %s to %s: %v", macAddress, contIface.Name, err)
			return fmt.Errorf("failed to add mac address %s to %s: %v", macAddress, contIface.Name, err)
		}
		contIface.Mac = macAddress
		contIface.Sandbox = netns.Path()

		for _, address := range ipAddress {
			err = addIpAddressToLinkDevice(address, &link, contIface.Name)
			if err != nil {
				fmt.Printf("addIpAddressToLinkDevice: %s\n", err.Error())
				return nil
			}
		}

		fmt.Printf("Value of defaultGateway- %v and ifname- %v\n", defaultGateway, ifName)
		logrus.Infof("Value of defaultGateway- %v and ifname- %v", defaultGateway, ifName)
		if defaultGateway == "true" && ifName == "eth0" {
			for _, address := range gatewayIP {
				err := setGateway(link, address)
				if err != nil {
					fmt.Printf("setGateway: %s\n", err.Error())
					return err
				}
			}
		}

		if defaultGateway == "true" && ifName != "eth0" {
			_, err = GetPrimaryInterface()
			if err != nil {
				fmt.Printf("GetPrimaryInterface: %s\n", err.Error())
				if strings.Contains(err.Error(), "Link not found") {
					for _, address := range gatewayIP {
						fmt.Printf("address: %s\n", address)
						err := setGateway(link, address)
						if err != nil {
							fmt.Printf("setGateway: %s\n", err.Error())
							return err
						}
					}
				} else {
					fmt.Printf("Error in getting the eth0 link in container ns: %s\n", err.Error())
					logrus.Error(err, "Error in getting the eth0 link in container ns")
					return err
				}
			} else {
				for index, address := range gatewayIP {
					err := setpodGWRoutes(hostNet[index], serviceSubnet, podSubnet, address)
					if err != nil {
						fmt.Printf("setpodGWRoutes: %s\n", err.Error())
						return err
					}
				}
			}
		}

		if defaultGateway == "false" && isDefaultGW == true && ifName == "eth0" {

			for index, address := range gatewayIP {
				err = setExtraRoutes(hostNet[index], serviceSubnet, podSubnet, address)
				if err != nil {
					fmt.Printf("setExtraRoutes: %s\n", err.Error())
					return err
				}
			}
		}

		oldHostVethName = hostVeth.Name

		fmt.Printf("oldHostVethName: %s\n", oldHostVethName)

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// rename the host end of veth pair
	hostIface.Name = containerID[:14] + strconv.Itoa(idx)
	fmt.Printf("renameLink - oldHostVethName: %s\n", oldHostVethName)
	if err := renameLink(oldHostVethName, hostIface.Name); err != nil {
		return nil, nil, fmt.Errorf("failed to rename %s to %s: %v", oldHostVethName, hostIface.Name, err)
	}

	return hostIface, contIface, nil
}

func addIpAddressToLinkDevice(ipAddress string, link *netlink.Link, contIfaceName string) error {
	addr, err := netlink.ParseAddr(ipAddress)
	if err != nil {
		logrus.Error("parsing address failed: ", err)
		return err
	}

	if addr.IP.To4() == nil {
		_, _, err = ovn.RunSysctl("net.ipv6.conf." + (*link).Attrs().Name + ".disable_ipv6=0")
		if err != nil {
			logrus.Error("enabling IPv6 failed: ", err)
			return fmt.Errorf("failed to enable IPv6")
		}
	}

	err = netlink.AddrAdd(*link, addr)
	if err != nil {
		return fmt.Errorf("failed to add IP addr %s to %s: %v", ipAddress, contIfaceName, err)
	}
	return nil
}

//DeleteInterface return ...
func DeleteInterface(netns ns.NetNS, ifName string) error {
	var err error

	err = netns.Do(func(hostNS ns.NetNS) error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to lookup %s: %v", ifName, err)
		}

		err = netlink.LinkDel(link)
		if err != nil {
			return fmt.Errorf("failed to delete %s: %v", link.Attrs().Name, err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// ConfigureDeleteInterface sets up the container interface
var ConfigureDeleteInterface = func(containerNetns, ifName string) error {
	netns, err := ns.GetNS(containerNetns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", containerNetns, err)
	}
	defer netns.Close()

	err = DeleteInterface(netns, ifName)
	if err != nil {
		return err
	}

	return nil
}

// ConfigureInterface sets up the container interface
var ConfigureInterface = func(containerNetns, containerID, ifName, namespace, podName, macAddress string, ipAddress, gatewayIP []string, interfaceName, defaultGateway string, idx, mtu int, isDefaultGW bool) ([]*current.Interface, error) {
	netns, err := ns.GetNS(containerNetns)
	if err != nil {
		return nil, fmt.Errorf("failed to open netns %q: %v", containerNetns, err)
	}
	defer netns.Close()

	var ifaceID string
	if interfaceName != "*" {
		ifaceID = fmt.Sprintf("%s_%s_%s", namespace, podName, interfaceName)
	} else {
		ifaceID = fmt.Sprintf("%s_%s", namespace, podName)
		interfaceName = ifName
	}
	hostIface, contIface, err := setupInterface(netns, containerID, interfaceName, macAddress, ipAddress, gatewayIP, defaultGateway, idx, mtu, isDefaultGW)
	if err != nil {
		return nil, err
	}

	ovsArgs := []string{
		"add-port", "br-int", hostIface.Name, "--", "set",
		"interface", hostIface.Name,
		fmt.Sprintf("external_ids:attached_mac=%s", macAddress),
		fmt.Sprintf("external_ids:iface-id=%s", ifaceID),
		fmt.Sprintf("external_ids:ip_address=%s", ipAddress),
		fmt.Sprintf("external_ids:sandbox=%s", containerID),
	}

	var out []byte
	out, err = exec.Command("ovs-vsctl", ovsArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failure in plugging pod interface: %v\n  %q", err, string(out))
	}

	return []*current.Interface{hostIface, contIface}, nil
}

func setupRoute(netns ns.NetNS, dst, gw, dev string) error {
	// Add Route to the namespace
	err := netns.Do(func(_ ns.NetNS) error {
		dstAddr, dstAddrNet, _ := net.ParseCIDR(dst)
		ipNet := net.IPNet{IP: dstAddr, Mask: dstAddrNet.Mask}
		link, err := netlink.LinkByName(dev)
		err = ip.AddRoute(&ipNet, net.ParseIP(gw), link)
		if err != nil {
			logrus.Errorf("ip.AddRoute failed %v dst %v gw %v", err, dst, gw)
		}
		return err
	})
	return err
}

// ConfigureRoute sets up the container routes
var ConfigureRoute = func(containerNetns, dst, gw, dev string) error {
	netns, err := ns.GetNS(containerNetns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", containerNetns, err)
	}
	defer netns.Close()
	err = setupRoute(netns, dst, gw, dev)
	return err
}

// PlatformSpecificCleanup deletes the OVS port
func PlatformSpecificCleanup(ifaceName string) (bool, error) {
	done := false
	ovsArgs := []string{
		"del-port", "br-int", ifaceName,
	}
	out, err := exec.Command("ovs-vsctl", ovsArgs...).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "no port named") {
		// DEL should be idempotent; don't return an error just log it
		logrus.Warningf("failed to delete OVS port %s: %v\n  %q", ifaceName, err, string(out))
		done = true
	}

	return done, nil
}
