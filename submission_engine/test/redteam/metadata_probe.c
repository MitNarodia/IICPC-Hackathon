#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

int main(void) {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in addr = {0};
    addr.sin_family = AF_INET;
    addr.sin_port = htons(80);
    inet_pton(AF_INET, "169.254.169.254", &addr.sin_addr);
    return connect(fd, (struct sockaddr *)&addr, sizeof(addr)) == 0 ? 1 : 0;
}
