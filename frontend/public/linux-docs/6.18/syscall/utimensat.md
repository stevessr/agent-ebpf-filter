> Local snapshot: Linux 6.18 LTS
> Source: https://man7.org/linux/man-pages/man2/utimensat.2.html
> Cached: 2026-04-28

|  |  |
| --- | --- |
| [man7.org](../../../index.html) > Linux > [man-pages](../index.html) | [Linux/UNIX system programming training](http://man7.org/training/) |

---

# utimensat(2) — Linux manual page

|  |
| --- |
| [NAME](#NAME) | [LIBRARY](#LIBRARY) | [SYNOPSIS](#SYNOPSIS) | [DESCRIPTION](#DESCRIPTION) | [RETURN VALUE](#RETURN_VALUE) | [ERRORS](#ERRORS) | [ATTRIBUTES](#ATTRIBUTES) | [VERSIONS](#VERSIONS) | [STANDARDS](#STANDARDS) | [HISTORY](#HISTORY) | [NOTES](#NOTES) | [BUGS](#BUGS) | [SEE ALSO](#SEE_ALSO) | [COLOPHON](#COLOPHON) |
|  |  |

```
utimensat(2) System Calls Manual utimensat(2)
```

## NAME         [top](#top_of_page)

```
utimensat, futimens - change file timestamps with nanosecond precision 
```

## LIBRARY         [top](#top_of_page)

```
Standard C library (libc, -lc) 
```

## SYNOPSIS         [top](#top_of_page)

```
#include  /* Definition of AT_* constants */ #include  int utimensat(int dirfd, const char *path, const struct timespec times[_Nullable 2], int flags); int futimens(int fd, const struct timespec times[_Nullable 2]); Feature Test Macro Requirements for glibc (see feature_test_macros(7)): utimensat(): Since glibc 2.10: _POSIX_C_SOURCE >= 200809L Before glibc 2.10: _ATFILE_SOURCE futimens(): Since glibc 2.10: _POSIX_C_SOURCE >= 200809L Before glibc 2.10: _GNU_SOURCE 
```

## DESCRIPTION         [top](#top_of_page)

```
utimensat() and futimens() update the timestamps of a file with nanosecond precision. This contrasts with the historical utime(2) and utimes(2), which permit only second and microsecond precision, respectively, when setting file timestamps. With utimensat() the file is specified via the pathname given in path. With futimens() the file whose timestamps are to be updated is specified via an open file descriptor, fd. For both calls, the new file timestamps are specified in the array times: times[0] specifies the new "last access time" (atime); times[1] specifies the new "last modification time" (mtime). Each of the elements of times specifies a time as the number of seconds and nanoseconds since the Epoch, 1970-01-01 00:00:00 +0000 (UTC). This information is conveyed in a timespec(3) structure. Updated file timestamps are set to the greatest value supported by the filesystem that is not greater than the specified time. If the tv_nsec field of one of the timespec structures has the special value UTIME_NOW, then the corresponding file timestamp is set to the current time. If the tv_nsec field of one of the timespec structures has the special value UTIME_OMIT, then the corresponding file timestamp is left unchanged. In both of these cases, the value of the corresponding tv_sec field is ignored. If times is NULL, then both timestamps are set to the current time. The status change time (ctime) will be set to the current time, even if the other time stamps don't actually change. Permissions requirements To set both file timestamps to the current time (i.e., times is NULL, or both tv_nsec fields specify UTIME_NOW), either: • the caller must have write access to the file; • the caller's effective user ID must match the owner of the file; or • the caller must have appropriate privileges. To make any change other than setting both timestamps to the current time (i.e., times is not NULL, and neither tv_nsec field is UTIME_NOW and neither tv_nsec field is UTIME_OMIT), either condition 2 or 3 above must apply. If both tv_nsec fields are specified as UTIME_OMIT, then no file ownership or permission checks are performed, and the file timestamps are not modified, but other error conditions may still be detected. utimensat() specifics If path is relative, then by default it is interpreted relative to the directory referred to by the open file descriptor, dirfd (rather than relative to the current working directory of the calling process, as is done by utimes(2) for a relative pathname). See openat(2) for an explanation of why this can be useful. If path is relative and dirfd is the special value AT_FDCWD, then path is interpreted relative to the current working directory of the calling process (like utimes(2)). If path is absolute, then dirfd is ignored. The flags argument is a bit mask created by ORing together zero or more of the following values defined in : AT_EMPTY_PATH (since Linux 5.8) If path is an empty string, operate on the file referred to by dirfd (which may have been obtained using the open(2) O_PATH flag). In this case, dirfd can refer to any type of file, not just a directory. If dirfd is AT_FDCWD, the call operates on the current working directory. This flag is Linux-specific; define _GNU_SOURCE to obtain its definition. AT_SYMLINK_NOFOLLOW If path specifies a symbolic link, then update the timestamps of the link, rather than the file to which it refers. 
```

## RETURN VALUE         [top](#top_of_page)

```
On success, utimensat() and futimens() return 0. On error, -1 is returned and errno is set to indicate the error. 
```

## ERRORS         [top](#top_of_page)

```
EACCES times is NULL, or both tv_nsec values are UTIME_NOW, and the effective user ID of the caller does not match the owner of the file, the caller does not have write access to the file, and the caller is not privileged (Linux: does not have either the CAP_FOWNER or the CAP_DAC_OVERRIDE capability). EBADF (futimens()) fd is not a valid file descriptor. EBADF (utimensat()) path is relative but dirfd is neither AT_FDCWD nor a valid file descriptor. EFAULT times pointed to an invalid address; or, dirfd was AT_FDCWD, and path is NULL or an invalid address. EINVAL Invalid value in flags. EINVAL Invalid value in one of the tv_nsec fields (value outside range [0, 999,999,999], and not UTIME_NOW or UTIME_OMIT); or an invalid value in one of the tv_sec fields. EINVAL path is NULL, dirfd is not AT_FDCWD, and flags contains AT_SYMLINK_NOFOLLOW. ELOOP (utimensat()) Too many symbolic links were encountered in resolving path. ENAMETOOLONG (utimensat()) path is too long. ENOENT (utimensat()) A component of path does not refer to an existing directory or file, or path is an empty string. ENOTDIR (utimensat()) path is relative, but dirfd is neither AT_FDCWD nor a file descriptor referring to a directory; or, one of the prefix components of path is not a directory. EPERM The caller attempted to change one or both timestamps to a value other than the current time, or to change one of the timestamps to the current time while leaving the other timestamp unchanged, (i.e., times is not NULL, neither tv_nsec field is UTIME_NOW, and neither tv_nsec field is UTIME_OMIT) and either: • the caller's effective user ID does not match the owner of file, and the caller is not privileged (Linux: does not have the CAP_FOWNER capability); or, • the file is marked append-only or immutable (see chattr(1)). EROFS The file is on a read-only filesystem. ESRCH (utimensat()) Search permission is denied for one of the prefix components of path. 
```

## ATTRIBUTES         [top](#top_of_page)

```
For an explanation of the terms used in this section, see attributes(7). ┌──────────────────────────────────────┬───────────────┬─────────┐ │ Interface │ Attribute │ Value │ ├──────────────────────────────────────┼───────────────┼─────────┤ │ utimensat(), futimens() │ Thread safety │ MT-Safe │ └──────────────────────────────────────┴───────────────┴─────────┘ 
```

## VERSIONS         [top](#top_of_page)

```
C library/kernel ABI differences On Linux, futimens() is a library function implemented on top of the utimensat() system call. To support this, the Linux utimensat() system call implements a nonstandard feature: if path is NULL, then the call modifies the timestamps of the file referred to by the file descriptor dirfd (which may refer to any type of file). Using this feature, the call futimens(fd, times) is implemented as: utimensat(fd, NULL, times, 0); Note, however, that the glibc wrapper for utimensat() disallows passing NULL as the value for path: the wrapper function returns the error EINVAL in this case. 
```

## STANDARDS         [top](#top_of_page)

```
POSIX.1-2024. 
```

## HISTORY         [top](#top_of_page)

```
utimensat() Linux 2.6.22, glibc 2.6. POSIX.1-2008. futimens() glibc 2.6. POSIX.1-2008. 
```

## NOTES         [top](#top_of_page)

```
utimensat() obsoletes futimesat(2). On Linux, timestamps cannot be changed for a file marked immutable, and the only change permitted for files marked append- only is to set the timestamps to the current time. (This is consistent with the historical behavior of utime(2) and utimes(2) on Linux.) If both tv_nsec fields are specified as UTIME_OMIT, then the Linux implementation of utimensat() succeeds even if the file referred to by dirfd and path does not exist. 
```

## BUGS         [top](#top_of_page)

```
Several bugs afflict utimensat() and futimens() before Linux 2.6.26. These bugs are either nonconformances with the POSIX.1 draft specification or inconsistencies with historical Linux behavior. • POSIX.1 specifies that if one of the tv_nsec fields has the value UTIME_NOW or UTIME_OMIT, then the value of the corresponding tv_sec field should be ignored. Instead, the value of the tv_sec field is required to be 0 (or the error EINVAL results). • Various bugs mean that for the purposes of permission checking, the case where both tv_nsec fields are set to UTIME_NOW isn't always treated the same as specifying times as NULL, and the case where one tv_nsec value is UTIME_NOW and the other is UTIME_OMIT isn't treated the same as specifying times as a pointer to an array of structures containing arbitrary time values. As a result, in some cases: a) file timestamps can be updated by a process that shouldn't have permission to perform updates; b) file timestamps can't be updated by a process that should have permission to perform updates; and c) the wrong errno value is returned in case of an error. • POSIX.1 says that a process that has write access to the file can make a call with times as NULL, or with times pointing to an array of structures in which both tv_nsec fields are UTIME_NOW, in order to update both timestamps to the current time. However, futimens() instead checks whether the access mode of the file descriptor allows writing. 
```

## SEE ALSO         [top](#top_of_page)

```
chattr(1), touch(1), futimesat(2), openat(2), stat(2), utimes(2), futimes(3), timespec(3), inode(7), path_resolution(7), symlink(7) 
```

## COLOPHON         [top](#top_of_page)

```
Linux man-pages 6.16 2025-10-29 utimensat(2)
```

---

Pages that refer to this page: [F\_NOTIFY(2const)](../man2/F_NOTIFY.2const.html),  [futimesat(2)](../man2/futimesat.2.html),  [open(2)](../man2/open.2.html),  [syscalls(2)](../man2/syscalls.2.html),  [utime(2)](../man2/utime.2.html),  [futimes(3)](../man3/futimes.3.html),  [inotify(7)](../man7/inotify.7.html),  [signal-safety(7)](../man7/signal-safety.7.html),  [symlink(7)](../man7/symlink.7.html),  [xfs\_io(8)](../man8/xfs_io.8.html)

---

 

---

|  |  |  |
| --- | --- | --- |
| HTML rendering created 2026-01-16 by [Michael Kerrisk](https://man7.org/mtk/index.html), author of [*The Linux Programming Interface*](https://man7.org/tlpi/).  For details of in-depth **Linux/UNIX system programming training courses** that I teach, look [here](https://man7.org/training/).  Hosting by [jambit GmbH](https://www.jambit.com/index_en.html). |  | [Cover of TLPI](https://man7.org/tlpi/) |

---
