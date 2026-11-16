// Explicit syscall-level integration fixture for the eBPF soak.
// Compiles on Linux and never calls a model provider or reads other files.
// Unlike coreutils touch, the default performs exactly one SYS_openat create.
// The soak-repeat mode keeps short-lived forked children observable across
// bounded HTTP polling windows while still requiring real kernel evidence.
#define _GNU_SOURCE
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <sys/syscall.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if ((argc != 2 && argc != 3) || argv[1][0] != '/' ||
        (argc == 3 && strcmp(argv[2], "--soak-repeat") != 0)) {
        fputs("expected absolute marker path [--soak-repeat]\n", stderr);
        return 2;
    }
    const int attempts = argc == 3 ? 12 : 1;
    const struct timespec pause = {.tv_sec = 0, .tv_nsec = 25000000};
    for (int i = 0; i < attempts; i++) {
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
        if (i + 1 < attempts && nanosleep(&pause, NULL) < 0) {
            perror("nanosleep");
            return 1;
        }
    }
    return 0;
}
