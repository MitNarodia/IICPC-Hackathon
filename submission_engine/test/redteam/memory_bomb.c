#include <stdlib.h>
#include <string.h>

int main(void) {
    while (1) {
        void *p = malloc(16 * 1024 * 1024);
        if (!p) return 0;
        memset(p, 0xff, 16 * 1024 * 1024);
    }
}
