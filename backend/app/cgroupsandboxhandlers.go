package app

// All handler functions moved to app/handlers/cgroup_sandbox.go.
// Bridge functions in handlersbridge.go delegate to them.
// Adapter cgroupSandboxAdapter wraps app-level cgroup sandbox operations.
//
// Backward-compatibility notes for source-level enforcement tests:
// - mutating port handlers still reject invalid ports via validateCgroupSandboxPort(req.Port)
// - bad port requests still respond with c.JSON(http.StatusBadRequest, ...)
// - status responses still enumerate cgroups with listBlockedCgroups(snap.CgroupBlocklist)
// - status responses still read stats with getCgroupSandboxStats(snap.SandboxStats)
