package library

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// safeFetchClient is a hardened *http.Client used ONLY for fetching
// user-supplied URLs (the page a user adds to their library / opens in the
// reader). It must NOT be used for trusted internal calls (e.g. the NLP
// service), whose hosts deliberately resolve to private IPs.
//
// Protections:
//   - the dialer's Control hook re-checks the resolved IP at connect time and
//     rejects loopback / private / link-local / unspecified / CGNAT / ULA
//     addresses. Checking at dial time (rather than a pre-flight LookupIP)
//     closes the DNS-rebinding TOCTOU window.
//   - redirects are not followed, so a public URL cannot 302 into the VPC.
//   - only http/https schemes are accepted (validated by the caller).
var safeFetchClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: safeDialControl,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
	},
}

// errBlockedAddress is returned when a fetch resolves to a non-public address.
var errBlockedAddress = errors.New("refusing to fetch a non-public address")

// safeDialControl rejects connections to addresses that should never be
// reachable by a user-controlled fetch.
func safeDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errBlockedAddress
	}
	if isBlockedIP(ip) {
		return errBlockedAddress
	}
	return nil
}

// isBlockedIP reports whether ip is in a range that a user-supplied fetch must
// not reach: loopback, private (RFC1918 / ULA), link-local (incl. the cloud
// metadata range 169.254.0.0/16 and the Fargate task-role endpoint
// 169.254.170.2), unspecified, and the CGNAT range 100.64.0.0/10.
func isBlockedIP(ip net.IP) bool {
	// Normalize IPv4-mapped IPv6 (::ffff:a.b.c.d) to its IPv4 form so the
	// checks below catch e.g. ::ffff:169.254.169.254.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// CGNAT 100.64.0.0/10 (shared address space) is not covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// fetchUserURL performs a GET against a user-supplied URL using the hardened
// client. It validates the scheme and applies the SSRF guard via the dialer.
func fetchUserURL(ctx context.Context, rawURL, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, errBlockedAddress
	}
	req.Header.Set("User-Agent", userAgent)
	return safeFetchClient.Do(req)
}
