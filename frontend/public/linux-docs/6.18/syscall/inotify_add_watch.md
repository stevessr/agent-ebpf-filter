> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/inotify_add_watch.2.html
> Cached: 2026-04-28

---



# inotify\_add\_watch(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [EXAMPLES](#EXAMPLES) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
inotify_add_watch(2)       System Calls Manual       inotify_add_watch(2)

```

## NAME         [top](#top_of_page)

```
       inotify_add_watch - add a watch to an initialized inotify instance

```

## LIBRARY         [top](#top_of_page)

```
       Standard C library (libc, -lc)

```

## SYNOPSIS         [top](#top_of_page)

```
       #include <sys/inotify.h>

       int inotify_add_watch(int fd, const char *path, uint32_t mask);

```

## DESCRIPTION         [top](#top_of_page)

```
       inotify_add_watch() adds a new watch, or modifies an existing
       watch, for the file whose location is specified in path; the
       caller must have read permission for this file.  The fd argument
       is a file descriptor referring to the inotify instance whose watch
       list is to be modified.  The events to be monitored for path are
       specified in the mask bit-mask argument.  See inotify(7) for a
       description of the bits that can be set in mask.

       A successful call to inotify_add_watch() returns a unique watch
       descriptor for this inotify instance, for the filesystem object
       (inode) that corresponds to path.  If the filesystem object was
       not previously being watched by this inotify instance, then the
       watch descriptor is newly allocated.  If the filesystem object was
       already being watched (perhaps via a different link to the same
       object), then the descriptor for the existing watch is returned.

       The watch descriptor is returned by later read(2)s from the
       inotify file descriptor.  These reads fetch inotify_event
       structures (see inotify(7)) indicating filesystem events; the
       watch descriptor inside this structure identifies the object for
       which the event occurred.

```

## RETURN VALUE         [top](#top_of_page)

```
       On success, inotify_add_watch() returns a watch descriptor (a
       nonnegative integer).  On error, -1 is returned and errno is set
       to indicate the error.

```

## ERRORS         [top](#top_of_page)

```
       EACCES Read access to the given file is not permitted.

       EBADF  The given file descriptor is not valid.

       EEXIST mask contains IN_MASK_CREATE and path refers to a file
              already being watched by the same fd.

       EFAULT path points outside of the process's accessible address
              space.

       EINVAL The given event mask contains no valid events; or mask
              contains both IN_MASK_ADD and IN_MASK_CREATE; or fd is not
              an inotify file descriptor.

       ENAMETOOLONG
              path is too long.

       ENOENT A directory component in path does not exist or is a
              dangling symbolic link.

       ENOMEM Insufficient kernel memory was available.

       ENOSPC The user limit on the total number of inotify watches was
              reached or the kernel failed to allocate a needed resource.

       ENOTDIR
              mask contains IN_ONLYDIR and path is not a directory.

```

## STANDARDS         [top](#top_of_page)

```
       Linux.

```

## HISTORY         [top](#top_of_page)

```
       Linux 2.6.13.

```

## EXAMPLES         [top](#top_of_page)

```
       See inotify(7).

```

## SEE ALSO         [top](#top_of_page)

```
       inotify_init(2), inotify_rm_watch(2), inotify(7)

```

## COLOPHON         [top](#top_of_page)

```
       This page is part of the man-pages (Linux kernel and C library
       user-space interface documentation) project.  Information about
       the project can be found at 
       ⟨https://www.kernel.org/doc/man-pages/⟩.  If you have a bug report
       for this manual page, see
       ⟨https://git.kernel.org/pub/scm/docs/man-pages/man-pages.git/tree/CONTRIBUTING⟩.
       This page was obtained from the tarball man-pages-6.16.tar.gz
       fetched from
       ⟨https://mirrors.edge.kernel.org/pub/linux/docs/man-pages/⟩ on
       2026-01-16.  If you discover any rendering problems in this HTML
       version of the page, or you believe there is a better or more up-
       to-date source for the page, or you have corrections or
       improvements to the information in this COLOPHON (which is not
       part of the original manual page), send a mail to
       man-pages@man7.org

Linux man-pages 6.16            2025-09-21           inotify_add_watch(2)

```

---

Pages that refer to this page:
[inotify\_init(2)](../man2/inotify_init.2.html), 
[inotify\_rm\_watch(2)](../man2/inotify_rm_watch.2.html), 
[syscalls(2)](../man2/syscalls.2.html), 
[inotify(7)](../man7/inotify.7.html)

---



---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  |  |

---
