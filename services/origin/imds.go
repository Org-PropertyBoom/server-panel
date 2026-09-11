package origin

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// IMDS reads this instance's public IPv4 addresses (including Elastic IPs) from EC2
// instance metadata using IMDSv2:
//
//	PUT /latest/api/token                                        → session token
//	GET /latest/meta-data/network/interfaces/macs/               → one MAC per line
//	GET /latest/meta-data/network/interfaces/macs/<mac>/public-ipv4s
//
// Short timeouts, no proxy, and callers treat any error as "nothing discovered".
type IMDS struct {
	BaseURL string
	Client  *http.Client
}

const (
	imdsTokenPath = "/latest/api/token"
	imdsMACsPath  = "/latest/meta-data/network/interfaces/macs/"
	imdsBodyLimit = 64 << 10
)

// NewIMDS returns a client for the link-local metadata endpoint. Proxy is
// explicitly nil: an HTTP_PROXY in the service env must not swallow 169.254.169.254.
func NewIMDS(timeout time.Duration) *IMDS {
	return &IMDS{
		BaseURL: "http://169.254.169.254",
		Client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:       nil,
				DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
			},
		},
	}
}

// PublicIPv4s returns every public IPv4 on every network interface of this instance.
// An interface with no public address (404) is skipped, not an error.
func (m *IMDS) PublicIPv4s(ctx context.Context) ([]string, error) {
	token, err := m.token(ctx)
	if err != nil {
		return nil, err
	}
	body, status, err := m.get(ctx, token, imdsMACsPath)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("imds: list macs: HTTP %d", status)
	}
	var out []string
	for _, line := range lines(body) {
		mac := strings.TrimSuffix(line, "/")
		if !validMAC(mac) {
			continue
		}
		b, st, err := m.get(ctx, token, imdsMACsPath+mac+"/public-ipv4s")
		if err != nil {
			return nil, err
		}
		if st == http.StatusNotFound {
			continue // this interface has no public address
		}
		if st != http.StatusOK {
			return nil, fmt.Errorf("imds: %s public-ipv4s: HTTP %d", mac, st)
		}
		for _, l := range lines(b) {
			if ip := net.ParseIP(l); ip != nil && ip.To4() != nil {
				out = append(out, ip.String())
			}
		}
	}
	return out, nil
}

func (m *IMDS) token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, m.BaseURL+imdsTokenPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, imdsBodyLimit))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imds: token: HTTP %d", resp.StatusCode)
	}
	t := strings.TrimSpace(string(b))
	if t == "" {
		return "", fmt.Errorf("imds: empty token")
	}
	return t, nil
}

func (m *IMDS) get(ctx context.Context, token, path string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.BaseURL+path, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("X-aws-ec2-metadata-token", token)
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, imdsBodyLimit))
	if err != nil {
		return "", 0, err
	}
	return string(b), resp.StatusCode, nil
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// validMAC keeps the value we splice into the next URL path to hex + colons.
func validMAC(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == ':' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
