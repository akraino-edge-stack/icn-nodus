#!/usr/bin/env bash
OVS_RUNDIR=/var/run/openvswitch
OVS_LOGDIR=/var/log/openvswitch

DB_NB_ADDR=${DB_NB_ADDR:-::}
DB_NB_PORT=${DB_NB_PORT:-6641}
DB_SB_ADDR=${DB_SB_ADDR:-::}
DB_SB_PORT=${DB_SB_PORT:-6642}
cmd=${1:-""}

ovn-ctl()
{
   if [[ -f /usr/bin/ovn-appctl ]] ; then
        echo /usr/share/ovn/scripts/ovn-ctl;
   else
        echo /usr/share/openvswitch/scripts/ovn-ctl;
   fi
}

if [[ -f /usr/bin/ovn-appctl ]] ; then
    # ovn-appctl is present. Use new ovn run dir path.
    OVN_RUNDIR=/var/run/ovn
    OVNCTL_PATH=/usr/share/ovn/scripts/ovn-ctl
    OVN_LOGDIR=/var/log/ovn
    OVN_ETCDIR=/etc/ovn
else
    # ovn-appctl is not present. Use openvswitch run dir path.
    OVN_RUNDIR=/var/run/openvswitch
    OVNCTL_PATH=/usr/share/openvswitch/scripts/ovn-ctl
    OVN_LOGDIR=/var/log/openvswitch
    OVN_ETCDIR=/etc/openvswitch
fi

check_ovn_control_plane() {
    $(ovn-ctl) status_northd
    $(ovn-ctl) status_ovnnb
    $(ovn-ctl) status_ovnsb
}

check_ovn_controller() {
    $(ovn-ctl) status_controller
}

# wait for ovn-sb ready
wait_ovn_sb() {
    if [[ -z "${OVN_SB_TCP_SERVICE_HOST}" ]]; then
        echo "env OVN_SB_SERVICE_HOST not exists"
        exit 1
    fi
    if [[ -z "${OVN_SB_TCP_SERVICE_PORT}" ]]; then
        echo "env OVN_SB_SERVICE_PORT not exists"
        exit 1
    fi
    while ! nc -z "${OVN_SB_TCP_SERVICE_HOST}" "${OVN_SB_TCP_SERVICE_PORT}" </dev/null;
    do
        echo "sleep 10 seconds, waiting for ovn-sb [${OVN_SB_TCP_SERVICE_HOST}]:${OVN_SB_TCP_SERVICE_PORT} ready "
        sleep 10;
    done
}

start_ovs_vswitch() {
    wait_ovn_sb
    function quit {
	/usr/share/openvswitch/scripts/ovs-ctl stop
	$(ovn-ctl) stop_controller
	exit 0
    }
    trap quit EXIT
    /usr/share/openvswitch/scripts/ovs-ctl restart --no-ovs-vswitchd --system-id=random
    # Restrict the number of pthreads ovs-vswitchd creates to reduce the
    # amount of RSS it uses on hosts with many cores
    # https://bugzilla.redhat.com/show_bug.cgi?id=1571379
    # https://bugzilla.redhat.com/show_bug.cgi?id=1572797
    if [[ `nproc` -gt 12 ]]; then
        ovs-vsctl --no-wait set Open_vSwitch . other_config:n-revalidator-threads=4
        ovs-vsctl --no-wait set Open_vSwitch . other_config:n-handler-threads=10
    fi

    # Start ovsdb
    /usr/share/openvswitch/scripts/ovs-ctl restart --no-ovsdb-server  --system-id=random
    /usr/share/openvswitch/scripts/ovs-ctl --protocol=udp --dport=6081 enable-protocol
    
}

#cleanup_ovs_server() {
#}

#cleanup_ovs_controller() {
#}

function select_interface () {
  local _interface=$1
  local _intvar=$(echo ${NODENAME}_INTERFACE)
  _interface=$(echo ${!_intvar})
}

function prepare_address {
    local _ip=$1
    if [[ "$_ip" == *":"* ]]; then
        _ip="[$_ip]"
    else
        _ip="$_ip"
    fi
    echo $_ip
}

function get_interface_ipaddress {
    local _inet="inet"
    if [[ "${POD_IP}" == *":"* ]]; then
        _inet="inet6"
    fi
    local _ipaddress=$(ip addr show dev $1 | awk -v inetvar="$_inet" '$1 == inetvar { sub("/.*", "", $2); print $2 }' | grep -v '^fe80')
    echo $_ipaddress
}

function get_default_interface_ipaddress {
    local _ip=$1
    local _default_interface=$(awk '$2 == 00000000 { print $1 }' /proc/net/route | head -n 1)
    local _ipaddress
    _ipaddress=$(get_interface_ipaddress $_default_interface)
    echo $_ipaddress
}

