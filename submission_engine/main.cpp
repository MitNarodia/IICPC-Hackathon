#include <arpa/inet.h>
#include <unistd.h>
#include <cstring>
#include <iostream>

int main() {
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);

    int opt = 1;
    setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons(8081);

    bind(server_fd, (sockaddr*)&addr, sizeof(addr));
    listen(server_fd, 16);

    std::cout << "HTTP server listening on 8081" << std::endl;

    while (true) {
        int client = accept(server_fd, nullptr, nullptr);
        if (client < 0) continue;

        char buffer[1024];
        read(client, buffer, sizeof(buffer));

        const char* response =
            "HTTP/1.1 200 OK\r\n"
            "Content-Type: text/plain\r\n"
            "Content-Length: 2\r\n"
            "\r\n"
            "OK";

        write(client, response, strlen(response));
        close(client);
    }
}
