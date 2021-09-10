package ovn

// PortGroup defines OVN port group struct
type PortGroup struct {
	Name string
	Ports []string
}

const (
	DefaultDenyIngress = "NodusDefaultIngress"
	DefaultDenyEgress = "NodusDefaultEgress"
)

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
func AddDenyPG(name string, isIngressPolicy, isEgressPolicy bool) error {
	pgName := name + "_Deny"
	stdout, existingACLs, err := ACLList(pgName, "")

	if err != nil {
		log.V(1).Info("PGAdd")
		PGAdd(pgName)
	}

	if len(existingACLs) < 4 {
		if isIngressPolicy {
			err = addDenyRule(pgName, Ingress)
			if err != nil {
				log.Error(err, "Failed to add general deny all ingress ACL", "stdout", stdout)
				return err
			}
		}
		if isEgressPolicy {
			err = addDenyRule(pgName, Egress)
			if err != nil {
				log.Error(err, "Failed to add general deny all ingress ACL", "stdout", stdout)
				return err
			}
		}
	}

	return nil
}

func addDenyRule(pgName string, direction PolicyDirection) error {
	ruleDeny := ACL{Entity: pgName,
		Direction: direction,
		Priority: 0,
		Match: "tcp || udp || icmp",
		Verdict: "drop",
	}

	stdout, err := ACLAdd(ruleDeny, "", "", "", "GeneralDenyACL", true, false)
	if err != nil {
		log.Error(err, "Failed to add general deny all ACL", "stdout", stdout)
		return err
	}

	ruleAllowARP := ACL{Entity: pgName,
		Direction: direction,
		Priority: 1,
		Match: "arp",
		Verdict: "allow",
	}

	stdout, err = ACLAdd(ruleAllowARP, "", "", "", "ArpAllowACL", true, false)
	if err != nil {
		log.Error(err, "Failed to add general ARP allow ACL", "stdout", stdout)
		return err
	}

	return nil
}

// AddDefaultPG adds default PG to deny ingress/egress traffic
func AddDefaultPG(direction PolicyDirection) error {
	var pgName string
	if direction == Ingress {
		pgName = DefaultDenyIngress
	} else {
		pgName = DefaultDenyEgress
	}

	stdout, existingACLs, err := ACLList(pgName, "")
	log.V(1).Info("ACLList output")
	log.V(1).Info(stdout)

	if err != nil {
		PGAdd(pgName)
	}

	if len(existingACLs) < 2 {
		ruleDeny := ACL{Entity: pgName,
			Direction: direction,
			Priority: 0,
			Match: "tcp || udp || icmp",
			Verdict: "drop",
		}

		stdout, err := ACLAdd(ruleDeny, "", "", "", "GeneralDenyACL", true, false)
		if err != nil {
			log.Error(err, "Failed to add general deny all ACL", "stdout", stdout)
			return err
		}

		ruleAllowARP := ACL{Entity: pgName,
			Direction: direction,
			Priority: 1,
			Match: "arp",
			Verdict: "allow",
		}

		stdout, err = ACLAdd(ruleAllowARP, "", "", "", "ArpAllowACL", true, false)
		if err != nil {
			log.Error(err, "Failed to add general ARP allow ACL", "stdout", stdout)
			return err
		}
	}

	return nil
}
