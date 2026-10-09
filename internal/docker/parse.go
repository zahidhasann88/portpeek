package docker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// shortIDLength is the number of characters of a container ID that are shown.
const shortIDLength = 12

// containerRecord holds the fields used from GET /containers/json.
type containerRecord struct {
	ID    string       `json:"Id"`
	Names []string     `json:"Names"`
	Image string       `json:"Image"`
	Ports []portRecord `json:"Ports"`
}

// portRecord is one entry of a container's Ports array. Ports that are only
// exposed (no host binding) have PublicPort 0.
type portRecord struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

// ParseContainers converts a GET /containers/json response into one Binding
// per published TCP or UDP host port.
func ParseContainers(data []byte) ([]ports.Binding, error) {
	var records []containerRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode container list: %w", err)
	}
	var out []ports.Binding
	for _, r := range records {
		c := ports.Container{
			ID:    shortID(r.ID),
			Name:  containerName(r),
			Image: r.Image,
		}
		for _, p := range r.Ports {
			if p.PublicPort == 0 {
				continue
			}
			proto := strings.ToLower(p.Type)
			if proto != ports.ProtocolTCP && proto != ports.ProtocolUDP {
				continue
			}
			out = append(out, ports.Binding{
				HostIP:    p.IP,
				HostPort:  p.PublicPort,
				Protocol:  proto,
				Container: c,
			})
		}
	}
	return out, nil
}

func shortID(id string) string {
	if len(id) > shortIDLength {
		return id[:shortIDLength]
	}
	return id
}

func containerName(r containerRecord) string {
	if len(r.Names) > 0 {
		if name := strings.TrimPrefix(r.Names[0], "/"); name != "" {
			return name
		}
	}
	return shortID(r.ID)
}
