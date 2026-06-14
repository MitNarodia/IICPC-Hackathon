# Red-Team Corpus

These fixtures exercise the containment gates described in `docs/03_SECURITY.md`.

- `fork_bomb.sh`: must hit `pids.max` and stay contained to one sandbox.
- `memory_bomb.c`: must hit `memory.max` and OOM only its own cgroup.
- `metadata_probe.c`: must fail because sandbox egress is denied, including `169.254.169.254`.

The current repository contains the fixtures and unit-level policy tests. Full execution of this corpus requires a host or cluster with cgroups v2, AppArmor, and gVisor/runsc installed.
