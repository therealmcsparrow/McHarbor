// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Sample is one resource reading of a container at a moment in time.
type Sample struct {
	CPUPercent float64
	MemUsed    int64
	MemLimit   int64
	NetRx      int64
	NetTx      int64
	BlockRead  int64
	BlockWrite int64
	PIDs       int64
	CPUTotalNS uint64
}

// wave is a smooth, deterministic noise in [0,1] for key at t, so every chart
// is a believable curve without storing any history.
func wave(key uint64, t time.Time, period float64) float64 {
	x := float64(t.Unix()) / period
	phase := float64(key%1000) / 1000 * 2 * math.Pi
	v := 0.5 + 0.28*math.Sin(x+phase) + 0.14*math.Sin(2.7*x+phase*1.7) + 0.08*math.Sin(7.1*x+phase*3.1)
	return math.Max(0, math.Min(1, v))
}

// SampleContainer returns c's resources at t. Stopped containers read zero.
func (h *Host) SampleContainer(c Container, t time.Time) Sample {
	if c.State != StateRunning && c.State != StateRestarting {
		return Sample{MemLimit: c.MemLimit}
	}
	key := hashString(c.ID)
	up := t.Sub(c.StartedAt).Seconds()
	if up < 0 {
		up = 0
	}
	cpu := c.CPUBase * (0.35 + 1.3*wave(key, t, 90))
	if wave(key^0xabc, t, 600) > 0.93 {
		cpu *= 2.4
	}
	mem := int64(float64(c.MemBase) * (0.85 + 0.3*wave(key^0x55, t, 1800)))
	if c.MemLimit > 0 && mem > c.MemLimit {
		mem = c.MemLimit * 97 / 100
	}
	rate := c.CPUBase * 12000
	return Sample{
		CPUPercent: math.Round(cpu*100) / 100,
		MemUsed:    mem,
		MemLimit:   c.MemLimit,
		NetRx:      int64(up * rate * (1 + wave(key^1, t, 300))),
		NetTx:      int64(up * rate * 0.6 * (1 + wave(key^2, t, 300))),
		BlockRead:  int64(up * rate * 0.2),
		BlockWrite: int64(up * rate * 0.35),
		PIDs:       int64(4 + key%40),
		CPUTotalNS: uint64(up * c.CPUBase / 100 * 1e9),
	}
}

// HostSample is the host-level reading the environment page shows.
type HostSample struct {
	CPUPercent float64
	MemUsed    int64
	Load1      float64
	Load5      float64
	Load15     float64
	Uptime     int64
}

// SampleHost sums the running containers onto the host's baseline at t.
func (h *Host) SampleHost(t time.Time) HostSample {
	var cpu float64
	var mem int64
	for _, c := range h.Containers() {
		s := h.SampleContainer(c, t)
		cpu += s.CPUPercent
		mem += s.MemUsed
	}
	pct := math.Min(97, cpu/float64(max(1, h.CPUs))+3+4*wave(hashString(h.Key), t, 240))
	mem += h.MemTotal / 20
	if mem > h.MemTotal {
		mem = h.MemTotal * 93 / 100
	}
	load := pct / 100 * float64(h.CPUs)
	return HostSample{
		CPUPercent: math.Round(pct*100) / 100,
		MemUsed:    mem,
		Load1:      math.Round(load*100) / 100,
		Load5:      math.Round(load*0.94*100) / 100,
		Load15:     math.Round(load*0.9*100) / 100,
		Uptime:     int64(t.Sub(h.BootedAt).Seconds()),
	}
}

