// Explicit syscall-level integration fixture for the eBPF soak.
// Compiles on Linux and never calls a model provider or reads other files.
// Unlike coreutils touch, this always performs exactly one SYS_openat create.
#define _GNU_SOURCE
#include <fcntl.h>
#include <stdio.h>
#include <sys/syscall.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc != 2 || argv[1][0] != '/') {
        fputs("expected one absolute marker path\n", stderr);
        return 2;
    }
    int fd = (int)syscall(SYS_openat, AT_FDCWD, argv[1],
                          O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (fd < 0) {
        perror("SYS_openat");
        return 1;
    }
    if (close(fd) < 0) {
        perror("close");
        return 1;
    }
    return 0;
}
