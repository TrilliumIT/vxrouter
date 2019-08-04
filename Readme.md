VxRouter is the merging of
[docker-vxlan-plugin](https://github.com/TrilliumIT/docker-vxlan-plugin) and
[docker-drouter](https://github.com/TrilliumIT/docker-drouter).

It is a vxlan plugin for docker networks allowing layer 2 connectivity
between containers on a cluster of hosts. It is intended to be used with a
dynamic routing protocol like BGP. When a container is started a /32 route to
the continer's IP is added to the host which should be redistributed via BGP to
other hosts in the cluster. These /32 routes provide efficient routing between
the diferent vxlans across hosts, as well as the distributed database that is
used for the IPAM driver.
