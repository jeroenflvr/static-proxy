package main

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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

	// Reverse proxy for HTTP requests destined for "myurl.com"
	myURLProxy := httputil.NewSingleHostReverseProxy(target)
	myURLProxy.Transport = &http.Transport{
		// Ignore SSL certificate errors when connecting to the target
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Reverse proxy for all other HTTP requests using upstream proxy
	defaultProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Do not modify the URL: the upstream proxy will handle it.
		},
		Transport: &http.Transport{
			// Use the upstream proxy for requests
			Proxy: http.ProxyURL(upstream),
		},
	}

	// Main handler for both HTTP and CONNECT methods
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodConnect {
			handleConnect(w, req, target, upstream)
			return
		}

		// For HTTP requests, route based on the Host header
		if strings.EqualFold(req.Host, "myurl.com") {
			myURLProxy.ServeHTTP(w, req)
		} else {
			defaultProxy.ServeHTTP(w, req)
		}
	})

	log.Println("Starting proxy server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// handleConnect handles HTTPS tunneling requests.
func handleConnect(w http.ResponseWriter, req *http.Request, target *url.URL, upstream *url.URL) {
	// Determine if the CONNECT request is for "myurl.com"
	if strings.HasPrefix(req.Host, "myurl.com") {
		// Directly connect to the target IP on port 443.
		targetConn, err := net.Dial("tcp", "30.20.55.43:443")
		if err != nil {
			http.Error(w, "Error connecting to target", http.StatusServiceUnavailable)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
			targetConn.Close()
			return
		}
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			targetConn.Close()
			return
		}
		// Signal that the tunnel is established
		_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			clientConn.Close()
			targetConn.Close()
			return
		}
		// Tunnel data between client and target
		go transfer(targetConn, clientConn)
		go transfer(clientConn, targetConn)
	} else {
		// For other hosts, forward the CONNECT request to the upstream proxy.
		upConn, err := net.Dial("tcp", upstream.Host)
		if err != nil {
			http.Error(w, "Error connecting to upstream proxy", http.StatusServiceUnavailable)
			return
		}
		// Forward the CONNECT request as-is to the upstream proxy
		_, err = upConn.Write([]byte(req.Method + " " + req.Host + " " + req.Proto + "\r\n"))
		if err != nil {
			upConn.Close()
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		// Write out all headers from the original request
		req.Header.Write(upConn)
		_, err = upConn.Write([]byte("\r\n"))
		if err != nil {
			upConn.Close()
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
			upConn.Close()
			return
		}
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			upConn.Close()
			return
		}
		// Relay data between client and the upstream proxy
		go transfer(upConn, clientConn)
		go transfer(clientConn, upConn)
	}
}

// transfer pipes data between two connections.
func transfer(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}
