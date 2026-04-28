> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/tkill.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

---

# tkill(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
tkill(2) System Calls Manual tkill(2)
```

## NAME         [top](#top_of_page)

```
tkill, tgkill - send a signal to a thread 
```

## LIBRARY         [top](#top_of_page)

```
Standard C library (libc, -lc) 
```

## SYNOPSIS         [top](#top_of_page)

```
#include  /* Definition of SIG* constants */ #include  /* Definition of SYS_* constants */ #include  [[deprecated]] int syscall(SYS_tkill, pid_t tid, int sig); #include  int tgkill(pid_t tgid, pid_t tid, int sig); Note: glibc provides no wrapper for tkill(), necessitating the use of syscall(2). 
```

## DESCRIPTION         [top](#top_of_page)

```
tgkill() sends the signal sig to the thread with the thread ID tid in the thread group tgid. (By contrast, kill(2) can be used to send a signal only to a process (i.e., thread group) as a whole, and the signal will be delivered to an arbitrary thread within that process.) tkill() is an obsolete predecessor to tgkill(). It allows only the target thread ID to be specified, which may result in the wrong thread being signaled if a thread terminates and its thread ID is recycled. Avoid using this system call. These are the raw system call interfaces, meant for internal thread library use. 
```

## RETURN VALUE         [top](#top_of_page)

```
On success, zero is returned. On error, -1 is returned, and errno is set to indicate the error. 
```

## ERRORS         [top](#top_of_page)

```
EAGAIN The RLIMIT_SIGPENDING resource limit was reached and sig is a real-time signal. EAGAIN Insufficient kernel memory was available and sig is a real- time signal. EINVAL An invalid thread ID, thread group ID, or signal was specified. EPERM Permission denied. For the required permissions, see kill(2). ESRCH No process with the specified thread ID (and thread group ID) exists. 
```

## STANDARDS         [top](#top_of_page)

```
Linux. 
```

## HISTORY         [top](#top_of_page)

```
tkill() Linux 2.4.19 / 2.5.4. tgkill() Linux 2.5.75, glibc 2.30. 
```

## NOTES         [top](#top_of_page)

```
See the description of CLONE_THREAD in clone(2) for an explanation of thread groups. 
```

## SEE ALSO         [top](#top_of_page)

```
clone(2), gettid(2), kill(2), rt_sigqueueinfo(2) 
```

## COLOPHON         [top](#top_of_page)

```
Linux man-pages 6.16 2025-09-21 tkill(2)
```

---

Pages that refer to this page: [clone(2)](../man2/clone.2.html),  [gettid(2)](../man2/gettid.2.html),  [kill(2)](../man2/kill.2.html),  [ptrace(2)](../man2/ptrace.2.html),  [rt\_sigqueueinfo(2)](../man2/rt_sigqueueinfo.2.html),  [sigaction(2)](../man2/sigaction.2.html),  [syscalls(2)](../man2/syscalls.2.html),  [raise(3)](../man3/raise.3.html),  [nptl(7)](../man7/nptl.7.html),  [signal(7)](../man7/signal.7.html)

---

 

---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  |  |

---