start_ovn_control_plane() {
    function quit {
        $(ovn-ctl) stop_northd
         exit 0
    }
    trap quit EXIT
    $(ovn-ctl) \
    --ovn-nb-db-ssl-key=/etc/openvswitch/certs/tls.key  \
    --ovn-nb-db-ssl-cert=/etc/openvswitch/certs/tls.crt \
    --ovn-nb-db-ssl-ca-cert=/etc/openvswitch/certs/ca.crt \
    --ovn-sb-db-ssl-key=/etc/openvswitch/certs/tls.key \
    --ovn-sb-db-ssl-cert=/etc/openvswitch/certs/tls.crt \
    --ovn-sb-db-ssl-ca-cert=/etc/openvswitch/certs/ca.crt \
    restart_northd
    
    ovn-nbctl --private-key=/etc/openvswitch/certs/tls.key \
    --certificate=/etc/openvswitch/certs/tls.crt \
    --ca-cert=/etc/openvswitch/certs/ca.crt \
    --ssl-protocols=TLSv1.2 \
    --ssl-ciphers=ECDHE-ECDSA-AES256-GCM-SHA384 \
    set-connection pssl:"${DB_NB_PORT}":["${DB_NB_ADDR}"]

    ovn-nbctl --private-key=/etc/openvswitch/certs/tls.key \
    --certificate=/etc/openvswitch/certs/tls.crt \
    --ca-cert=/etc/openvswitch/certs/ca.crt \
    --ssl-protocols=TLSv1.2 \
    --ssl-ciphers=ECDHE-ECDSA-AES256-GCM-SHA384 \
    set Connection . inactivity_probe=0

    ovn-sbctl --private-key=/etc/openvswitch/certs/tls.key \
    --certificate=/etc/openvswitch/certs/tls.crt \
    --ca-cert=/etc/openvswitch/certs/ca.crt \
    --ssl-protocols=TLSv1.2 \
    --ssl-ciphers=ECDHE-ECDSA-AES256-GCM-SHA384 \
    set-connection pssl:"${DB_SB_PORT}":["${DB_SB_ADDR}"]
    
    ovn-sbctl --private-key=/etc/openvswitch/certs/tls.key \
    --certificate=/etc/openvswitch/certs/tls.crt \
    --ca-cert=/etc/openvswitch/certs/ca.crt \
    --ssl-protocols=TLSv1.2 \
    --ssl-ciphers=ECDHE-ECDSA-AES256-GCM-SHA384 \
    set Connection . inactivity_probe=0

    tail -f ${OVN_LOGDIR}/ovn-northd.log
}

start_ovn_controller() {
    function quit {
	$(ovn-ctl) stop_controller
	exit 0
    }
    trap quit EXIT
    wait_ovn_sb
    local _interface
    select_interface _interface
    local node_ip_address
    if [ -z "$_interface" ]
    then
        node_ip_address=$(get_default_interface_ipaddress)
    else
        node_ip_address=$(get_interface_ipaddress $_interface)
    fi
    
    $(ovn-ctl) \
    --ovn-controller-ssl-key=/etc/openvswitch/certs/tls.key \
    --ovn-controller-ssl-cert=/etc/openvswitch/certs/tls.crt \
    --ovn-controller-ssl-ca-cert=/etc/openvswitch/certs/ca.crt \
    restart_controller

    ovs-vsctl set-ssl /etc/openvswitch/certs/tls.key /etc/openvswitch/certs/tls.crt /etc/openvswitch/certs/ca.crt

    local _ovn_sb_tcp_service_host=$(prepare_address ${OVN_SB_TCP_SERVICE_HOST})
    # Set remote ovn-sb for ovn-controller to connect to
    ovs-vsctl set open . external-ids:ovn-remote=ssl:"$_ovn_sb_tcp_service_host":"${OVN_SB_TCP_SERVICE_PORT}"
    ovs-vsctl set open . external-ids:ovn-remote-probe-interval=10000
    ovs-vsctl set open . external-ids:ovn-openflow-probe-interval=180
    ovs-vsctl set open . external-ids:ovn-encap-type=geneve
    ovs-vsctl set open . external-ids:ovn-encap-ip=$node_ip_address
    tail -f ${OVN_LOGDIR}/ovn-controller.log
}

set_nbctl() {
    wait_ovn_sb
    ovn-nbctl set-ssl /etc/openvswitch/certs/tls.key /etc/openvswitch/certs/tls.crt /etc/openvswitch/certs/ca.crt
    
    ovn-nbctl -p /etc/openvswitch/certs/tls.key -c /etc/openvswitch/certs/tls.crt -C /etc/openvswitch/certs/ca.crt \
    --db=ssl:["${OVN_NB_TCP_SERVICE_HOST}"]:"${OVN_NB_TCP_SERVICE_PORT}" --pidfile --detach --overwrite-pidfile
}

check_ovs_vswitch() {
    /usr/share/openvswitch/scripts/ovs-ctl status
}

case ${cmd} in
  "start_ovn_control_plane")
        start_ovn_control_plane
    ;;
  "check_ovn_control_plane")
        check_ovn_control_plane
    ;;
  "start_ovn_controller")
        start_ovs_vswitch
        set_nbctl
        start_ovn_controller 
    ;;
  "check_ovs_vswitch")
        check_ovs_vswitch
    ;;
  "check_ovn_controller")
        check_ovs_vswitch
        check_ovn_controller
    ;;
  "cleanup_ovs_controller")
        cleanup_ovs_controller
    ;;
  *)
    echo "invalid command ${cmd}"
    echo "valid commands: start-ovn-control-plane check_ovn_control_plane start-ovs-vswitch"
    exit 0
esac

exit 0

