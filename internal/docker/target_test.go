package docker

import (
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestResolveTarget(t *testing.T) {
	cases := []struct {
		name       string
		dockerHost string
		want       Target
		wantErr    bool
	}{
		{name: "default", dockerHost: "", want: defaultTarget()},
		{name: "unix socket", dockerHost: "unix:///run/user/1000/docker.sock", want: Target{Network: "unix", Address: "/run/user/1000/docker.sock"}},
		{name: "npipe", dockerHost: "npipe:////./pipe/docker_engine", want: Target{Network: "npipe", Address: "//./pipe/docker_engine"}},
		{name: "tcp", dockerHost: "tcp://127.0.0.1:2375", want: Target{Network: "tcp", Address: "127.0.0.1:2375"}},
		{name: "unix without path", dockerHost: "unix://", wantErr: true},
		{name: "unsupported scheme", dockerHost: "ssh://user@host", wantErr: true},
		{name: "garbage", dockerHost: "%%%", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTarget(env(map[string]string{"DOCKER_HOST": tc.dockerHost}))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got target %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveTarget() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDefaultTargetIsPlatformSocket(t *testing.T) {
	got := defaultTarget()
	if got.Network != "unix" && got.Network != "npipe" {
		t.Errorf("default network = %q", got.Network)
	}
	if got.Address == "" {
		t.Error("default address is empty")
	}
}
