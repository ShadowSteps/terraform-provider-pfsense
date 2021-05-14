package pfsense

var PFSenseApiUri = struct {
	DHCPStaticMapping string
	Auth string
	NATPortForward string
	Alias string
}{
	"/services/dhcpd/static_mapping",
	"/access_token",
	"/firewall/nat/port_forward",
	"/firewall/alias",
}
