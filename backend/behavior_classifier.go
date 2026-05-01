package main

import (
	"regexp"
	"sort"
	"strings"

	"agent-ebpf-filter/pb"
)

type classificationRule struct {
	category pb.BehaviorCategory
	commRe   *regexp.Regexp
	argRe    *regexp.Regexp
}

var categoryNames = map[pb.BehaviorCategory]string{
	pb.BehaviorCategory_UNKNOWN:          "UNKNOWN",
	pb.BehaviorCategory_FILE_READ:        "FILE_READ",
	pb.BehaviorCategory_FILE_WRITE:       "FILE_WRITE",
	pb.BehaviorCategory_FILE_DELETE:      "FILE_DELETE",
	pb.BehaviorCategory_FILE_PERMISSION:  "FILE_PERMISSION",
	pb.BehaviorCategory_NETWORK:          "NETWORK",
	pb.BehaviorCategory_PROCESS_EXEC:     "PROCESS_EXEC",
	pb.BehaviorCategory_PROCESS_KILL:     "PROCESS_KILL",
	pb.BehaviorCategory_SYSTEM_INFO:      "SYSTEM_INFO",
	pb.BehaviorCategory_PACKAGE_MANAGER:  "PACKAGE_MANAGER",
	pb.BehaviorCategory_DATABASE:         "DATABASE",
	pb.BehaviorCategory_COMPRESSION:      "COMPRESSION",
	pb.BehaviorCategory_DEVELOPMENT:      "DEVELOPMENT",
	pb.BehaviorCategory_CONTAINER:        "CONTAINER",
	pb.BehaviorCategory_SENSITIVE:        "SENSITIVE",
}

var categoryPriority = map[pb.BehaviorCategory]int{
	pb.BehaviorCategory_SENSITIVE:        1,
	pb.BehaviorCategory_PROCESS_EXEC:     2,
	pb.BehaviorCategory_FILE_DELETE:      3,
	pb.BehaviorCategory_NETWORK:          4,
	pb.BehaviorCategory_FILE_WRITE:       5,
	pb.BehaviorCategory_FILE_PERMISSION:  6,
	pb.BehaviorCategory_CONTAINER:        7,
	pb.BehaviorCategory_DATABASE:         8,
	pb.BehaviorCategory_PACKAGE_MANAGER:  9,
	pb.BehaviorCategory_FILE_READ:        10,
	pb.BehaviorCategory_PROCESS_KILL:     11,
	pb.BehaviorCategory_COMPRESSION:      12,
	pb.BehaviorCategory_DEVELOPMENT:      13,
	pb.BehaviorCategory_SYSTEM_INFO:      14,
	pb.BehaviorCategory_UNKNOWN:          99,
}

// matchAnyArg checks if any arg string matches the regex
func matchAnyArg(re *regexp.Regexp, args []string) bool {
	for _, a := range args {
		if re.MatchString(a) {
			return true
		}
	}
	return false
}

