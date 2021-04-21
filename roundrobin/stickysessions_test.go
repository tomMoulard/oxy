package roundrobin

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/testutils"
)

func TestBasic(t *testing.T) {
	a := testutils.NewResponder("a")
	b := testutils.NewResponder("b")

	defer a.Close()
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	sticky := NewStickySession("test")
	require.NotNil(t, sticky)

	lb, err := New(fwd, EnableStickySession(sticky))
	require.NoError(t, err)

	err = lb.UpsertServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)
	err = lb.UpsertServer(testutils.ParseURI(b.URL))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	client := http.DefaultClient

	for i := 0; i < 30; i++ {
		req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
		require.NoError(t, err)
		if i%3 == 0 {
			req.AddCookie(&http.Cookie{Name: "test", Value: hash(a.URL)})
		} else {
			req.AddCookie(&http.Cookie{Name: "test", Value: a.URL})
		}

		resp, err := client.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)

		require.NoError(t, err)
		assert.Equal(t, "a", string(body))
	}
}

func TestStickyCookie(t *testing.T) {
	a := testutils.NewResponder("a")
	b := testutils.NewResponder("b")

	defer a.Close()
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	sticky := NewStickySession("test")
	require.NotNil(t, sticky)

	lb, err := New(fwd, EnableStickySession(sticky))
	require.NoError(t, err)

	err = lb.UpsertServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)
	err = lb.UpsertServer(testutils.ParseURI(b.URL))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	require.NoError(t, err)

	cookie := resp.Cookies()[0]
	assert.Equal(t, "test", cookie.Name)
	assert.Equal(t, hash(a.URL), cookie.Value)
}

func TestStickyCookieWithOptions(t *testing.T) {
	a := testutils.NewResponder("a")
	b := testutils.NewResponder("b")

	defer a.Close()
	defer b.Close()

	testCases := []struct {
		desc     string
		name     string
		options  CookieOptions
		expected *http.Cookie
	}{
		{
			desc:    "no options",
			name:    "test",
			options: CookieOptions{},
			expected: &http.Cookie{
				Name:  "test",
				Value: hash(a.URL),
				Path:  "/",
				Raw:   fmt.Sprintf("test=%s; Path=/", hash(a.URL)),
			},
		},
		{
			desc: "HTTPOnly",
			name: "test",
			options: CookieOptions{
				HTTPOnly: true,
			},
			expected: &http.Cookie{
				Name:     "test",
				Value:    hash(a.URL),
				Path:     "/",
				HttpOnly: true,
				Raw:      fmt.Sprintf("test=%s; Path=/; HttpOnly", hash(a.URL)),
				Unparsed: nil,
			},
		},
		{
			desc: "Secure",
			name: "test",
			options: CookieOptions{
				Secure: true,
			},
			expected: &http.Cookie{
				Name:   "test",
				Value:  hash(a.URL),
				Path:   "/",
				Secure: true,
				Raw:    fmt.Sprintf("test=%s; Path=/; Secure", hash(a.URL)),
			},
		},
		{
			desc: "Path",
			name: "test",
			options: CookieOptions{
				Path: "/foo",
			},
			expected: &http.Cookie{
				Name:  "test",
				Value: hash(a.URL),
				Path:  "/foo",
				Raw:   fmt.Sprintf("test=%s; Path=/foo", hash(a.URL)),
			},
		},
		{
			desc: "Domain",
			name: "test",
			options: CookieOptions{
				Domain: "example.org",
			},
			expected: &http.Cookie{
				Name:   "test",
				Value:  hash(a.URL),
				Path:   "/",
				Domain: "example.org",
				Raw:    fmt.Sprintf("test=%s; Path=/; Domain=example.org", hash(a.URL)),
			},
		},
		{
			desc: "Expires",
			name: "test",
			options: CookieOptions{
				Expires: time.Date(1955, 11, 12, 1, 22, 0, 0, time.UTC),
			},
			expected: &http.Cookie{
				Name:       "test",
				Value:      hash(a.URL),
				Path:       "/",
				Expires:    time.Date(1955, 11, 12, 1, 22, 0, 0, time.UTC),
				RawExpires: "Sat, 12 Nov 1955 01:22:00 GMT",
				Raw:        fmt.Sprintf("test=%s; Path=/; Expires=Sat, 12 Nov 1955 01:22:00 GMT", hash(a.URL)),
			},
		},
		{
			desc: "MaxAge",
			name: "test",
			options: CookieOptions{
				MaxAge: -20,
			},
			expected: &http.Cookie{
				Name:   "test",
				Value:  hash(a.URL),
				Path:   "/",
				MaxAge: -1,
				Raw:    fmt.Sprintf("test=%s; Path=/; Max-Age=0", hash(a.URL)),
			},
		},
		{
			desc: "SameSite",
			name: "test",
			options: CookieOptions{
				SameSite: http.SameSiteNoneMode,
			},
			expected: &http.Cookie{
				Name:     "test",
				Value:    hash(a.URL),
				Path:     "/",
				SameSite: http.SameSiteNoneMode,
				Raw:      fmt.Sprintf("test=%s; Path=/; SameSite=None", hash(a.URL)),
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {

			fwd, err := forward.New()
			require.NoError(t, err)

			sticky := NewStickySessionWithOptions(test.name, test.options)
			require.NotNil(t, sticky)

			lb, err := New(fwd, EnableStickySession(sticky))
			require.NoError(t, err)

			err = lb.UpsertServer(testutils.ParseURI(a.URL))
			require.NoError(t, err)
			err = lb.UpsertServer(testutils.ParseURI(b.URL))
			require.NoError(t, err)

			proxy := httptest.NewServer(lb)
			defer proxy.Close()

			resp, err := http.Get(proxy.URL)
			require.NoError(t, err)

			require.Len(t, resp.Cookies(), 1)
			assert.Equal(t, test.expected, resp.Cookies()[0])
		})
	}
}

func TestRemoveRespondingServer(t *testing.T) {
	a := testutils.NewResponder("a")
	b := testutils.NewResponder("b")

	defer a.Close()
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	sticky := NewStickySession("test")
	require.NotNil(t, sticky)

	lb, err := New(fwd, EnableStickySession(sticky))
	require.NoError(t, err)

	err = lb.UpsertServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)
	err = lb.UpsertServer(testutils.ParseURI(b.URL))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	client := http.DefaultClient

	for i := 0; i < 10; i++ {
		req, errReq := http.NewRequest(http.MethodGet, proxy.URL, nil)
		require.NoError(t, errReq)

		req.AddCookie(&http.Cookie{Name: "test", Value: hash(a.URL)})

		resp, errReq := client.Do(req)
		require.NoError(t, errReq)
		defer resp.Body.Close()

		body, errReq := ioutil.ReadAll(resp.Body)
		require.NoError(t, errReq)

		assert.Equal(t, "a", string(body))
	}

	err = lb.RemoveServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)

	// Now, use the organic cookie response in our next requests.
	req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "test", Value: hash(a.URL)})
	resp, err := client.Do(req)
	require.NoError(t, err)

	assert.Equal(t, "test", resp.Cookies()[0].Name)
	assert.Equal(t, hash(b.URL), resp.Cookies()[0].Value)

	for i := 0; i < 10; i++ {
		req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)

		require.NoError(t, err)
		assert.Equal(t, "b", string(body))
	}
}

