package ovn

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/akraino-edge-stack/icn-nodus/internal/pkg/auth"
	kexec "k8s.io/utils/exec"
)

const (
	ovsCommandTimeout = 15
	ovnNbctlCommand   = "ovn-nbctl"
	ovsVsctlCommand   = "ovs-vsctl"
	ipCommand         = "ip"
)

// Exec runs various OVN and OVS utilities
type execHelper struct {
	exec      kexec.Interface
	nbctlPath string
	vsctlPath string
	ipPath    string
	hostIP    string
	hostPort  string
}

var runner *execHelper

// SetupOvnUtils does internal OVN initialization
var SetupOvnUtils = func() error {
	// Setup Distributed Router
	err := setupDistributedRouter(ovn4nfvRouterName)
	if err != nil {
		log.Error(err, "Failed to initialize OVN Distributed Router")
		return err
	}

	log.Info("OVN Network", "OVN Default NW", Ovn4nfvDefaultNw,
		"OVN Subnet", ovnConf.Subnetv4,
		"OVN Gateway IP", ovnConf.GatewayIPv4,
		"OVN ExcludeIPs", ovnConf.ExcludeIPs,
		"OVN IPv6 Subnet", ovnConf.Subnetv6,
		"OVN IPv6 Gateway IP", ovnConf.GatewayIPv6)

	_, _, err = createOvnLS(Ovn4nfvDefaultNw, ovnConf.Subnetv4, ovnConf.GatewayIPv4, ovnConf.ExcludeIPs, ovnConf.Subnetv6, ovnConf.GatewayIPv6)
	if err != nil && !reflect.DeepEqual(err, fmt.Errorf("LS exists")) {
		log.Error(err, "Failed to create ovn4nfvk8s default nw")
		return err
	}
	return nil
}

// SetExec validates executable paths and saves the given exec interface
// to be used for running various OVS and OVN utilites
func SetExec(exec kexec.Interface, isOpenshift bool) error {
	var err error

	runner = &execHelper{exec: exec}
	runner.nbctlPath, err = exec.LookPath(ovnNbctlCommand)
	if err != nil {
		return err
	}
	runner.vsctlPath, err = exec.LookPath(ovsVsctlCommand)
	if err != nil {
		return err
	}
	runner.ipPath, err = exec.LookPath(ipCommand)
	if err != nil {
		return err
	}

	if !isOpenshift {
		runner.hostIP = getHostIP()
		// OVN Host Port
		runner.hostPort = os.Getenv("OVN_NB_TCP_SERVICE_PORT")
	} else {
		runner.hostIP = "ovnkube-db." + auth.OpenshiftNamespace + ".svc.cluster.local"
		runner.hostPort = "9641"
	}
	log.Info("Host Port", "IP", runner.hostIP, "Port", runner.hostPort)

	return nil
}

func getHostIP() string {
	hostIP := os.Getenv("OVN_NB_TCP_SERVICE_HOST")

	if strings.Contains(hostIP, ":") {
		hostIP = "[" + hostIP + "]"
	}
	return hostIP
}

// Run the ovn-ctl command and retry if "Connection refused"
// poll waitng for service to become available
func runOVNretry(cmdPath string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {

	retriesLeft := 200
	for {
		stdout, stderr, err := run(cmdPath, args...)
		if err == nil {
			return stdout, stderr, err
		}
		// Connection refused
		// Master may not be up so keep trying
		if strings.Contains(stderr.String(), "Connection refused") {
			if retriesLeft == 0 {
				return stdout, stderr, err
			}
			retriesLeft--
			time.Sleep(2 * time.Second)
		} else {
			// Some other problem for caller to handle
			return stdout, stderr, err
		}
	}
}

func run(cmdPath string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := runner.exec.Command(cmdPath, args...)
	cmd.SetStdout(stdout)
	cmd.SetStderr(stderr)
	log.V(1).Info("exec:", "cmdPath", cmdPath, "args", strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		log.Info("ovs", "Error:", err, "cmdPath", cmdPath, "args", strings.Join(args, " "), "stdout", stdout, "stderr", stderr)
	} else {
		log.V(1).Info("output:", "stdout", stdout)
	}
	return stdout, stderr, err
}

// RunOVNNbctlWithTimeout runs command via ovn-nbctl with a specific timeout
func RunOVNNbctlWithTimeout(timeout int, args ...string) (string, string, error) {
	var cmdArgs []string
	if len(runner.hostIP) > 0 {
		cmdArgs = []string{
			fmt.Sprintf("--db=ssl:%s:%s", runner.hostIP, runner.hostPort),
			"-p", path.Join(auth.DefaultOvnCertDir, auth.KeyFile),
			"-c", path.Join(auth.DefaultOvnCertDir, auth.CertFile),
			"-C", path.Join(auth.DefaultOvnCertDir, auth.CAFile),
		}
	}
	cmdArgs = append(cmdArgs, fmt.Sprintf("--timeout=%d", timeout))
	cmdArgs = append(cmdArgs, args...)
	stdout, stderr, err := runOVNretry(runner.nbctlPath, cmdArgs...)
	return strings.Trim(strings.TrimSpace(stdout.String()), "\""), stderr.String(), err
}

// RunOVNNbctl runs a command via ovn-nbctl.
func RunOVNNbctl(args ...string) (string, string, error) {
	return RunOVNNbctlWithTimeout(ovsCommandTimeout, args...)
}

// RunIP runs a command via the iproute2 "ip" utility
func RunIP(args ...string) (string, string, error) {
	stdout, stderr, err := run(runner.ipPath, args...)
	return strings.TrimSpace(stdout.String()), stderr.String(), err
}

// RunOVSVsctl runs a command via ovs-vsctl.
func RunOVSVsctl(args ...string) (string, string, error) {
	cmdArgs := []string{fmt.Sprintf("--timeout=%d", ovsCommandTimeout)}
	cmdArgs = append(cmdArgs, args...)
	stdout, stderr, err := run(runner.vsctlPath, cmdArgs...)
	return strings.Trim(strings.TrimSpace(stdout.String()), "\""), stderr.String(), err
}

func RunSysctl(args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	return run("sysctl", args...)
}

// Hash takes string input and outputs hash value for that string as string
func Hash(s string) string {
	hash := fnv.New64a()
	if _, err := hash.Write([]byte(s)); err == nil {
		output := strconv.FormatUint(hash.Sum64(), 10)
		return output
	} else {
		log.Error(err, "Hashing failure")
	}
    return ""
}
