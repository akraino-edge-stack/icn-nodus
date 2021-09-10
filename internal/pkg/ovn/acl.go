package ovn

import (
	"fmt"
	"strconv"
	"strings"
	"regexp"
)

// ACL defines structure that holds ACL rule
type ACL struct {
	Entity string
	Direction PolicyDirection
	Priority int16
	Match string
	Verdict string
}

type PolicyDirection string

const (
	Ingress PolicyDirection = "to-lport"
	Egress PolicyDirection = "from-lport"
)

// ACLList will run ovn-nbctl acl-list command
func ACLList(entity, entityType string) (string, []ACL, error) {
	var args []string

	if entityType != "" {
		args = append(args, fmt.Sprintf("--type=%sd", entityType))
	}

	args = append(args, "acl-list")

	args = append(args, entity)
	
	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to list ACLs", "stdout", stdout, "stderr", stderr)
		return stdout, nil, err
	}

	stdoutLines := strings.Split(stdout,"\n")

	var rules []ACL
	for _, line := range(stdoutLines) {
		var rule ACL
		rule.Entity = entity
		err = rule.fromString(line)
		if err != nil {
			return stdout, nil, err
		}
		rules = append(rules, rule)
	}

	return stdout, rules, nil
}

// ACLAdd will run ovn-nbctl acl-add command
func ACLAdd(rule ACL, entityType, meter, severity, name string, mayExist, enLog bool) (string, error) {
	var args []string
	
	if entityType != "" {
		args = append(args, fmt.Sprintf("--type=%sd", entityType))
	}

	if enLog {
		args = append(args, "--log")
	}

	if meter != "" {
		args = append(args, fmt.Sprintf("--meter=%sd", meter))
	}

	if name != "" {
		args = append(args, fmt.Sprintf("--name=%sd", name))
	}

	if mayExist {
		args = append(args, "--may-exist")
	}

	args = append(args, "acl-add", rule.Entity, string(rule.Direction), fmt.Sprint(rule.Priority), rule.Match, rule.Verdict)

	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to add ACL", "stdout", stdout, "stderr", stderr)
		return "", err
	}

	return stdout, nil
}

// ACLDel will run ovn-nbctl acl-del command
func ACLDel(rule ACL, entityType string) (string, error)  {
	var args []string
	
	if entityType != "" {
		args = append(args, fmt.Sprintf("--type=%sd", entityType))
	}

	args = append(args, "acl-del", rule.Entity, string(rule.Direction), fmt.Sprint(rule.Priority), rule.Match)

	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to add ACL", "stdout", stdout, "stderr", stderr)
		return "", err
	}
	return stdout, err
}

// ACLDelEntity will run ovn-nbctl acl-del command for entity
func ACLDelEntity(entity, entityType string) (string, error)  {
	var args []string
	
	if entityType != "" {
		args = append(args, fmt.Sprintf("--type=%sd", entityType))
	}

	args = append(args, "acl-del", entity)

	stdout, stderr, err := RunOVNNbctl(args...)
	if err != nil {
		log.Error(err, "Failed to add ACL", "stdout", stdout, "stderr", stderr)
		return "", err
	}
	return stdout, err
}

func (rule *ACL) fromString(line string) error {
	// remove redundant spaces
	leadSpace := regexp.MustCompile(`^\s+`)
	line = leadSpace.ReplaceAllString(line, "")
	space := regexp.MustCompile(`\s+`)
	line = space.ReplaceAllString(line, " ")
	
	// find 'match' value
	parentheses := regexp.MustCompile("\\(.*\\)")
	match := parentheses.FindString(line)
	match = strings.Trim(match,"(")
	match = strings.Trim(match,")")
	rule.Match = match

	// get 'direction', 'prioroty' and 'verdict' values
	bySpace := strings.Split(line, " ")
	if len(bySpace) < 2 {
		return fmt.Errorf("Unable to parse ACL")
	}
	rule.Direction = PolicyDirection(bySpace[0])
	p, err := strconv.ParseInt(bySpace[1], 10, 16)
	if err != nil {
		return err
	}
	rule.Priority = int16(p)
	rule.Verdict = bySpace[len(bySpace) - 1]

	return nil
}

// ToString converts ACL to string for debug purpose
func (rule ACL) ToString() string {
	return fmt.Sprintf("Entity: %s, Direction: %s, Priority: %d, Match: %s, Verdict: %s", 
	rule.Entity, string(rule.Direction), rule.Priority, rule.Match, rule.Verdict)
}

