> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/write.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

---

# write(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [BUGS](#BUGS) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
write(2) System Calls Manual write(2)
```

## NAME         [top](#top_of_page)

```
write - write to a file descriptor 
```

## LIBRARY         [top](#top_of_page)

```
Standard C library (libc, -lc) 
```

## SYNOPSIS         [top](#top_of_page)

```
#include  ssize_t write(size_t count; int fd, const void buf[count], size_t count); 
```

## DESCRIPTION         [top](#top_of_page)

```
write() writes up to count bytes from the buffer starting at buf to the file referred to by the file descriptor fd. The number of bytes written may be less than count if, for example, there is insufficient space on the underlying physical medium, or the RLIMIT_FSIZE resource limit is encountered (see setrlimit(2)), or the call was interrupted by a signal handler after having written less than count bytes. (See also pipe(7).) For a seekable file (i.e., one to which lseek(2) may be applied, for example, a regular file) writing takes place at the file offset, and the file offset is incremented by the number of bytes actually written. If the file was open(2)ed with O_APPEND, the file offset is first set to the end of the file before writing. The adjustment of the file offset and the write operation are performed as an atomic step. POSIX requires that a read(2) that can be proved to occur after a write() has returned will return the new data. Note that not all filesystems are POSIX conforming. According to POSIX.1, if count is greater than SSIZE_MAX, the result is implementation-defined; see NOTES for the upper limit on Linux. 
```

## RETURN VALUE         [top](#top_of_page)

```
On success, the number of bytes written is returned. On error, -1 is returned, and errno is set to indicate the error. Note that a successful write() may transfer fewer than count bytes. Such partial writes can occur for various reasons; for example, because there was insufficient space on the disk device to write all of the requested bytes, or because a blocked write() to a socket, pipe, or similar was interrupted by a signal handler after it had transferred some, but before it had transferred all of the requested bytes. In the event of a partial write, the caller can make another write() call to transfer the remaining bytes. The subsequent call will either transfer further bytes or may result in an error (e.g., if the disk is now full). If count is zero and fd refers to a regular file, then write() may return a failure status if one of the errors below is detected. If no errors are detected, or error detection is not performed, 0 is returned without causing any other effect. If count is zero and fd refers to a file other than a regular file, the results are not specified. 
```

## ERRORS         [top](#top_of_page)

```
EAGAIN The file descriptor fd refers to a file other than a socket and has been marked nonblocking (O_NONBLOCK), and the write would block. See open(2) for further details on the O_NONBLOCK flag. EAGAIN or EWOULDBLOCK The file descriptor fd refers to a socket and has been marked nonblocking (O_NONBLOCK), and the write would block. POSIX.1-2001 allows either error to be returned for this case, and does not require these constants to have the same value, so a portable application should check for both possibilities. EBADF fd is not a valid file descriptor or is not open for writing. EDESTADDRREQ fd refers to a datagram socket for which a peer address has not been set using connect(2). EDQUOT The user's quota of disk blocks on the filesystem containing the file referred to by fd has been exhausted. EFAULT buf is outside your accessible address space. EFBIG An attempt was made to write a file that exceeds the implementation-defined maximum file size or the process's file size limit, or to write at a position past the maximum allowed offset. EINTR The call was interrupted by a signal before any data was written; see signal(7). EINVAL fd is attached to an object which is unsuitable for writing; or the file was opened with the O_DIRECT flag, and either the address specified in buf, the value specified in count, or the file offset is not suitably aligned. EIO A low-level I/O error occurred while modifying the inode. This error may relate to the write-back of data written by an earlier write(), which may have been issued to a different file descriptor on the same file. Since Linux 4.13, errors from write-back come with a promise that they may be reported by subsequent. write() requests, and will be reported by a subsequent fsync(2) (whether or not they were also reported by write()). An alternate cause of EIO on networked filesystems is when an advisory lock had been taken out on the file descriptor and this lock has been lost. See the Lost locks section of fcntl(2) for further details. ENOSPC The device containing the file referred to by fd has no room for the data. EPERM The operation was prevented by a file seal; see fcntl(2). EPIPE fd is connected to a pipe or socket whose reading end is closed. When this happens the writing process will also receive a SIGPIPE signal. (Thus, the write return value is seen only if the program catches, blocks or ignores this signal.) Other errors may occur, depending on the object connected to fd. 
```

## STANDARDS         [top](#top_of_page)

```
POSIX.1-2024. 
```

## HISTORY         [top](#top_of_page)

```
SVr4, 4.3BSD, POSIX.1-2001. Under SVr4 a write may be interrupted and return EINTR at any point, not just before any data is written. 
```

## NOTES         [top](#top_of_page)

```
A successful return from write() does not make any guarantee that data has been committed to disk. On some filesystems, including NFS, it does not even guarantee that space has successfully been reserved for the data. In this case, some errors might be delayed until a future write(), fsync(2), or even close(2). The only way to be sure is to call fsync(2) after you are done writing all your data. If a write() is interrupted by a signal handler before any bytes are written, then the call fails with the error EINTR; if it is interrupted after at least one byte has been written, the call succeeds, and returns the number of bytes written. On Linux, write() (and similar system calls) will transfer at most 0x7ffff000 (2,147,479,552) bytes, returning the number of bytes actually transferred. (This is true on both 32-bit and 64-bit systems.) An error return value while performing write() using direct I/O does not mean the entire write has failed. Partial data may be written and the data at the file offset on which the write() was attempted should be considered inconsistent. 
```

## BUGS         [top](#top_of_page)

```
According to POSIX.1-2008/SUSv4 Section XSI 2.9.7 ("Thread Interactions with Regular File Operations"): All of the following functions shall be atomic with respect to each other in the effects specified in POSIX.1-2008 when they operate on regular files or symbolic links: ... Among the APIs subsequently listed are write() and writev(2). And among the effects that should be atomic across threads (and processes) are updates of the file offset. However, before Linux 3.14, this was not the case: if two processes that share an open file description (see open(2)) perform a write() (or writev(2)) at the same time, then the I/O operations were not atomic with respect to updating the file offset, with the result that the blocks of data output by the two processes might (incorrectly) overlap. This problem was fixed in Linux 3.14. 
```

## SEE ALSO         [top](#top_of_page)

```
close(2), fcntl(2), fsync(2), ioctl(2), lseek(2), open(2), pwrite(2), read(2), select(2), writev(2), fwrite(3) 
```

## COLOPHON         [top](#top_of_page)

```
Linux man-pages 6.16 2025-10-29 write(2)
```

---

Pages that refer to this page: [ps(1)](../man1/ps.1.html),  [pv(1)](../man1/pv.1.html),  [strace(1)](../man1/strace.1.html),  [telnet-probe(1)](../man1/telnet-probe.1.html),  [close(2)](../man2/close.2.html),  [epoll\_ctl(2)](../man2/epoll_ctl.2.html),  [eventfd(2)](../man2/eventfd.2.html),  [fcntl\_locking(2)](../man2/fcntl_locking.2.html),  [F\_GET\_SEALS(2const)](../man2/F_GET_SEALS.2const.html),  [F\_NOTIFY(2const)](../man2/F_NOTIFY.2const.html),  [fsync(2)](../man2/fsync.2.html),  [getpeername(2)](../man2/getpeername.2.html),  [getrlimit(2)](../man2/getrlimit.2.html),  [io\_uring\_enter2(2)](../man2/io_uring_enter2.2.html),  [io\_uring\_enter(2)](../man2/io_uring_enter.2.html),  [lseek(2)](../man2/lseek.2.html),  [memfd\_create(2)](../man2/memfd_create.2.html),  [mmap(2)](../man2/mmap.2.html),  [open(2)](../man2/open.2.html),  [pipe(2)](../man2/pipe.2.html),  [pread(2)](../man2/pread.2.html),  [read(2)](../man2/read.2.html),  [readv(2)](../man2/readv.2.html),  [seccomp(2)](../man2/seccomp.2.html),  [select(2)](../man2/select.2.html),  [select\_tut(2)](../man2/select_tut.2.html),  [send(2)](../man2/send.2.html),  [sendfile(2)](../man2/sendfile.2.html),  [socket(2)](../man2/socket.2.html),  [socketpair(2)](../man2/socketpair.2.html),  [sync(2)](../man2/sync.2.html),  [syscalls(2)](../man2/syscalls.2.html),  [aio\_error(3)](../man3/aio_error.3.html),  [aio\_return(3)](../man3/aio_return.3.html),  [aio\_write(3)](../man3/aio_write.3.html),  [curs\_print(3x)](../man3/curs_print.3x.html),  [curs\_util(3x)](../man3/curs_util.3x.html),  [dbopen(3)](../man3/dbopen.3.html),  [fclose(3)](../man3/fclose.3.html),  [fflush(3)](../man3/fflush.3.html),  [fgetc(3)](../man3/fgetc.3.html),  [fopen(3)](../man3/fopen.3.html),  [fread(3)](../man3/fread.3.html),  [gets(3)](../man3/gets.3.html),  [io\_uring\_prep\_write(3)](../man3/io_uring_prep_write.3.html),  [io\_uring\_prep\_writev2(3)](../man3/io_uring_prep_writev2.3.html),  [io\_uring\_prep\_writev(3)](../man3/io_uring_prep_writev.3.html),  [libexpect(3)](../man3/libexpect.3.html),  [mkfifo(3)](../man3/mkfifo.3.html),  [mpool(3)](../man3/mpool.3.html),  [printf(3)](../man3/printf.3.html),  [puts(3)](../man3/puts.3.html),  [size\_t(3type)](../man3/size_t.3type.html),  [stdio(3)](../man3/stdio.3.html),  [wprintf(3)](../man3/wprintf.3.html),  [xdr(3)](../man3/xdr.3.html),  [xfsctl(3)](../man3/xfsctl.3.html),  [dsp56k(4)](../man4/dsp56k.4.html),  [fuse(4)](../man4/fuse.4.html),  [lirc(4)](../man4/lirc.4.html),  [st(4)](../man4/st.4.html),  [proc\_pid\_io(5)](../man5/proc_pid_io.5.html),  [proc\_sys\_kernel(5)](../man5/proc_sys_kernel.5.html),  [systemd.exec(5)](../man5/systemd.exec.5.html),  [aio(7)](../man7/aio.7.html),  [cgroups(7)](../man7/cgroups.7.html),  [cpuset(7)](../man7/cpuset.7.html),  [epoll(7)](../man7/epoll.7.html),  [fanotify(7)](../man7/fanotify.7.html),  [inode(7)](../man7/inode.7.html),  [inotify(7)](../man7/inotify.7.html),  [landlock(7)](../man7/landlock.7.html),  [pipe(7)](../man7/pipe.7.html),  [sched(7)](../man7/sched.7.html),  [signal(7)](../man7/signal.7.html),  [signal-safety(7)](../man7/signal-safety.7.html),  [socket(7)](../man7/socket.7.html),  [spufs(7)](../man7/spufs.7.html),  [tcp(7)](../man7/tcp.7.html),  [time\_namespaces(7)](../man7/time_namespaces.7.html),  [udp(7)](../man7/udp.7.html),  [user\_namespaces(7)](../man7/user_namespaces.7.html),  [vsock(7)](../man7/vsock.7.html),  [x25(7)](../man7/x25.7.html),  [fsfreeze(8)](../man8/fsfreeze.8.html),  [netsniff-ng(8)](../man8/netsniff-ng.8.html),  [wipefs(8)](../man8/wipefs.8.html),  [xfs\_io(8)](../man8/xfs_io.8.html)

---

 

---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  | [Cover of TLPI](https://man7.org/tlpi/) |

---
