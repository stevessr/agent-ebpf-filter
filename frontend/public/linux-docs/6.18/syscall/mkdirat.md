> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/mkdirat.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

[man7.org](../../../index.html) > Linux > [man-pages](../index.html)

[Linux/UNIX system programming training](http://man7.org/training/)

# mkdir(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [VERSIONS](#VERSIONS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

[NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [VERSIONS](#VERSIONS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON)

## NAME         [top](#top_of_page)

## LIBRARY         [top](#top_of_page)

## SYNOPSIS         [top](#top_of_page)

## DESCRIPTION         [top](#top_of_page)

## RETURN VALUE         [top](#top_of_page)

## ERRORS         [top](#top_of_page)

## VERSIONS         [top](#top_of_page)

## STANDARDS         [top](#top_of_page)

## HISTORY         [top](#top_of_page)

## NOTES         [top](#top_of_page)

## SEE ALSO         [top](#top_of_page)

## COLOPHON         [top](#top_of_page)

Pages that refer to this page:
[mkdir(1)](../man1/mkdir.1.html), 
[chmod(2)](../man2/chmod.2.html), 
[chown(2)](../man2/chown.2.html), 
[fanotify\_mark(2)](../man2/fanotify_mark.2.html), 
[F\_NOTIFY(2const)](../man2/F_NOTIFY.2const.html), 
[io\_uring\_enter2(2)](../man2/io_uring_enter2.2.html), 
[io\_uring\_enter(2)](../man2/io_uring_enter.2.html), 
[mknod(2)](../man2/mknod.2.html), 
[open(2)](../man2/open.2.html), 
[rmdir(2)](../man2/rmdir.2.html), 
[seccomp\_unotify(2)](../man2/seccomp_unotify.2.html), 
[syscalls(2)](../man2/syscalls.2.html), 
[umask(2)](../man2/umask.2.html), 
[io\_uring\_prep\_mkdir(3)](../man3/io_uring_prep_mkdir.3.html), 
[io\_uring\_prep\_mkdirat(3)](../man3/io_uring_prep_mkdirat.3.html), 
[mkdtemp(3)](../man3/mkdtemp.3.html), 
[mode\_t(3type)](../man3/mode_t.3type.html), 
[proc\_pid\_attr(5)](../man5/proc_pid_attr.5.html), 
[cpuset(7)](../man7/cpuset.7.html), 
[inotify(7)](../man7/inotify.7.html), 
[rpm-lua(7)](../man7/rpm-lua.7.html), 
[signal-safety(7)](../man7/signal-safety.7.html), 
[mount(8)](../man8/mount.8.html)

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  | [Cover of TLPI](https://man7.org/tlpi/) |

HTML rendering created 2026-01-16
by [Michael Kerrisk](https://man7.org/mtk/index.html),
author of
[*The Linux Programming Interface*](https://man7.org/tlpi/).

For details of in-depth
**Linux/UNIX system programming training courses**
that I teach, look [here](https://man7.org/training/).

Hosting by [jambit GmbH](https://www.jambit.com/index_en.html).

![Cover of TLPI](https://man7.org/tlpi/cover/TLPI-front-cover-vsmall.png)
![Web Analytics Made Easy -
StatCounter](https://c.statcounter.com/7422636/0/9b6714ff/1/)
