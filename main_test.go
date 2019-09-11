package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const (
	testImageURL200 = "http://www.test.com/valid.jpg"
	testImageURL404 = "http://www.test.com/bogus.jpg"
)

const (
	testImagePathValid   = "testing/valid.jpg"
	testImagePathInvalid = "testing/bogus.jpg"
)

// create a client for mocking requests
func mockHTTPClient(client http.Client, handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)

	cli := client

	cli.Transport = &http.Transport{
		DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
			return net.Dial(network, s.Listener.Addr().String())
		},
	}

	return &cli, s.Close
}

// create the handler for mock requests
func mockHandlerFunc() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid.jpg":
			http.ServeFile(w, r, "./testing/valid.jpg")
		case "/slow":
			time.Sleep(10 * time.Second)
			http.ServeFile(w, r, "./testing/valid.jpg")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

var testClient *http.Client

func TestMain(m *testing.M) {
	// setup
	var sClose func()
	testClient, sClose = mockHTTPClient(*newClient(defaultTimeout), mockHandlerFunc())

	// run tests
	res := m.Run()

	// cleanup and exit
	sClose()
	os.Exit(res)
}
