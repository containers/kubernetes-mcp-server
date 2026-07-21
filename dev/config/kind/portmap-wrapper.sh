#!/bin/bash
# CNI portmap wrapper: fixes nftables hostport rules for podman compatibility.
# The CNI portmap nftables backend creates PREROUTING rules without a
# destination address check, causing all traffic on hostPort-mapped ports
# (including pod-to-ClusterIP) to be DNAT'd to the hostPort pod. This
# wrapper calls the real portmap binary and then patches the rules to only
# match locally-destined traffic, matching the OUTPUT chain's behavior.
/opt/cni/bin/portmap.real "$@"
RC=$?
{
  if nft list chain ip cni_hostport prerouting 2>/dev/null | grep -q "jump hostports"; then
    # Remove ALL "jump hostports" rules (both bare and fib-guarded) to avoid duplicates
    for H in $(nft -a list chain ip cni_hostport prerouting 2>/dev/null | grep "jump hostports" | sed 's/.*handle //'); do
      nft delete rule ip cni_hostport prerouting handle "$H"
    done
    # Add back exactly one guarded rule
    nft add rule ip cni_hostport prerouting fib daddr type local jump hostports
    conntrack -F 2>/dev/null || true
  fi
} >/dev/null 2>&1
exit $RC
