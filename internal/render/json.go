package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/zahidhasann88/portpeek/internal/ports"
)

// jsonEntry is the documented JSON object for one socket. Unknown values are
// null, not empty strings or zero, so consumers can tell them apart.
type jsonEntry struct {
	Port          uint16         `json:"port"`
	Protocol      string         `json:"protocol"`
	Family        string         `json:"family"`
	Address       *string        `json:"address"`
	State         string         `json:"state"`
	PID           *int32         `json:"pid"`
	ParentPID     *int32         `json:"parent_pid"`
	ProcessName   *string        `json:"process_name"`
	User          *string        `json:"user"`
	Command       *string        `json:"command"`
	Executable    *string        `json:"executable"`
	WorkingDir    *string        `json:"working_dir"`
	StartedAt     *string        `json:"started_at"`
	RemoteAddress *string        `json:"remote_address"`
	Container     *jsonContainer `json:"container"`
}

type jsonContainer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// JSON writes entries as a JSON array with snake_case keys. An empty result is
// written as [] so the output is always valid JSON.
func JSON(w io.Writer, entries []ports.Entry) error {
	out := make([]jsonEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, toJSON(e))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

func toJSON(e ports.Entry) jsonEntry {
	j := jsonEntry{
		Port:          e.Port,
		Protocol:      e.Protocol,
		Family:        e.Family,
		Address:       nullString(e.Address),
		State:         e.State,
		PID:           nullPID(e.PID),
		ParentPID:     nullPID(e.PPID),
		ProcessName:   nullString(e.ProcessName),
		User:          nullString(e.User),
		Command:       nullString(e.Command),
		Executable:    nullString(e.Executable),
		WorkingDir:    nullString(e.WorkingDir),
		RemoteAddress: nullString(e.RemoteAddress),
	}
	if !e.StartTime.IsZero() {
		s := e.StartTime.Format(time.RFC3339)
		j.StartedAt = &s
	}
	if e.Container != nil {
		j.Container = &jsonContainer{ID: e.Container.ID, Name: e.Container.Name, Image: e.Container.Image}
	}
	return j
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullPID(pid int32) *int32 {
	if pid <= 0 {
		return nil
	}
	return &pid
}
