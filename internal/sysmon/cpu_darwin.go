//go:build darwin

package sysmon

/*
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <sys/sysctl.h>

// mem_used follows Activity Monitor's "Memory Used": app memory (internal
// pages minus purgeable) + wired + compressed.
static int mem_info(unsigned long long *used, unsigned long long *total) {
	vm_statistics64_data_t vm;
	mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
	if (host_statistics64(mach_host_self(), HOST_VM_INFO64, (host_info64_t)&vm, &count) != KERN_SUCCESS) {
		return -1;
	}
	vm_size_t page;
	if (host_page_size(mach_host_self(), &page) != KERN_SUCCESS) {
		return -1;
	}
	unsigned long long mem = 0;
	size_t len = sizeof(mem);
	if (sysctlbyname("hw.memsize", &mem, &len, NULL, 0) != 0) {
		return -1;
	}
	unsigned long long app = vm.internal_page_count > vm.purgeable_count ? vm.internal_page_count - vm.purgeable_count : 0;
	*used = (app + vm.wire_count + vm.compressor_page_count) * (unsigned long long)page;
	*total = mem;
	return 0;
}

static int cpu_ticks(unsigned long long *busy, unsigned long long *total) {
	host_cpu_load_info_data_t info;
	mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
	if (host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)&info, &count) != KERN_SUCCESS) {
		return -1;
	}
	unsigned long long user = info.cpu_ticks[CPU_STATE_USER];
	unsigned long long sys = info.cpu_ticks[CPU_STATE_SYSTEM];
	unsigned long long nice = info.cpu_ticks[CPU_STATE_NICE];
	unsigned long long idle = info.cpu_ticks[CPU_STATE_IDLE];
	*busy = user + sys + nice;
	*total = user + sys + nice + idle;
	return 0;
}
*/
import "C"

// cpuTicks reads the whole machine's CPU counters (no process started).
func cpuTicks() (busy, total uint64, ok bool) {
	var b, t C.ulonglong
	if C.cpu_ticks(&b, &t) != 0 {
		return 0, 0, false
	}
	return uint64(b), uint64(t), true
}

// memInfo reads used and total memory in bytes (no process started).
func memInfo() (used, total uint64, ok bool) {
	var u, t C.ulonglong
	if C.mem_info(&u, &t) != 0 {
		return 0, 0, false
	}
	return uint64(u), uint64(t), true
}

// nextPID is not exposed on macOS; the monitor falls back to a probe process.
func nextPID() (int, bool) { return 0, false }
