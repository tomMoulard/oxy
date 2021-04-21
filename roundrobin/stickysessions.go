package roundrobin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
)

// CookieOptions has all the options one would like to set on the affinity cookie
type CookieOptions struct {
	HTTPOnly bool
	Secure   bool

	Path    string
	Domain  string
	Expires time.Time

	MaxAge   int
	SameSite http.SameSite
}

// StickySession is a mixin for load balancers that implements layer 7 (http cookie) session affinity
type StickySession struct {
	cookieName string
	options    CookieOptions
}

// NewStickySession creates a new StickySession
func NewStickySession(cookieName string) *StickySession {
	return &StickySession{cookieName: cookieName}
}

// NewStickySessionWithOptions creates a new StickySession whilst allowing for options to
// shape its affinity cookie such as "httpOnly" or "secure"
func NewStickySessionWithOptions(cookieName string, options CookieOptions) *StickySession {
	return &StickySession{cookieName: cookieName, options: options}
}

// GetBackend returns the backend URL stored in the sticky cookie, iff the backend is still in the valid list of servers.
func (s *StickySession) GetBackend(req *http.Request, servers []*url.URL) (*url.URL, bool, error) {
	cookie, err := req.Cookie(s.cookieName)
	switch err {
	case nil:
	case http.ErrNoCookie:
		return nil, false, nil
	default:
		return nil, false, err
	}

	serverURL := s.getBackendURL(cookie.Value, servers)
	return serverURL, serverURL != nil, nil
}

func getCleanServerURL(serverURL *url.URL) *url.URL {
	return &url.URL{
		Scheme: serverURL.Scheme,
		Host:   serverURL.Host,
		Path:   serverURL.Path,
	}
}

// StickBackend creates and sets the cookie
func (s *StickySession) StickBackend(backend *url.URL, w *http.ResponseWriter) {
	opt := s.options

	cp := "/"
	if opt.Path != "" {
		cp = opt.Path
	}

	cookie := &http.Cookie{
		Name:     s.cookieName,
		Value:    hash(getCleanServerURL(backend).String()),
		Path:     cp,
		Domain:   opt.Domain,
		Expires:  opt.Expires,
		MaxAge:   opt.MaxAge,
		Secure:   opt.Secure,
		HttpOnly: opt.HTTPOnly,
		SameSite: opt.SameSite,
	}
	http.SetCookie(*w, cookie)
}

func (s *StickySession) getBackendURL(needle string, haystack []*url.URL) *url.URL {
	if len(haystack) == 0 {
		return nil
	}

	if strings.Contains(needle, "://") {
		// Honour old cookies which have URLs instead of hash
		needleURL, err := url.Parse(needle)
		if err != nil {
			return nil
		}
		for _, serverURL := range haystack {
			if sameURL(needleURL, serverURL) {
				return serverURL
			}
		}

		return nil
	}

	for _, serverURL := range haystack {
		// Copy serverURL and remove user info that we don't expectedIsAlive in the
		// needle/haystack comparison
		if needle == hash(getCleanServerURL(serverURL).String()) {
			return serverURL
		}
	}

	return nil
}

func hash(input string) string {
	return fmt.Sprintf("%x", fnv1a.HashString64(input))
}
