//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// procStat returns ppid, name and a start token for pid: the boot-relative
// start tick from /proc on Linux, otherwise `ps -o lstart`.
func procStat(pid int) (int, string, string, bool) {
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		s := string(data)
		open, close := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if open < 0 || close < open {
			return 0, "", "", false
		}
		fields := strings.Fields(s[close+1:])
		if len(fields) < 20 {
			return 0, "", "", false
		}
		return atoi(fields[1]), s[open+1 : close], "tick:" + fields[19], true
	}
	out, err := exec.Command("ps", "-o", "ppid=,lstart=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", "", false
	}
	f := strings.Fields(string(out))
	if len(f) < 7 {
		return 0, "", "", false
	}
	return atoi(f[0]), strings.Join(f[6:], " "), strings.Join(f[1:6], " "), true
}

func ancestry(pid, depth int) []procInfo {
	var out []procInfo
	for i := 0; i < depth && pid > 0; i++ {
		ppid, name, start, ok := procStat(pid)
		if !ok {
			out = append(out, procInfo{PID: pid, Name: "?", Note: "exited"})
			break
		}
		out = append(out, procInfo{PID: pid, PPID: ppid, Name: name, Start: start})
		pid = ppid
	}
	return out
}