func TestRemoveAllServers(t *testing.T) {
	a := testutils.NewResponder("a")
	b := testutils.NewResponder("b")

	defer a.Close()
	defer b.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	sticky := NewStickySession("test")
	require.NotNil(t, sticky)

	lb, err := New(fwd, EnableStickySession(sticky))
	require.NoError(t, err)

	err = lb.UpsertServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)
	err = lb.UpsertServer(testutils.ParseURI(b.URL))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	client := http.DefaultClient

	for i := 0; i < 10; i++ {
		req, errReq := http.NewRequest(http.MethodGet, proxy.URL, nil)
		require.NoError(t, errReq)
		req.AddCookie(&http.Cookie{Name: "test", Value: hash(a.URL)})

		resp, errReq := client.Do(req)
		require.NoError(t, errReq)
		defer resp.Body.Close()

		body, errReq := ioutil.ReadAll(resp.Body)
		require.NoError(t, errReq)

		assert.Equal(t, "a", string(body))
	}

	err = lb.RemoveServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)
	err = lb.RemoveServer(testutils.ParseURI(b.URL))
	require.NoError(t, err)

	// Now, use the organic cookie response in our next requests.
	req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "test", Value: hash(a.URL)})
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestBadCookieVal(t *testing.T) {
	a := testutils.NewResponder("a")

	defer a.Close()

	fwd, err := forward.New()
	require.NoError(t, err)

	sticky := NewStickySession("test")
	require.NotNil(t, sticky)

	lb, err := New(fwd, EnableStickySession(sticky))
	require.NoError(t, err)

	err = lb.UpsertServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)

	proxy := httptest.NewServer(lb)
	defer proxy.Close()

	client := http.DefaultClient

	req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "test", Value: "This is a patently invalid url!  You can't parse it!  :-)"})

	resp, err := client.Do(req)
	require.NoError(t, err)

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "a", string(body))

	// Now, cycle off the good server to cause an error
	err = lb.RemoveServer(testutils.ParseURI(a.URL))
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)

	_, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func BenchmarkStickysessions(b *testing.B) {
	s := NewStickySession("sticky")

	urlsn := 200
	urlsh := make([]string, urlsn)
	urls := make([]*url.URL, urlsn)
	for i := 0; i < urlsn; i++ {
		urls[i] = &url.URL{Scheme: "http", Host: fmt.Sprintf("10.10.10.%d", i), Path: "/"}
		urlsh[i] = hash(urls[i].String())
	}

	numCPU := runtime.NumCPU()
	wg := sync.WaitGroup{}

	b.ResetTimer()

	b.Run(
		"getBackendURL",
		func(b *testing.B) {
			wg.Add(numCPU)
			for i := 0; i < numCPU; i++ {
				go func(bN int) {
					for n := 0; n < bN; n++ {
						s.getBackendURL(urlsh[rand.Intn(urlsn)], urls)
					}
					wg.Done()
				}(b.N)
			}
			wg.Wait()
		},
	)
}

