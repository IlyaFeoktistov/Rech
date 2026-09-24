package udp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// DatagramHelpers - "получить_от"/"отправить_в" (internal/semantic,
// registerNetworkFuncs) - recvfrom/sendto над udp-узлом, полученным
// "сетевой_узел(СетевойПротокол.UDP, "", порт)" (см. rech_listen_udp, builtins_shared.go) -
// в отличие от rech_recv/rech_send (подключённый сокет, собеседник уже
// зафиксирован connect()), здесь каждый вызов сам несёт адрес - "Дейтаграмма"
// (result "получить_от") несёт АДРЕС ОТПРАВИТЕЛЯ, чтобы ответить именно ему
// через "отправить_в", без отдельного "подключения". datagramC/dataField/
// addrField/portField - уже вычисленные вызывающей стороной (mangleType/
// mangleField - внутренняя, неэкспортированная инфраструктура cbackend,
// этот пакет от неё не зависит).
func DatagramHelpers(datagramC, dataField, addrField, portField string) string {
	return fmt.Sprintf(`static %[1]s rech_recv_from(rech_socket_t s, long long max) {
    if (max <= 0) {
        rech_panic("%[6]s");
    }
    char *buf = malloc((size_t)max + 1);
    struct sockaddr_in from;
    socklen_t fromlen = sizeof(from);
    long long n = recvfrom(s, buf, (int)max, 0, (struct sockaddr *)&from, &fromlen);
    if (n < 0) {
        free(buf);
        rech_panic("%[7]s");
    }
    %[1]s d;
    d.%[2]s = rech_text_take(buf, (size_t)n);
    d.%[3]s = rech_dup(inet_ntoa(from.sin_addr));
    d.%[4]s = (long long)ntohs(from.sin_port);
    return d;
}

static void rech_send_to(rech_socket_t s, const char *data, const char *addr, long long port) {
    struct addrinfo hints, *res;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_INET;
    hints.ai_socktype = SOCK_DGRAM;
    char portbuf[16];
    snprintf(portbuf, sizeof(portbuf), "%%lld", port);
    if (getaddrinfo(addr, portbuf, &hints, &res) != 0) {
        rech_panic("%[8]s");
    }
    long long n = sendto(s, data, (int)strlen(data), 0, res->ai_addr, (int)res->ai_addrlen);
    freeaddrinfo(res);
    if (n < 0) {
        rech_panic("%[5]s");
    }
}
`, datagramC, dataField, addrField, portField,
		rtmsg.SocketSendFailed, rtmsg.SocketRecvInvalidMax, rtmsg.SocketRecvFailed, rtmsg.SocketResolveFailed)
}
