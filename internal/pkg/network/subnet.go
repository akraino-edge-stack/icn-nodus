package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	v1alpha1 "github.com/akraino-edge-stack/icn-nodus/pkg/apis/k8s/v1alpha1"
	"github.com/flannel-io/flannel/pkg/ip"
)

type Config struct {
	Network   ip.IP4Net
	SubnetMin ip.IP4
	SubnetMax ip.IP4
	SubnetLen uint
}

const netConfPath = "/etc/subnet/virutal-net-conf.json"
const virutalnetwork = "virutal-net"

// ParseConfig return network
func ParseConfig(s string) ([]v1alpha1.IpSubnet, error) {
	cfg := new(Config)
	err := json.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	log.V(1).Info("Value of the config", "cfg", cfg)

	if cfg.SubnetLen > 0 {
		// SubnetLen needs to allow for a virutal network.
		if cfg.SubnetLen > 30 {
			return nil, errors.New("SubnetLen must be less than /31")
		}

		// SubnetLen needs to fit _more_ than twice into the Network.
		// the first subnet isn't used, so splitting into two one only provide one usable virutal network
		if cfg.SubnetLen < cfg.Network.PrefixLen+2 {
			return nil, errors.New("Network must be able to accommodate at least four subnets")
		}
	} else {
		// If the network is smaller than a /28 then the network isn't big enough for Nodus so return an error.
		// Default to giving each virutal network at least a /24 (as long as the network is big enough to support at least four virutal networks)
		// Otherwise, if the network is too small to give each virutal network a /24 just split the network into four.
		if cfg.Network.PrefixLen > 28 {
			// Each subnet needs at least four addresses (/30) and the network needs to accommodate at least four
			// since the first subnet isn't used, so splitting into two would only provide one usable virutal network.
			// So the min useful PrefixLen is /28
			return nil, errors.New("Network is too small. Minimum useful network prefix is /28")
		} else if cfg.Network.PrefixLen <= 22 {
			// Subent is big enough to give each virutal network a /24
			cfg.SubnetLen = 24
		} else {
			// Use +2 to provide four virutal network per subnet.
			cfg.SubnetLen = cfg.Network.PrefixLen + 2
		}
	}

	subnetSize := ip.IP4(1 << (32 - cfg.SubnetLen))

	if cfg.SubnetMin == ip.IP4(0) {
		// skip over the first subnet otherwise it causes problems. e.g.
		// if Network is 10.100.0.0/16, having an interface with 10.0.0.0
		// conflicts with the broadcast address.
		cfg.SubnetMin = cfg.Network.IP + subnetSize
	} else if !cfg.Network.Contains(cfg.SubnetMin) {
		return nil, errors.New("SubnetMin is not in the range of the Network")
	}

	if cfg.SubnetMax == ip.IP4(0) {
		cfg.SubnetMax = cfg.Network.Next().IP - subnetSize
	} else if !cfg.Network.Contains(cfg.SubnetMax) {
		return nil, errors.New("SubnetMax is not in the range of the Network")
	}

	// The SubnetMin and SubnetMax need to be aligned to a SubnetLen boundary
	mask := ip.IP4(0xFFFFFFFF << (32 - cfg.SubnetLen))
	if cfg.SubnetMin != cfg.SubnetMin&mask {
		return nil, fmt.Errorf("SubnetMin is not on a SubnetLen boundary: %v", cfg.SubnetMin)
	}

	if cfg.SubnetMax != cfg.SubnetMax&mask {
		return nil, fmt.Errorf("SubnetMax is not on a SubnetLen boundary: %v", cfg.SubnetMax)
	}

	currentIP := cfg.Network.IP
	var vn []v1alpha1.IpSubnet
	for i := 0; currentIP != cfg.Network.Next().IP; i++ {
		var n v1alpha1.IpSubnet
		n.Name = fmt.Sprintf("%s%s", virutalnetwork, strconv.Itoa(i))
		n.Subnet = fmt.Sprintf("%s/%s", currentIP, strconv.FormatUint(uint64(cfg.SubnetLen), 10))
		n.Gateway = fmt.Sprintf("%s/%s", currentIP+ip.IP4(1), strconv.FormatUint(uint64(cfg.SubnetLen), 10))
		n.ExcludeIps = fmt.Sprintf("%s", currentIP+ip.IP4(2))
		vn = append(vn, n)
		currentIP = currentIP + subnetSize
	}

	if len(vn) == 0 {
		return nil, fmt.Errorf("value of virutal network is nil, user subnet is not right")
	}

	log.Info("Value of the virutal network slice", "virutal networks", vn)
	return vn, nil
}

// CheckVirutalNetworkConf returns true or false
func CheckVirutalNetworkConf() (bool, error) {
	_, err := os.Stat(netConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CalculateVirutalNetworkSubnet returns virutal networks configuration
func CalculateVirutalNetworkSubnet() ([]v1alpha1.IpSubnet, error) {
	netConf, err := ioutil.ReadFile(netConfPath)
	if err != nil {
		return nil, fmt.Errorf("Error in reading the subnet conf file: %v", err.Error())
	}

	virutalNets, err := ParseConfig(string(netConf))
	if err != nil {
		return nil, fmt.Errorf("Error in Parsing netConfPath file and creating virutal networks : %v", err.Error())
	}

	return virutalNets, nil
}