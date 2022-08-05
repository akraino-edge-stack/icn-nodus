package network

import (
	"fmt"
	"os"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/coreos/go-iptables/iptables"
)

var log = logf.Log.WithName("network")

type IPTables interface {
	AppendUnique(table string, chain string, rulespec ...string) error
	Delete(table string, chain string, rulespec ...string) error
	Exists(table string, chain string, rulespec ...string) (bool, error)
}

type IPTablesRule struct {
	table    string
	chain    string
	rulespec []string
}

func MasqRules(ifname string) ([]IPTablesRule, []IPTablesRule) {

	subnet := os.Getenv("OVN_SUBNET")
	subnetV6 := os.Getenv("OVN_SUBNET_V6")

	if subnet == "" && subnetV6 == "" {
		log.Info("OVN subnet is not set in nfn-operator configmap env")
	}

	var ipv4Rules []IPTablesRule
	var ipv6Rules []IPTablesRule

	if subnet != "" {
		ipv4Rules = []IPTablesRule{
			// This rule makes sure ifname is SNAT
			{"nat", "POSTROUTING", []string{"-o", ifname, "-j", "MASQUERADE"}},
			// NAT if it's not multicast traffic
			{"nat", "POSTROUTING", []string{"-s", subnet, "!", "-d", "224.0.0.0/4", "-j", "MASQUERADE"}},
		}
	}

	if subnetV6 != "" {
		ipv6Rules = []IPTablesRule{
			// This rule makes sure ifname is SNAT
			{"nat", "POSTROUTING", []string{"-o", ifname, "-j", "MASQUERADE"}},
			// NAT if it's not multicast traffic
			{"nat", "POSTROUTING", []string{"-s", subnet, "!", "-d", "ff00::/8", "-j", "MASQUERADE"}},
		}
	}

	return ipv4Rules, ipv6Rules
}

func FilterRules() ([]IPTablesRule, []IPTablesRule) {
	var ipv4Rules []IPTablesRule
	var ipv6Rules []IPTablesRule

	for _, port := range []string{"6081", "6641", "6642", "8080", "50000"} {
		ipv4Rules = append(ipv4Rules, IPTablesRule{
			"filter", "INPUT", []string{"-p", "tcp", "-m", "tcp", "--dport", port, "-j", "ACCEPT"}})
		ipv4Rules = append(ipv4Rules, IPTablesRule{
			"filter", "INPUT", []string{"-p", "udp", "-m", "udp", "--dport", port, "-j", "ACCEPT"}})

		ipv6Rules = append(ipv6Rules, IPTablesRule{
			"filter", "INPUT", []string{"-p", "tcp", "-m", "tcp", "--dport", port, "-j", "ACCEPT"}})
		ipv6Rules = append(ipv6Rules, IPTablesRule{
			"filter", "INPUT", []string{"-p", "udp", "-m", "udp", "--dport", port, "-j", "ACCEPT"}})
	}

	return ipv4Rules, ipv6Rules
}

func ForwardRules(ovnNetwork string) []IPTablesRule {
	return []IPTablesRule{
		// These rules allow traffic to be forwarded if it is to or from the ovn network range.
		{"filter", "FORWARD", []string{"-s", ovnNetwork, "-j", "ACCEPT"}},
		{"filter", "FORWARD", []string{"-d", ovnNetwork, "-j", "ACCEPT"}},
	}
}

func ipTablesRulesExist(ipt IPTables, rules []IPTablesRule) (bool, error) {
	for _, rule := range rules {
		exists, err := ipt.Exists(rule.table, rule.chain, rule.rulespec...)
		if err != nil {
			// this shouldn't ever happen
			return false, fmt.Errorf("failed to check rule existence: %v", err)
		}
		if !exists {
			return false, nil
		}
	}

	return true, nil
}

func SetupAndEnsureIPTables(rules []IPTablesRule, protocol iptables.Protocol) error {
	ipt, err := iptables.NewWithProtocol(protocol)
	if err != nil {
		// if we can't find iptables, give up and return
		log.Error(err, "Failed to setup IPTables. iptables binary was not found")
		return err
	}

	// Ensure that all the iptables rules exist every 5 seconds
	if err := ensureIPTables(ipt, rules); err != nil {
		log.Error(err, "Failed to ensure iptables rules")
		return err
	}

	return nil
}

// DeleteIPTables delete specified iptables rules
func DeleteIPTables(rules []IPTablesRule) error {
	ipt, err := iptables.New()
	if err != nil {
		// if we can't find iptables, give up and return
		log.Error(err, "Failed to setup IPTables. iptables binary was not found")
		return err
	}
	teardownIPTables(ipt, rules)
	return nil
}

func ensureIPTables(ipt IPTables, rules []IPTablesRule) error {
	exists, err := ipTablesRulesExist(ipt, rules)
	if err != nil {
		return fmt.Errorf("Error checking rule existence: %v", err)
	}
	if exists {
		// if all the rules already exist, no need to do anything
		return nil
	}
	// Otherwise, teardown all the rules and set them up again
	// We do this because the order of the rules is important
	log.Info("Some iptables rules are missing; deleting and recreating rules")
	teardownIPTables(ipt, rules)
	if err = setupIPTables(ipt, rules); err != nil {
		return fmt.Errorf("Error setting up rules: %v", err)
	}
	return nil
}

func setupIPTables(ipt IPTables, rules []IPTablesRule) error {
	for _, rule := range rules {
		log.Info("Adding iptables rule: ", "rule", strings.Join(rule.rulespec, " "))
		err := ipt.AppendUnique(rule.table, rule.chain, rule.rulespec...)
		if err != nil {
			return fmt.Errorf("failed to insert IPTables rule: %v", err)
		}
	}

	return nil
}

func teardownIPTables(ipt IPTables, rules []IPTablesRule) {
	for _, rule := range rules {
		log.Info("Deleting iptables rule: ", "rule", strings.Join(rule.rulespec, " "))
		// We ignore errors here because if there's an error it's almost certainly because the rule
		// doesn't exist, which is fine (we don't need to delete rules that don't exist)
		ipt.Delete(rule.table, rule.chain, rule.rulespec...)
	}
}
