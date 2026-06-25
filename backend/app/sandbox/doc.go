// Package sandbox provides cgroup network sandbox and BPF LSM enforcement.
//
// This package manages eBPF-based enforcement mechanisms:
//   - CgroupSandbox: network connect/sendmsg blocking by cgroup, IP, and port
//   - LSM enforcer: file path/name and executable blocking via BPF LSM
//
// Both mechanisms pin their eBPF maps and links to bpffs for persistence
// across backend restarts.
//
// This package is part of the app/ refactoring. The app package contains
// the original versions of these files which delegate to this package.
package sandbox
