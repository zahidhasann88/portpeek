// Package ports models local network sockets and the processes and containers
// that own them. It is platform-agnostic: operating-system access lives behind
// the source interfaces in source.go, and the OS-backed implementations are in
// system.go.
package ports

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// Protocol names used in Socket and Entry values.
const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// Socket states. TCP states come from the OS (for example "LISTEN" or
// "ESTABLISHED"). UDP sockets use the two states below.
const (
	StateListen    = "LISTEN"
	StateUnconnect = "UNCONN"    // UDP socket without a connected peer
	StateConnected = "CONNECTED" // UDP socket with a connected peer
)

// Family names for the IP version of a socket.
const (
	FamilyIPv4 = "ipv4"
	FamilyIPv6 = "ipv6"
)

// Socket is a raw socket as reported by the operating system, before process
// and container enrichment. PID is 0 when the owner is not visible.
type Socket struct {
	Protocol   string
	Address    string
	Port       uint16
	State      string
	PID        int32
	RemoteIP   string
	RemotePort uint16
}

// Container identifies a Docker container that publishes a host port.
type Container struct {
	ID    string // short (12 character) container ID
	Name  string
	Image string
}

// Binding is a published host port reported by the container runtime.
type Binding struct {
	HostIP    string
	HostPort  uint16
	Protocol  string
	Container Container
}

// ProcessInfo holds per-process details. Fields that could not be read are
// left empty or zero.
type ProcessInfo struct {
	Name       string
	User       string
	Command    string
	Executable string
	WorkingDir string
	PPID       int32
	StartTime  time.Time
}

// Entry is one socket row after enrichment. It is the platform-agnostic model
// consumed by rendering and the CLI. Empty strings and zero values mean
// "unknown".
type Entry struct {
	Port          uint16
	Protocol      string
	Family        string
	Address       string
	State         string
	RemoteAddress string
	PID           int32
	PPID          int32
	ProcessName   string
	User          string
	Command       string
	Executable    string
	WorkingDir    string
	StartTime     time.Time
	Container     *Container
}

// IsListening reports whether the entry is a listening TCP socket or an
// unconnected UDP socket.
func (e Entry) IsListening() bool {
	return isListeningState(e.State)
}

// DisplayProcess returns the process name to show to a user. A docker-proxy
// entry that has been attributed to a container shows the container name.
func (e Entry) DisplayProcess() string {
	if e.Container != nil && (e.ProcessName == "" || IsDockerProxy(e.ProcessName)) {
		return e.Container.Name
	}
	return e.ProcessName
}

// dockerProxyNames are process names that hold published container ports on
// behalf of the container runtime.
var dockerProxyNames = map[string]bool{
	"docker-proxy":       true, // Docker Engine on Linux
	"rootlesskit":        true, // rootless Docker port driver
	"com.docker.backend": true, // Docker Desktop (macOS, Windows)
	"vpnkit":             true, // older Docker Desktop releases
}

// IsDockerProxy reports whether a process name belongs to the container
// runtime's port-forwarding helpers.
func IsDockerProxy(name string) bool {
	n := strings.ToLower(strings.TrimSuffix(name, ".exe"))
	return dockerProxyNames[n]
}

// Family returns the IP family ("ipv4" or "ipv6") of an address.
func Family(addr string) string {
	if strings.Contains(addr, ":") {
		return FamilyIPv6
	}
	return FamilyIPv4
}

// remoteString joins an IP and port the way net.JoinHostPort does. It returns
// "" when the socket has no remote peer.
func remoteString(ip string, port uint16) string {
	if port == 0 {
		return ""
	}
	return net.JoinHostPort(ip, strconv.FormatUint(uint64(port), 10))
}
