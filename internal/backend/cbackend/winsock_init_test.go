package cbackend

import (
	"strings"
	"testing"
)

// Winsock инициализируется лениво (WSAStartup) до первого socket()/
// getaddrinfo() - иначе на Windows оба падают с WSANOTINITIALISED.
func TestSocketCreatorsInitWinsock(t *testing.T) {
	if !strings.Contains(rechSocketTypedef, "WSAStartup(MAKEWORD(2, 2), &wsa);") {
		t.Fatal("rech_net_init без WSAStartup")
	}
	for _, fn := range []string{
		"static rech_socket_t rech_listen_tcp(long long port) {\n    rech_net_init();",
		"static rech_socket_t rech_listen_udp(long long port) {\n    rech_net_init();",
		"static rech_socket_t rech_connect(const char *host, long long port, int type) {\n    rech_net_init();",
	} {
		if !strings.Contains(socketTypeAndConnectCode, fn) {
			t.Errorf("нет rech_net_init в начале %q", strings.SplitN(fn, "\n", 2)[0])
		}
	}
}
