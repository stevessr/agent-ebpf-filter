> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/connect.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

---

# connect(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [VERSIONS](#VERSIONS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [EXAMPLES](#EXAMPLES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
connect(2) System Calls Manual connect(2)
```

## NAME         [top](#top_of_page)

```
connect - initiate a connection on a socket 
```

## LIBRARY         [top](#top_of_page)

```
Standard C library (libc, -lc) 
```

## SYNOPSIS         [top](#top_of_page)

```
#include  int connect(int sockfd, const struct sockaddr *addr, socklen_t addrlen); 
```

## DESCRIPTION         [top](#top_of_page)

```
The connect() system call connects the socket referred to by the file descriptor sockfd to the address specified by addr. The addrlen argument specifies the size of addr. The format of the address in addr is determined by the address space of the socket sockfd; see socket(2) for further details. If the socket sockfd is of type SOCK_DGRAM, then addr is the address to which datagrams are sent by default, and the only address from which datagrams are received. If the socket is of type SOCK_STREAM or SOCK_SEQPACKET, this call attempts to make a connection to the socket that is bound to the address specified by addr. Some protocol sockets (e.g., UNIX domain stream sockets) may successfully connect() only once. Some protocol sockets (e.g., datagram sockets in the UNIX and Internet domains) may use connect() multiple times to change their association. Some protocol sockets (e.g., TCP sockets as well as datagram sockets in the UNIX and Internet domains) may dissolve the association by connecting to an address with the sa_family member of sockaddr set to AF_UNSPEC; thereafter, the socket can be connected to another address. (AF_UNSPEC is supported since Linux 2.2.) 
```

## RETURN VALUE         [top](#top_of_page)

```
If the connection or binding succeeds, zero is returned. On error, -1 is returned, and errno is set to indicate the error. 
```

## ERRORS         [top](#top_of_page)

```
The following are general socket errors only. There may be other domain-specific error codes. EACCES For UNIX domain sockets, which are identified by pathname: Write permission is denied on the socket file, or search permission is denied for one of the directories in the path prefix. (See also path_resolution(7).) EACCES EPERM The user tried to connect to a broadcast address without having the socket broadcast flag enabled or the connection request failed because of a local firewall rule. EACCES It can also be returned if an SELinux policy denied a connection (for example, if there is a policy saying that an HTTP proxy can only connect to ports associated with HTTP servers, and the proxy tries to connect to a different port). EADDRINUSE Local address is already in use. EADDRNOTAVAIL (Internet domain sockets) The socket referred to by sockfd had not previously been bound to an address and, upon attempting to bind it to an ephemeral port, it was determined that all port numbers in the ephemeral port range are currently in use. See the discussion of /proc/sys/net/ipv4/ip_local_port_range in ip(7). EAFNOSUPPORT The passed address didn't have the correct address family in its sa_family field. EAGAIN For nonblocking UNIX domain sockets, the socket is nonblocking, and the connection cannot be completed immediately. For other socket families, there are insufficient entries in the routing cache. EALREADY The socket is nonblocking and a previous connection attempt has not yet been completed. EBADF sockfd is not a valid open file descriptor. ECONNREFUSED A connect() on a stream socket found no one listening on the remote address. EFAULT The socket structure address is outside the user's address space. EINPROGRESS The socket is nonblocking and the connection cannot be completed immediately. (UNIX domain sockets failed with EAGAIN instead.) It is possible to select(2) or poll(2) for completion by selecting the socket for writing. After select(2) indicates writability, use getsockopt(2) to read the SO_ERROR option at level SOL_SOCKET to determine whether connect() completed successfully (SO_ERROR is zero) or unsuccessfully (SO_ERROR is one of the usual error codes listed here, explaining the reason for the failure). EINTR The system call was interrupted by a signal that was caught; see signal(7). EISCONN The socket is already connected. ENETUNREACH Network is unreachable. ENOTSOCK The file descriptor sockfd does not refer to a socket. EPROTOTYPE The socket type does not support the requested communications protocol. This error can occur, for example, on an attempt to connect a UNIX domain datagram socket to a stream socket. ETIMEDOUT Timeout while attempting connection. The server may be too busy to accept new connections. Note that for IP sockets the timeout may be very long when syncookies are enabled on the server. 
```

## VERSIONS         [top](#top_of_page)

```
Portable programs must ensure that addr.sun_path is a null- terminated string for AF_UNIX sockets. 
```

## STANDARDS         [top](#top_of_page)

```
POSIX.1-2024. 
```

## HISTORY         [top](#top_of_page)

```
POSIX.1-2001, SVr4, 4.2BSD. 
```

## NOTES         [top](#top_of_page)

```
If connect() fails, consider the state of the socket as unspecified. Portable applications should close the socket and create a new one for reconnecting. 
```

## EXAMPLES         [top](#top_of_page)

```
An example of the use of connect() is shown in getaddrinfo(3). 
```

## SEE ALSO         [top](#top_of_page)

```
accept(2), bind(2), getsockname(2), listen(2), socket(2), path_resolution(7), selinux(8) 
```

## COLOPHON         [top](#top_of_page)

```
Linux man-pages 6.16 2025-10-29 connect(2)
```

---

Pages that refer to this page: [telnet-probe(1)](../man1/telnet-probe.1.html),  [accept(2)](../man2/accept.2.html),  [bind(2)](../man2/bind.2.html),  [getpeername(2)](../man2/getpeername.2.html),  [io\_uring\_enter2(2)](../man2/io_uring_enter2.2.html),  [io\_uring\_enter(2)](../man2/io_uring_enter.2.html),  [listen(2)](../man2/listen.2.html),  [recv(2)](../man2/recv.2.html),  [select(2)](../man2/select.2.html),  [select\_tut(2)](../man2/select_tut.2.html),  [send(2)](../man2/send.2.html),  [shutdown(2)](../man2/shutdown.2.html),  [socket(2)](../man2/socket.2.html),  [socketcall(2)](../man2/socketcall.2.html),  [syscalls(2)](../man2/syscalls.2.html),  [write(2)](../man2/write.2.html),  [getaddrinfo(3)](../man3/getaddrinfo.3.html),  [io\_uring\_prep\_connect(3)](../man3/io_uring_prep_connect.3.html),  [ldap\_get\_option(3)](../man3/ldap_get_option.3.html),  [rtime(3)](../man3/rtime.3.html),  [sockaddr(3type)](../man3/sockaddr.3type.html),  [ldap.conf(5)](../man5/ldap.conf.5.html),  [slapd-asyncmeta(5)](../man5/slapd-asyncmeta.5.html),  [slapd-ldap(5)](../man5/slapd-ldap.5.html),  [slapd-meta(5)](../man5/slapd-meta.5.html),  [ddp(7)](../man7/ddp.7.html),  [ip(7)](../man7/ip.7.html),  [landlock(7)](../man7/landlock.7.html),  [netlink(7)](../man7/netlink.7.html),  [packet(7)](../man7/packet.7.html),  [sctp(7)](../man7/sctp.7.html),  [signal(7)](../man7/signal.7.html),  [signal-safety(7)](../man7/signal-safety.7.html),  [sock\_diag(7)](../man7/sock_diag.7.html),  [socket(7)](../man7/socket.7.html),  [tcp(7)](../man7/tcp.7.html),  [udp(7)](../man7/udp.7.html),  [unix(7)](../man7/unix.7.html),  [vsock(7)](../man7/vsock.7.html)

---

 

---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  |  |

---
