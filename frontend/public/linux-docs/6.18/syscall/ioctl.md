> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/ioctl.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

---

# ioctl(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [VERSIONS](#VERSIONS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
ioctl(2) System Calls Manual ioctl(2)
```

## NAME         [top](#top_of_page)

```
ioctl - control device 
```

## LIBRARY         [top](#top_of_page)

```
Standard C library (libc, -lc) 
```

## SYNOPSIS         [top](#top_of_page)

```
#include  int ioctl(int fd, unsigned long op, ...); /* glibc, BSD */ int ioctl(int fd, int op, ...); /* musl, other UNIX */ 
```

## DESCRIPTION         [top](#top_of_page)

```
The ioctl() system call manipulates the underlying device parameters of special files. In particular, many operating characteristics of character special files (e.g., terminals) may be controlled with ioctl() operations. The argument fd must be an open file descriptor. The second argument is a device-dependent operation code. The third argument is an untyped pointer to memory. It's traditionally char *argp (from the days before void * was valid C), and will be so named for this discussion. An ioctl() op has encoded in it whether the argument is an in parameter or out parameter, and the size of the argument argp in bytes. Macros and defines used in specifying an ioctl() op are located in the file . See NOTES. 
```

## RETURN VALUE         [top](#top_of_page)

```
Usually, on success zero is returned. A few ioctl() operations use the return value as an output parameter and return a nonnegative value on success. On error, -1 is returned, and errno is set to indicate the error. 
```

## ERRORS         [top](#top_of_page)

```
EBADF fd is not a valid file descriptor. EFAULT argp references an inaccessible memory area. EINVAL op or argp is not valid. ENOTTY fd is not associated with a character special device. ENOTTY The specified operation does not apply to the kind of object that the file descriptor fd references. 
```

## VERSIONS         [top](#top_of_page)

```
Arguments, returns, and semantics of ioctl() vary according to the device driver in question (the call is used as a catch-all for operations that don't cleanly fit the UNIX stream I/O model). 
```

## STANDARDS         [top](#top_of_page)

```
None. 
```

## HISTORY         [top](#top_of_page)

```
Version 7 AT&T UNIX has ioctl(int fd, int op, struct sgttyb *argp); (where struct sgttyb has historically been used by stty(2) and gtty(2), and is polymorphic by operation type (like a void * would be, if it had been available)). SysIII documents arg without a type at all. 4.3BSD has ioctl(int d, unsigned long op, char *argp); (with char * similarly in for void *). SysVr4 has int ioctl(int fd, int op, ... /* arg */); 
```

## NOTES         [top](#top_of_page)

```
In order to use this call, one needs an open file descriptor. Often the open(2) call has unwanted side effects, that can be avoided under Linux by giving it the O_NONBLOCK flag. ioctl structure Ioctl op values are 32-bit constants. In principle these constants are completely arbitrary, but people have tried to build some structure into them. The old Linux situation was that of mostly 16-bit constants, where the last byte is a serial number, and the preceding byte(s) give a type indicating the driver. Sometimes the major number was used: 0x03 for the HDIO_* ioctls, 0x06 for the LP* ioctls. And sometimes one or more ASCII letters were used. For example, TCGETS has value 0x00005401, with 0x54 = 'T' indicating the terminal driver, and CYGETTIMEOUT has value 0x00435906, with 0x43 0x59 = 'C' 'Y' indicating the cyclades driver. Later (0.98p5) some more information was built into the number. One has 2 direction bits (00: none, 01: write, 10: read, 11: read/write) followed by 14 size bits (giving the size of the argument), followed by an 8-bit type (collecting the ioctls in groups for a common purpose or a common driver), and an 8-bit serial number. The macros describing this structure live in  and are _IO(type,nr) and {_IOR,_IOW,_IOWR}(type,nr,size). They use sizeof(size) so that size is a misnomer here: this third argument is a data type. Note that the size bits are very unreliable: in lots of cases they are wrong, either because of buggy macros using sizeof(sizeof(struct)), or because of legacy values. Thus, it seems that the new structure only gave disadvantages: it does not help in checking, but it causes varying values for the various architectures. 
```

## SEE ALSO         [top](#top_of_page)

```
execve(2), fcntl(2), ioctl_console(2), ioctl_fat(2), ioctl_fs(2), ioctl_fsmap(2), ioctl_nsfs(2), ioctl_tty(2), ioctl_userfaultfd(2), ioctl_eventpoll(2), open(2), sd(4), tty(4) 
```

## COLOPHON         [top](#top_of_page)

```
Linux man-pages 6.16 2025-09-21 ioctl(2)
```

---

Pages that refer to this page: [apropos(1)](../man1/apropos.1.html),  [man(1)](../man1/man.1.html),  [pipesz(1)](../man1/pipesz.1.html),  [setterm(1)](../man1/setterm.1.html),  [whatis(1)](../man1/whatis.1.html),  [FAT\_IOCTL\_GET\_VOLUME\_ID(2const)](../man2/FAT_IOCTL_GET_VOLUME_ID.2const.html),  [FAT\_IOCTL\_SET\_ATTRIBUTES(2const)](../man2/FAT_IOCTL_SET_ATTRIBUTES.2const.html),  [FICLONE(2const)](../man2/FICLONE.2const.html),  [FIDEDUPERANGE(2const)](../man2/FIDEDUPERANGE.2const.html),  [FIONREAD(2const)](../man2/FIONREAD.2const.html),  [FS\_IOC\_SETFLAGS(2const)](../man2/FS_IOC_SETFLAGS.2const.html),  [FS\_IOC\_SETFSLABEL(2const)](../man2/FS_IOC_SETFSLABEL.2const.html),  [getsockopt(2)](../man2/getsockopt.2.html),  [ioctl\_console(2)](../man2/ioctl_console.2.html),  [ioctl\_eventpoll(2)](../man2/ioctl_eventpoll.2.html),  [ioctl\_fat(2)](../man2/ioctl_fat.2.html),  [ioctl\_fs(2)](../man2/ioctl_fs.2.html),  [ioctl\_fsmap(2)](../man2/ioctl_fsmap.2.html),  [ioctl\_kd(2)](../man2/ioctl_kd.2.html),  [ioctl\_nsfs(2)](../man2/ioctl_nsfs.2.html),  [ioctl\_pipe(2)](../man2/ioctl_pipe.2.html),  [ioctl\_tty(2)](../man2/ioctl_tty.2.html),  [ioctl\_userfaultfd(2)](../man2/ioctl_userfaultfd.2.html),  [ioctl\_vt(2)](../man2/ioctl_vt.2.html),  [ioctl\_xfs\_ag\_geometry(2)](../man2/ioctl_xfs_ag_geometry.2.html),  [ioctl\_xfs\_bulkstat(2)](../man2/ioctl_xfs_bulkstat.2.html),  [ioctl\_xfs\_commit\_range(2)](../man2/ioctl_xfs_commit_range.2.html),  [ioctl\_xfs\_exchange\_range(2)](../man2/ioctl_xfs_exchange_range.2.html),  [ioctl\_xfs\_fsbulkstat(2)](../man2/ioctl_xfs_fsbulkstat.2.html),  [ioctl\_xfs\_fscounts(2)](../man2/ioctl_xfs_fscounts.2.html),  [ioctl\_xfs\_fsgeometry(2)](../man2/ioctl_xfs_fsgeometry.2.html),  [ioctl\_xfs\_fsgetxattr(2)](../man2/ioctl_xfs_fsgetxattr.2.html),  [ioctl\_xfs\_fsinumbers(2)](../man2/ioctl_xfs_fsinumbers.2.html),  [ioctl\_xfs\_getbmapx(2)](../man2/ioctl_xfs_getbmapx.2.html),  [ioctl\_xfs\_getparents(2)](../man2/ioctl_xfs_getparents.2.html),  [ioctl\_xfs\_getresblks(2)](../man2/ioctl_xfs_getresblks.2.html),  [ioctl\_xfs\_goingdown(2)](../man2/ioctl_xfs_goingdown.2.html),  [ioctl\_xfs\_inumbers(2)](../man2/ioctl_xfs_inumbers.2.html),  [ioctl\_xfs\_rtgroup\_geometry(2)](../man2/ioctl_xfs_rtgroup_geometry.2.html),  [ioctl\_xfs\_scrub\_metadata(2)](../man2/ioctl_xfs_scrub_metadata.2.html),  [ioctl\_xfs\_scrubv\_metadata(2)](../man2/ioctl_xfs_scrubv_metadata.2.html),  [io\_uring\_enter2(2)](../man2/io_uring_enter2.2.html),  [io\_uring\_enter(2)](../man2/io_uring_enter.2.html),  [NS\_GET\_NSTYPE(2const)](../man2/NS_GET_NSTYPE.2const.html),  [NS\_GET\_OWNER\_UID(2const)](../man2/NS_GET_OWNER_UID.2const.html),  [NS\_GET\_USERNS(2const)](../man2/NS_GET_USERNS.2const.html),  [open(2)](../man2/open.2.html),  [PAGEMAP\_SCAN(2const)](../man2/PAGEMAP_SCAN.2const.html),  [perf\_event\_open(2)](../man2/perf_event_open.2.html),  [PR\_SET\_TAGGED\_ADDR\_CTRL(2const)](../man2/PR_SET_TAGGED_ADDR_CTRL.2const.html),  [read(2)](../man2/read.2.html),  [seccomp\_unotify(2)](../man2/seccomp_unotify.2.html),  [socket(2)](../man2/socket.2.html),  [syscalls(2)](../man2/syscalls.2.html),  [TCSBRK(2const)](../man2/TCSBRK.2const.html),  [TCSETS(2const)](../man2/TCSETS.2const.html),  [TCXONC(2const)](../man2/TCXONC.2const.html),  [timerfd\_create(2)](../man2/timerfd_create.2.html),  [TIOCCONS(2const)](../man2/TIOCCONS.2const.html),  [TIOCEXCL(2const)](../man2/TIOCEXCL.2const.html),  [TIOCLINUX(2const)](../man2/TIOCLINUX.2const.html),  [TIOCMSET(2const)](../man2/TIOCMSET.2const.html),  [TIOCPKT(2const)](../man2/TIOCPKT.2const.html),  [TIOCSCTTY(2const)](../man2/TIOCSCTTY.2const.html),  [TIOCSETD(2const)](../man2/TIOCSETD.2const.html),  [TIOCSLCKTRMIOS(2const)](../man2/TIOCSLCKTRMIOS.2const.html),  [TIOCSPGRP(2const)](../man2/TIOCSPGRP.2const.html),  [TIOCSSOFTCAR(2const)](../man2/TIOCSSOFTCAR.2const.html),  [TIOCSTI(2const)](../man2/TIOCSTI.2const.html),  [TIOCSWINSZ(2const)](../man2/TIOCSWINSZ.2const.html),  [TIOCTTYGSTRUCT(2const)](../man2/TIOCTTYGSTRUCT.2const.html),  [UFFDIO\_API(2const)](../man2/UFFDIO_API.2const.html),  [UFFDIO\_CONTINUE(2const)](../man2/UFFDIO_CONTINUE.2const.html),  [UFFDIO\_COPY(2const)](../man2/UFFDIO_COPY.2const.html),  [UFFDIO\_MOVE(2const)](../man2/UFFDIO_MOVE.2const.html),  [UFFDIO\_POISON(2const)](../man2/UFFDIO_POISON.2const.html),  [UFFDIO\_REGISTER(2const)](../man2/UFFDIO_REGISTER.2const.html),  [UFFDIO\_UNREGISTER(2const)](../man2/UFFDIO_UNREGISTER.2const.html),  [UFFDIO\_WAKE(2const)](../man2/UFFDIO_WAKE.2const.html),  [UFFDIO\_WRITEPROTECT(2const)](../man2/UFFDIO_WRITEPROTECT.2const.html),  [UFFDIO\_ZEROPAGE(2const)](../man2/UFFDIO_ZEROPAGE.2const.html),  [userfaultfd(2)](../man2/userfaultfd.2.html),  [VFAT\_IOCTL\_READDIR\_BOTH(2const)](../man2/VFAT_IOCTL_READDIR_BOTH.2const.html),  [write(2)](../man2/write.2.html),  [errno(3)](../man3/errno.3.html),  [grantpt(3)](../man3/grantpt.3.html),  [if\_nameindex(3)](../man3/if_nameindex.3.html),  [if\_nametoindex(3)](../man3/if_nametoindex.3.html),  [openpty(3)](../man3/openpty.3.html),  [dsp56k(4)](../man4/dsp56k.4.html),  [fd(4)](../man4/fd.4.html),  [lirc(4)](../man4/lirc.4.html),  [loop(4)](../man4/loop.4.html),  [lp(4)](../man4/lp.4.html),  [random(4)](../man4/random.4.html),  [rtc(4)](../man4/rtc.4.html),  [sd(4)](../man4/sd.4.html),  [smartpqi(4)](../man4/smartpqi.4.html),  [st(4)](../man4/st.4.html),  [tty(4)](../man4/tty.4.html),  [vcs(4)](../man4/vcs.4.html),  [proc\_pid\_io(5)](../man5/proc_pid_io.5.html),  [arp(7)](../man7/arp.7.html),  [capabilities(7)](../man7/capabilities.7.html),  [inotify(7)](../man7/inotify.7.html),  [landlock(7)](../man7/landlock.7.html),  [namespaces(7)](../man7/namespaces.7.html),  [pipe(7)](../man7/pipe.7.html),  [pty(7)](../man7/pty.7.html),  [signal(7)](../man7/signal.7.html),  [socket(7)](../man7/socket.7.html),  [tcp(7)](../man7/tcp.7.html),  [termio(7)](../man7/termio.7.html),  [udp(7)](../man7/udp.7.html),  [unix(7)](../man7/unix.7.html),  [systemd-makefs@.service(8)](../man8/systemd-makefs@.service.8.html)

---

 

---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  | [Cover of TLPI](https://man7.org/tlpi/) |

---
