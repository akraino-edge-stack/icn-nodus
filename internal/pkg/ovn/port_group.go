package ovn

// PortGroup defines OVN port group struct
type PortGroup struct {
	Name string
	Ports []string
}

// PGAddWithPorts creates port group and adds ports to this port group
func PGAddWithPorts(group string, ports []string) (string, error) {
	args := preparePGArgs("pg-add", group, ports)
	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to add port group", "stdout", stdout, "stderr", stderr)
		return "", err
	}
	return stdout, err
}

// PGAdd creates port group
func PGAdd(group string) (string, error) {
	return PGAddWithPorts(group, []string{})
}

// PGSetPorts adds ports to portg roup
func PGSetPorts(group string, ports []string) (string, error) {
	args := preparePGArgs("pg-set-ports", group, ports)
	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to set port group's ports", "stdout", stdout, "stderr", stderr)
		return "", err
	}
	return stdout, err
}

// PGDel deletes port group
func PGDel(group string) (string, error) {
	stdout, stderr, err := RunOVNNbctl("pg-del", group)
	if err != nil {
		log.Error(err, "Failed to delete port group", "stdout", stdout, "stderr", stderr)
		return "", err
	}
	return stdout, err
}

func preparePGArgs(command, group string, ports []string) []string {
	var args []string
	args = append(args, command, group)
	args = append(args, ports...)
	return args
}

// AddDenyPG creates PG that denies all ingress/egress access
func AddDenyPG(pgName string, isIngressPolicy, isEgressPolicy bool) error {
	_, existingACLs, err := ACLList(pgName, "")

	if err != nil {
		PGAdd(pgName)
	}

	if len(existingACLs) < 4 {
		if isIngressPolicy {
			err = addDenyRule(pgName, Ingress)
			if err != nil {
				log.Error(err, "Failed to add general deny all ingress ACL")
				return err
			}
		}
		if isEgressPolicy {
			err = addDenyRule(pgName, Egress)
			if err != nil {
				log.Error(err, "Failed to add general deny all ingress ACL")
				return err
			}
		}
	}

	return nil
}

func addDenyRule(pgName string, direction PolicyDirection) error {

	matchPort := "inport"
	if direction == Ingress {
		matchPort = "outport"
	}

	matchPort += " == " + "@" +pgName

	// rule drop all packets
	ruleDeny := ACL{Entity: pgName,
		Direction: direction,
		Priority: 0,
		Match: matchPort + " && (tcp || udp || icmp || sctp)",
		Verdict: "drop",
	}

	stdout, err := ACLAdd(ruleDeny, "", "", "", "GeneralDenyACL", true, false)
	if err != nil {
		log.Error(err, "Failed to add general deny all ACL", "stdout", stdout)
		return err
	}

	// rule to allow arp packets specifically
	ruleAllowARP := ACL{Entity: pgName,
		Direction: direction,
		Priority: 1,
		Match: matchPort + " && arp",
		Verdict: "allow",
	}

	stdout, err = ACLAdd(ruleAllowARP, "", "", "", "ArpAllowACL", true, false)
	if err != nil {
		log.Error(err, "Failed to add general ARP allow ACL", "stdout", stdout)
		return err
	}

	return nil
}
