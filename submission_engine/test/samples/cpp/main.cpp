#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <cstring>
#include <string>

int main() {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    int opt = 1;
    setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons(8080);
    bind(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr));
    listen(fd, 16);
    while (true) {
        int client = accept(fd, nullptr, nullptr);
        if (client < 0) continue;
        std::string response = "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok";
        write(client, response.data(), response.size());
        close(client);
    }
}
