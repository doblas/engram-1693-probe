//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startTime(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &creation, &exit, &kernel, &user) != nil {
		return ""
	}
	return time.Unix(0, creation.Nanoseconds()).UTC().Format(time.RFC3339Nano)
}

// ancestry walks parent links from one Toolhelp snapshot. Windows keeps a
// parent PID after the parent exits and may reuse it, so a parent that started
// after its child is reported as stale and the walk stops there.
func ancestry(pid, depth int) []procInfo {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []procInfo{{PID: pid, Note: "snapshot failed: " + err.Error()}}
	}
	defer windows.CloseHandle(snap)
	type entry struct {
		ppid int
		name string
	}
	table := map[int]entry{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		table[int(pe.ProcessID)] = entry{int(pe.ParentProcessID), windows.UTF16ToString(pe.ExeFile[:])}
	}
	var out []procInfo
	prevStart := ""
	for i := 0; i < depth && pid > 0; i++ {
		e, ok := table[pid]
		if !ok {
			out = append(out, procInfo{PID: pid, Name: "?", Note: "exited"})
			break
		}
		p := procInfo{PID: pid, PPID: e.ppid, Name: e.name, Start: startTime(pid)}
		if prevStart != "" && p.Start != "" && p.Start > prevStart {
			p.Note = "stale parent pid"
			out = append(out, p)
			break
		}
		out = append(out, p)
		prevStart = p.Start
		pid = e.ppid
	}
	return out
}
