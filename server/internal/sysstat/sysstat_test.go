package sysstat

import "testing"

func TestParsers(t *testing.T) {
	if v, ok := ParseCgroupCPU([]byte("usage_usec 2500000\nuser_usec 2000000\n")); !ok || v != 2.5 {
		t.Errorf("ParseCgroupCPU = %v %v", v, ok)
	}
	// user nice system idle iowait irq softirq steal guest guest_nice
	if v, ok := ParseProcStat([]byte("cpu  100 0 50 900 30 5 5 10 7 0\ncpu0 1 2 3\n")); !ok || v != 1.7 {
		t.Errorf("ParseProcStat = %v %v", v, ok)
	}
	if v, ok := ParseCPUMax("200000 100000"); !ok || v != 2 {
		t.Errorf("ParseCPUMax = %v %v", v, ok)
	}
	if _, ok := ParseCPUMax("max 100000"); ok {
		t.Error("ParseCPUMax(max) should mean no limit")
	}
	stat := "4242 (ffmpeg (x) y) S 1 1 1 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 1 0 100 1000 10"
	if v, ok := ParsePidStat([]byte(stat)); !ok || v != 3 {
		t.Errorf("ParsePidStat = %v %v", v, ok)
	}
	netdev := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo:  290176    2266    0    0    0     0          0         0   290176    2266    0    0    0     0       0          0\n" +
		"  eth0: 970980008  825370    0    0    0     0          0       161 48948954  299441    0    0    0     0       0          0\n" +
		"  eth1:    1000       1    0    0    0     0          0         0    2000       1    0    0    0     0       0          0\n"
	if c := ParseNetDev([]byte(netdev)); !c.ok || c.in != 970981008 || c.out != 48950954 {
		t.Errorf("ParseNetDev = %+v", c)
	}
	if c := ParseNetDev([]byte("Inter-|\n face |\n")); c.ok {
		t.Errorf("ParseNetDev of no interfaces = %+v", c)
	}
	mounts := "device proc mounted on /proc with fstype proc\n" +
		"device 10.0.0.5:/media mounted on /media with fstype nfs4 statvers=1.1\n" +
		"\topts:\trw,vers=4.2\n" +
		"\tbytes:\t5000 10 0 0 4000 20 1 2\n" +
		"device 10.0.0.6:/tv mounted on /tv with fstype nfs statvers=1.1\n" +
		"\tbytes:\t100 0 0 0 60 0 0 0\n" +
		"device /dev/sda1 mounted on /data with fstype ext4\n" +
		"\tbytes:\t1 1 1 1 1 1 1 1\n"
	if c := ParseMountstats([]byte(mounts)); !c.ok || c.in != 4060 || c.out != 20 {
		t.Errorf("ParseMountstats = %+v", c)
	}
	if c := ParseMountstats([]byte("device /dev/sda1 mounted on /data with fstype ext4\n")); c.ok {
		t.Errorf("ParseMountstats without NFS = %+v", c)
	}
}

func TestRead(t *testing.T) {
	var s Sampler
	st := s.Read()
	if st.Available && (st.Cores <= 0 || st.MemTotal == 0 || st.Server.Count != 1) {
		t.Errorf("implausible reading: %+v", st)
	}
}