var rules = []classificationRule{
	// File Read
	{category: pb.BehaviorCategory_FILE_READ, commRe: regexp.MustCompile(`^(cat|head|tail|less|more|view|bat|strings|od|hexdump|xxd)$`)},
	// File Write
	{category: pb.BehaviorCategory_FILE_WRITE, commRe: regexp.MustCompile(`^(cp|mv|tee|dd|install|touch)$`)},
	{category: pb.BehaviorCategory_FILE_WRITE, commRe: regexp.MustCompile(`^echo$`), argRe: regexp.MustCompile(`^[>|]`)},
	// File Delete
	{category: pb.BehaviorCategory_FILE_DELETE, commRe: regexp.MustCompile(`^(rm|unlink|shred|rmdir)$`)},
	// File Permission
	{category: pb.BehaviorCategory_FILE_PERMISSION, commRe: regexp.MustCompile(`^(chmod|chown|chattr|chgrp|setfacl|getfacl)$`)},
	// Network
	{category: pb.BehaviorCategory_NETWORK, commRe: regexp.MustCompile(`^(curl|wget|nc|netcat|telnet|ssh|scp|rsync|ftp|sftp|nmap|dig|host|nslookup|ping|traceroute|whois|socat)$`)},
	// Process Exec
	{category: pb.BehaviorCategory_PROCESS_EXEC, commRe: regexp.MustCompile(`^(exec|bash|sh|zsh|fish|dash|ksh|tcsh)$`)},
	// Process Kill
	{category: pb.BehaviorCategory_PROCESS_KILL, commRe: regexp.MustCompile(`^(kill|pkill|killall|skill|xkill)$`)},
	// System Info
	{category: pb.BehaviorCategory_SYSTEM_INFO, commRe: regexp.MustCompile(`^(ps|top|htop|btop|df|du|free|uname|uptime|whoami|id|groups|hostname|dmesg|lspci|lsusb|lsblk|lscpu|lsmem|lslocks|ss|netstat|env|printenv|set|ulimit|getconf)$`)},
	// Package Manager
	{category: pb.BehaviorCategory_PACKAGE_MANAGER, commRe: regexp.MustCompile(`^(apt|apt-get|apt-cache|yum|dnf|pacman|zypper|brew|port|pip|pip3|npm|yarn|pnpm|go|cargo|gem|composer|cpan|snap|flatpak|nix-env|rpm|dpkg)$`)},
	// Database
	{category: pb.BehaviorCategory_DATABASE, commRe: regexp.MustCompile(`^(mysql|psql|sqlite3|mongo|mongosh|redis-cli|cqlsh|influx|pg_dump|pg_restore|mysqldump)$`)},
	// Compression
	{category: pb.BehaviorCategory_COMPRESSION, commRe: regexp.MustCompile(`^(tar|gzip|gunzip|zip|unzip|7z|rar|bzip2|bunzip2|xz|unxz|lz4)$`)},
	// Development
	{category: pb.BehaviorCategory_DEVELOPMENT, commRe: regexp.MustCompile(`^(git|make|cmake|gcc|g\+\+|clang|clang\+\+|python|python3|node|java|javac|rustc|npx|tsc|eslint|prettier|jest|vitest)$`)},
	// Container
	{category: pb.BehaviorCategory_CONTAINER, commRe: regexp.MustCompile(`^(docker|podman|kubectl|k3s|helm|nerdctl|buildah|skopeo)$`)},
	// Sensitive
	{category: pb.BehaviorCategory_SENSITIVE, commRe: regexp.MustCompile(`^(sudo|su|doas|pkexec|passwd|chpasswd|cryptsetup|mount|umount|fdisk|parted|mkfs|lvm|pvcreate|vgcreate|iptables|nft|ufw|firewall-cmd|setenforce|sysctl)$`)},
}

type Classifier struct{}

var globalClassifier = &Classifier{}

func ClassifyBehavior(comm string, args []string) *pb.BehaviorClassification {
	type present struct{}
	cats := make(map[pb.BehaviorCategory]present)
	confidence := "low"

	for _, r := range rules {
		if !r.commRe.MatchString(comm) {
			continue
		}
		if r.argRe != nil {
			if !matchAnyArg(r.argRe, args) {
				continue
			}
			cats[r.category] = present{}
			confidence = "medium"
		} else {
			cats[r.category] = present{}
			confidence = "high"
		}
	}

	if len(cats) == 0 {
		cats[pb.BehaviorCategory_UNKNOWN] = present{}
		confidence = "low"
	}

	out := make([]pb.BehaviorCategory, 0, len(cats))
	for c := range cats {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })

	primary := pb.BehaviorCategory_UNKNOWN
	bestP := 99
	for _, c := range out {
		if p := categoryPriority[c]; p < bestP {
			bestP = p
			primary = c
		}
	}

	reason := "Classified as: "
	if primary == pb.BehaviorCategory_UNKNOWN {
		reason = "Command does not match any known behavior pattern"
	} else {
		names := make([]string, 0, len(out))
		for _, c := range out {
			names = append(names, categoryNames[c])
		}
		reason += strings.Join(names, ", ")
	}

	return &pb.BehaviorClassification{
		Categories:      out,
		PrimaryCategory: categoryNames[primary],
		Confidence:      confidence,
		Reasoning:       reason,
	}
}
