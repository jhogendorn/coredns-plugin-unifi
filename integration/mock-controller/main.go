// mock-controller serves deterministic UniFi Controller API responses
// for integration testing. It uses unpoller/unifi types and API path
// constants for realistic responses.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/unpoller/unifi"
)

func convertPathToRegexPattern(s string) string {
	tmp := strings.ReplaceAll(strings.ReplaceAll(s, "%s", "[^/]+"), "%d", "[0-9]+")
	return fmt.Sprintf("(%s)?%s", unifi.APIPrefixNew, tmp)
}

var (
	loginPath    = regexp.MustCompile(convertPathToRegexPattern(unifi.APILoginPath))
	loginPathNew = regexp.MustCompile(convertPathToRegexPattern(unifi.APILoginPathNew))
	statusPath   = regexp.MustCompile(convertPathToRegexPattern(unifi.APIStatusPath))
	sitesPath    = regexp.MustCompile(convertPathToRegexPattern(unifi.APISiteList))
	clientsPath  = regexp.MustCompile(convertPathToRegexPattern(unifi.APIClientPath))
	networksPath = regexp.MustCompile(convertPathToRegexPattern(unifi.APINetworkPath))
)

type dataResponse struct {
	Data any `json:"data"`
}

type metaResponse struct {
	Meta any `json:"meta"`
}

func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func testSites() []*unifi.Site {
	return []*unifi.Site{
		{ID: "site1", Name: "default", Desc: "Default"},
	}
}

func testClients() []*unifi.Client {
	return []*unifi.Client{
		{ID: "client1", Name: "desktop", Hostname: "desktop-dhcp", IP: "192.168.1.10", Mac: "00:11:22:33:44:01", NetworkID: "net1"},
		{ID: "client2", Name: "laptop", Hostname: "laptop-dhcp", IP: "192.168.1.11", Mac: "00:11:22:33:44:02", NetworkID: "net1"},
		{ID: "client3", Name: "", Hostname: "phone-dhcp", IP: "192.168.1.12", Mac: "00:11:22:33:44:03", NetworkID: "net1"},
		{ID: "client4", Name: "iot-sensor", Hostname: "", IP: "10.0.0.50", Mac: "00:11:22:33:44:04", NetworkID: "net2"},
		{ID: "client5", Name: "", Hostname: "", IP: "192.168.1.99", Mac: "00:11:22:33:44:05", NetworkID: "net1"},
	}
}

func testNetworks() []unifi.Network {
	return []unifi.Network{
		{ID: "net1", Name: "LAN", DomainName: "home.lan", IPSubnet: "192.168.1.0/24", Purpose: "corporate"},
		{ID: "net2", Name: "IoT", DomainName: "iot.lan", IPSubnet: "10.0.0.0/24", Purpose: "corporate"},
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	log.Printf("%s %s", r.Method, path)

	switch {
	case loginPath.MatchString(path), loginPathNew.MatchString(path):
		jsonResponse(w, dataResponse{Data: nil})

	case statusPath.MatchString(path):
		jsonResponse(w, metaResponse{
			Meta: unifi.ServerStatus{
				Up:            unifi.FlexBool{Val: true, Txt: "true"},
				ServerVersion: "7.0.0",
				UUID:          "mock-controller-uuid",
			},
		})

	case sitesPath.MatchString(path):
		jsonResponse(w, dataResponse{Data: testSites()})

	case clientsPath.MatchString(path):
		jsonResponse(w, dataResponse{Data: testClients()})

	case networksPath.MatchString(path):
		jsonResponse(w, dataResponse{Data: testNetworks()})

	default:
		log.Printf("404: %s", path)
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8443"
	}

	http.HandleFunc("/", handler)
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Mock UniFi controller listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
