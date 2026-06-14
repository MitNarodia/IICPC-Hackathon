package security

type SeccompProfile struct {
	DefaultAction string           `json:"defaultAction"`
	Architectures []string         `json:"architectures"`
	Syscalls      []SeccompSyscall `json:"syscalls"`
}

type SeccompSyscall struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

func DefaultSeccomp() SeccompProfile {
	allowed := []string{
		"accept", "accept4", "access", "arch_prctl", "bind", "brk", "clock_gettime",
		"close", "connect", "dup", "dup2", "epoll_create1", "epoll_ctl", "epoll_pwait",
		"epoll_wait", "eventfd2", "exit", "exit_group", "fcntl", "fstat", "futex",
		"getcwd", "getdents64", "getpid", "getsockname", "getsockopt", "gettid",
		"listen", "lseek", "madvise", "mmap", "mprotect", "munmap", "nanosleep",
		"newfstatat", "openat", "poll", "ppoll", "pread64", "prlimit64", "read",
		"readlink", "recvfrom", "recvmsg", "rt_sigaction", "rt_sigprocmask",
		"rt_sigreturn", "sched_getaffinity", "sendmsg", "sendto", "setsockopt",
		"shutdown", "socket", "statx", "tgkill", "write", "writev",
	}
	return SeccompProfile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{
			"SCMP_ARCH_X86_64",
			"SCMP_ARCH_AARCH64",
		},
		Syscalls: []SeccompSyscall{{
			Names:  allowed,
			Action: "SCMP_ACT_ALLOW",
		}},
	}
}

func DangerousSyscalls() []string {
	return []string{
		"bpf", "clone3", "finit_module", "init_module", "io_uring_setup",
		"kexec_load", "keyctl", "mount", "open_by_handle_at", "perf_event_open",
		"pivot_root", "process_vm_readv", "process_vm_writev", "ptrace",
		"reboot", "setns", "swapon", "swapoff", "umount", "umount2", "unshare",
	}
}