func TestStickySession_getBackendURL(t *testing.T) {
	s := &StickySession{
		cookieName: "sticky",
	}

	tests := []struct {
		name        string
		haystack    []*url.URL
		needle      string
		expectedURL *url.URL
		user        *url.Userinfo
	}{
		{
			name:     "Empty haystack",
			haystack: []*url.URL{},
			needle:   "http://10.10.10.10/",
		},
		{
			name:        "Simple",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:      "http://10.10.10.10/",
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/"},
		},
		{
			name:     "Host no match for needle",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:   "http://10.10.10.42/",
		},
		{
			name:     "Scheme no match for needle",
			haystack: []*url.URL{{Scheme: "https", Host: "10.10.10.10", Path: "/"}},
			needle:   "http://10.10.10.10/",
		},
		{
			name:     "Path no match for needle",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/foo/bar"}},
			needle:   "http://10.10.10.10/",
		},
		{
			name:        "With user in haystack but not in needle",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")}},
			needle:      "http://10.10.10.10/",
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")},
		},
		{
			name:        "With user in haystack, and in needle",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")}},
			needle:      "http://John%20Doe@10.10.10.10/",
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")},
		},
		{
			name:        "With user in needle but not in haystack",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:      "http://John%20Doe@10.10.10.10/",
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/"},
		},
		{
			name: "With multiple URLs in Haystack",
			haystack: []*url.URL{
				{Scheme: "http", Host: "10.10.10.10", Path: "/"},
				{Scheme: "https", Host: "10.10.10.42", Path: "/"},
			},
			needle:      "http://10.10.10.10/",
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/"},
		},
		{
			name: "With multiple URL in Haystack, not matched for needle",
			haystack: []*url.URL{
				{Scheme: "http", Host: "10.10.10.10", Path: "/"},
				{Scheme: "https", Host: "10.10.10.42", Path: "/"},
			},
			needle: "http://10.10.10.10/foo/bar",
		},
		{
			name:     "Empty haystack, hashed",
			haystack: []*url.URL{},
			needle:   hash("http://10.10.10.10/"),
		},
		{
			name:        "Simple, hashed",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:      hash("http://10.10.10.10/"),
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/"},
		},
		{
			name:     "Host no match for needle, hashed",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:   hash("http://10.10.10.42/"),
		},
		{
			name:     "Scheme no match for needle, hashed",
			haystack: []*url.URL{{Scheme: "https", Host: "10.10.10.10", Path: "/"}},
			needle:   hash("http://10.10.10.10/"),
		},
		{
			name:     "Path no match for needle, hashed",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/foo/bar"}},
			needle:   hash("http://10.10.10.10/"),
		},
		{
			name:        "With user in haystack but not in needle, hashed",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")}},
			needle:      hash("http://10.10.10.10/"),
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")},
		},
		{
			name:     "With user in haystack, and in needle, hashed",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/", User: url.User("John Doe")}},
			needle:   hash("http://John%20Doe@10.10.10.10/"),
		},
		{
			name:     "With user in needle but not in haystack, hashed",
			haystack: []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/"}},
			needle:   hash("http://John%20Doe@10.10.10.10/"),
		},
		{
			name: "With multiple URLs in Haystack, hashed",
			haystack: []*url.URL{
				{Scheme: "http", Host: "10.10.10.10", Path: "/"},
				{Scheme: "https", Host: "10.10.10.42", Path: "/"},
			},
			needle:      hash("http://10.10.10.10/"),
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/"},
		},
		{
			name: "With multiple URL in Haystack, not matched for needle, hashed",
			haystack: []*url.URL{
				{Scheme: "http", Host: "10.10.10.10", Path: "/"},
				{Scheme: "https", Host: "10.10.10.42", Path: "/"},
			},
			needle: hash("http://10.10.10.10/foo/bar"),
		},
		{
			name:        "Filtering unused parameters before hashing",
			haystack:    []*url.URL{{Scheme: "http", Host: "10.10.10.10", Path: "/", ForceQuery: true}},
			needle:      hash("http://10.10.10.10/"),
			expectedURL: &url.URL{Scheme: "http", Host: "10.10.10.10", Path: "/", ForceQuery: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			backendURL := s.getBackendURL(tt.needle, tt.haystack)
			assert.Equal(t, tt.expectedURL, backendURL)
		})
	}
}
