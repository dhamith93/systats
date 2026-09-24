package systats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// defaultContainerSocketTimeout bounds the metadata request when
// SyStats.ContainerSocketTimeout is zero. The socket is a local unix
// socket, so anything slower than this is a wedged daemon rather than a
// slow one.
const defaultContainerSocketTimeout = 2 * time.Second

// containerMetadata is what a Docker-compatible API adds on top of what
// the cgroup path already says: the human-facing identity.
type containerMetadata struct {
	name   string
	image  string
	state  string
	labels map[string]string
}

// apiContainer is the subset of the Engine API's /containers/json entry
// that's decoded. Podman's compat socket serves the same shape.
type apiContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
}

// fetchContainerMetadata lists running containers from a Docker-compatible
// API on socketPath, keyed by full container ID. It talks HTTP over the
// unix socket with the standard library rather than pulling in a client
// SDK. Every failure is returned rather than handled here; the caller
// treats metadata as optional.
func fetchContainerMetadata(ctx context.Context, socketPath string, timeout time.Duration) (map[string]containerMetadata, error) {
	if socketPath == "" {
		return nil, errors.New("no container socket configured")
	}
	if timeout <= 0 {
		timeout = defaultContainerSocketTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := &net.Dialer{}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socketPath)
			},
			DisableKeepAlives: true,
		},
	}

	// The host part is ignored - DialContext always goes to the socket.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://container-runtime/containers/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("container runtime API returned %s", resp.Status)
	}

	var listed []apiContainer
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, err
	}

	out := make(map[string]containerMetadata, len(listed))
	for _, c := range listed {
		name := ""
		if len(c.Names) > 0 {
			// The API reports names with a leading slash ("/web").
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out[c.ID] = containerMetadata{name: name, image: c.Image, state: c.State, labels: c.Labels}
	}
	return out, nil
}
