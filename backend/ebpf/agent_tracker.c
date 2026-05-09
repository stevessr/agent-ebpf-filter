// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#include "agent_tracker_common.h"
#include "agent_tracker_syscalls.h"
#include "agent_tracker_tail.h"
