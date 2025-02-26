package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// Target server for "myurl.com"
	target, err := url.Parse("https://30.20.55.43")
	if err != nil {
		log.Fatalf("failed to parse target URL: %v", err)
	}

	// Upstream proxy for all other requests (update with your proxy URL)
	upstream, err := url.Parse("http://your.upstream.proxy:8080")
	if err != nil {
		log.Fatalf("failed to parse upstream proxy URL: %v", err)
	}

	// Reverse proxy for "myurl.com"
	myURLProxy := httputil.NewSingleHostReverseProxy(target)
	// Set a custom transport to ignore SSL errors
	myURLProxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Reverse proxy for all other requests: forwarding via an upstream proxy.
	// Here, we leave the request as-is; the Transport handles the proxy routing.
	defaultProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// No modifications here: the incoming URL remains unchanged.
		},
		Transport: &http.Transport{
			// Set the proxy function to use the upstream proxy.
			Proxy: http.ProxyURL(upstream),
		},
	}

	// Main handler checks the Host and routes the request accordingly.
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Host == "myurl.com" {
			myURLProxy.ServeHTTP(w, req)
		} else {
			defaultProxy.ServeHTTP(w, req)
		}
	})

	log.Println("Starting reverse proxy on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