var logLines = map[string][]string{
	"web":        {`GET / 200 %dms`, `GET /products/%d 200 %dms`, `GET /api/cart 200 %dms`, `POST /checkout 302 %dms`, `GET /static/app.%d.js 304 %dms`},
	"api":        {`level=info msg="request" method=GET path=/v1/orders/%d status=200 duration=%dms`, `level=info msg="request" method=POST path=/v1/orders status=201 duration=%dms`, `level=warn msg="slow query" table=orders duration=%dms`, `level=info msg="cache hit" key=product:%d`},
	"worker":     {`processed job id=%d queue=orders in %dms`, `processed job id=%d queue=email in %dms`, `retrying job id=%d attempt=2 after %dms`, `queue depth orders=%d email=%d`},
	"postgres":   {`LOG:  checkpoint starting: time`, `LOG:  checkpoint complete: wrote %d buffers (%d.0%%); 0 WAL file(s) added`, `LOG:  automatic vacuum of table "app.public.orders": index scans: %d`, `LOG:  duration: %d.412 ms  statement: SELECT * FROM orders WHERE customer_id = %d`},
	"redis":      {`* %d changes in 60 seconds. Saving...`, `* Background saving started by pid %d`, `* DB saved on disk`, `* Background saving terminated with success`},
	"mysql":      {`[Note] InnoDB: Buffer pool(s) load completed at %d`, `[Warning] Aborted connection %d to db: 'wordpress' (Got timeout reading communication packets)`, `[Note] Slave SQL thread: %d events executed`},
	"nginx":      {`10.0.%d.%d - - "GET / HTTP/2.0" 200 5124 "-" "Mozilla/5.0"`, `10.0.%d.%d - - "GET /healthz HTTP/1.1" 200 2 "-" "kube-probe/1.30"`, `10.0.%d.%d - - "POST /api/login HTTP/2.0" 401 64 "-" "curl/8.5.0"`},
	"proxy":      {`level=debug msg="Serving HTTP" routers=%d`, `time="now" level=info msg="Configuration loaded from provider docker" providerName=docker routers=%d`, `acme: renewing certificate for shop-%d.acme.example`},
	"java":       {`INFO  [main] o.s.b.w.e.tomcat.TomcatWebServer : Tomcat started on port %d`, `INFO  [http-nio-exec-%d] c.a.api.OrderController : GET /orders took %dms`, `WARN  [gc] G1 Young Generation pause %dms`},
	"prometheus": {`ts=now caller=compact.go level=info msg="write block" mint=%d duration=%dms`, `ts=now caller=head.go level=info msg="Head GC completed" duration=%dms`, `ts=now caller=scrape.go level=warn msg="Scrape target down" target=node-%d`},
	"python":     {`INFO:     10.0.0.%d:0 - "GET /api/state HTTP/1.1" 200 OK`, `INFO  scheduler: next run in %ds`, `WARNING  Retrying request after %dms`},
	"php":        {`10.0.0.%d - - "GET /wp-admin/admin-ajax.php HTTP/1.1" 200 %d`, `[php:notice] PHP Notice: Undefined index: %d`},
	"node":       {`[info] listening on :%d`, `[info] connected clients=%d`, `[warn] reconnecting to broker in %dms`},
	"media":      {`[INF] Transcoding stream %d at %dfps`, `[INF] Library scan completed in %ds`, `[WRN] Slow disk read %dms`},
	"arr":        {`[Info] RssSyncService: RSS Sync Completed. Reports found: %d, Reports grabbed: %d`, `[Info] DownloadDecisionMaker: Processing %d releases`},
	"ci":         {`INF pipeline %d step build started`, `INF pipeline %d step test passed in %ds`, `INF pulled image layer %d/%d`},
	"agent":      {`level=info msg="shipping batch" entries=%d`, `level=info msg="collected metrics" series=%d duration=%dms`},
	"erlang":     {`accepting AMQP connection 10.0.0.%d:%d`, `closing AMQP connection 10.0.0.%d:%d (client closed)`},
	"db":         {`<Information> executeQuery: Read %d rows in %dms`, `<Information> MergeTree: merged %d parts`},
	"dns":        {`query[A] api.acme.example from 10.0.0.%d`, `cached api.acme.example is 10.0.%d.%d`},
	"mail":       {`postfix/smtpd[%d]: connect from mail-%d.example.net`, `postfix/qmgr[%d]: queue active, %d messages`},
	"mqtt":       {`New client connected from 10.0.0.%d as sensor-%d`, `Received PUBLISH from sensor-%d (%d bytes)`},
	"torrent":    {`Torrent %d finished downloading in %ds`, `Announcing to tracker, peers=%d`},
}

// LogLines returns n log lines of c ending at until, one every few seconds,
// deterministic per container so scrolling back shows the same history.
func (h *Host) LogLines(c Container, until time.Time, n int) []LogLine {
	lines, ok := logLines[c.Profile]
	if !ok {
		lines = logLines["api"]
	}
	key := hashString(c.ID)
	out := make([]LogLine, 0, n)
	step := time.Duration(2+key%9) * time.Second
	start := until.Add(-step * time.Duration(n))
	if c.State != StateRunning && !c.FinishedAt.IsZero() {
		start = c.FinishedAt.Add(-step * time.Duration(n))
	}
	for i := 0; i < n; i++ {
		t := start.Add(step * time.Duration(i))
		r := newRNG(key ^ uint64(t.Unix()))
		tpl := lines[r.intn(len(lines))]
		args := make([]any, strings.Count(tpl, "%d"))
		for j := range args {
			args[j] = r.between(1, 999)
		}
		out = append(out, LogLine{Time: t, Text: fmt.Sprintf(tpl, args...), Stderr: strings.Contains(tpl, "warn") || strings.Contains(tpl, "WARN")})
	}
	if c.Error != "" {
		out = append(out, LogLine{Time: c.FinishedAt, Text: c.Error, Stderr: true})
	}
	return out
}

// LogLine is one line of simulated container output.
type LogLine struct {
	Time   time.Time
	Text   string
	Stderr bool
}
