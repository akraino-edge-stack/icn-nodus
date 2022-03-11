#!/usr/bin/env bash

test_network () {
  name=$(fuzz_name "example-network")

  manifest="
apiVersion: k8s.plugin.opnfv.org/v1alpha1
kind: Network
metadata:
 name: '$name'
spec:
 cniType: ovn4nfv
 ipv4Subnets:
 - subnet: '$(fuzz "172.16.33.0/24")'
   name: '$(fuzz "subnet1")'
   gateway: '$(fuzz "172.16.33.1/24")'
   excludeIps: '$(fuzz "172.16.33.2 172.16.33.5..172.16.33.10")'"

  apply_and_delete "network" "$manifest" "$name"
}

test_provider_network () {
  name=$(fuzz_name "example-network")

  manifest="
apiVersion: k8s.plugin.opnfv.org/v1alpha1
kind: ProviderNetwork
metadata:
  name: '$name'
spec:
  cniType: ovn4nfv
  ipv4Subnets:
  - subnet: '$(fuzz "172.16.33.0/24")'
    name: '$(fuzz "subnet1")'
    gateway: '$(fuzz "172.16.33.1/24")'
    excludeIps: '$(fuzz "172.16.33.2 172.16.33.5..172.16.33.10")'
  providerNetType: '$(fuzz "VLAN")'
  vlan:
    vlanId: '$(fuzz "100")'
    providerInterfaceName: '$(fuzz "eth1")'
    logicalInterfaceName: '$(fuzz "eth1.100")'
    vlanNodeSelector: '$(fuzz "specific")'
    nodeLabelList:
    - '$(fuzz "kubernetes.io/hostname=testnode1")'"

  apply_and_delete "providernetwork" "$manifest" "$name"
}

test_network_chaining () {
  name=$(fuzz_name "example-network")

  manifest="
apiVersion: k8s.plugin.opnfv.org/v1alpha1
kind: NetworkChaining
metadata:
  name: '$name'
spec:
  chainType: '$(fuzz "Routing")'
  routingSpec:
    namespace: '$(fuzz "default")'
    networkChain: '$(fuzz "app=slb,dync-net1,app=ngfw,dync-net2,app=sdwan")'
    left:
    - networkName: '$(fuzz "pnet1")'
      gatewayIp: '$(fuzz "172.30.10.2")'
      subnet: '$(fuzz "172.30.10.0/24")'
    right:
    - networkName: '$(fuzz "pnet2")'
      gatewayIp: '$(fuzz "172.30.20.2")'
      subnet: '$(fuzz "172.30.20.0/24")'"

  apply_and_delete "networkchaining" "$manifest" "$name"
}

fuzz_name () {
  input="$1"
  fuzzed="$(fuzz "$input" | cut -c1-253 | tr '[:upper:]' '[:lower:]' | tr -dc '[:alnum:]-')"
  [[ -n "$fuzzed" ]] && echo -n "$fuzzed" || echo -n "$input"
}

fuzz () {
  input="$1"
  echo -n "$input" | radamsa | tr -dc '[:print:]' | tr -d '\n' | sed -e "s/'/''/g"
}

apply_and_delete () {
  kind="$1"
  manifest="$2"
  network_name="$3"

  echo
  echo "----------------------------------------------------------------------------------------------------------"
  echo "Applying $kind"
  echo "----------------------------------------------------------------------------------------------------------"
  echo "$manifest"
  echo
  echo "$manifest" | kubectl apply -f - && \
  echo && \
  echo "----------------------------------------------------------------------------------------------------------" && \
  echo "Deleting $kind $network_name" && \
  echo "----------------------------------------------------------------------------------------------------------" && \
  echo && \
  kubectl delete "$kind" "$network_name"
}

print_help () {
   echo "This script performs fuzz testing against Nodus. A test scenario consists of:"
   echo "- applying and deleting generated Network custom resource"
   echo "- applying and deleting generated NetworkProvider custom resource"
   echo "- applying and deleting generated NetworkChaining custom resource"
   echo
   echo "The script requires radamsa tool installed. A procedure of installation is described here:"
   echo "https://wiki.ith.intel.com/pages/viewpage.action?pageId=2102835228"
   echo
   echo "Syntax: test.sh [-n <digit>|-h]"
   echo "options:"
   echo "n     Perform the test scenario multiple times (by default 1)."
   echo "h     Print this Help."
   echo
}

declare -i iterations_number=1

while getopts ":n:h:" arg; do
    case $arg in
      n) iterations_number=$OPTARG;;
      h) print_help && exit 0;;
      *) print_help && exit 1;;
    esac
done

for i in $(seq "$iterations_number")
do
  echo
  echo "=========================================================================================================="
  echo "Iteration $i/$iterations_number"
  echo "=========================================================================================================="

  test_network
  test_provider_network
  test_network_chaining
done
