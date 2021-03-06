package collector

import (
	"runtime"
	"sync"
	"time"
)

// FieldsFunc represents a callback after successfully gathering statistics
type FieldsFunc func(Fields)

// Collector implements the periodic grabbing of informational data from the
// runtime package and outputting the values to a GaugeFunc.
type Collector struct {
	// PauseDur represents the interval in-between each set of stats output.
	// Defaults to 10 seconds.
	PauseDur time.Duration

	// EnableCPU determines whether CPU statistics will be output. Defaults to true.
	EnableCPU bool

	// EnableMem determines whether memory statistics will be output. Defaults to true.
	EnableMem bool

	// EnableGC determines whether garbage collection statistics will be output. EnableMem
	// must also be set to true for this to take affect. Defaults to true.
	EnableGC bool

	// Done, when closed, is used to signal Collector that is should stop collecting
	// statistics and the Run function should return.
	Done <-chan struct{}

	fieldsFunc FieldsFunc

	fields Fields

	mu sync.RWMutex
}

// New creates a new Collector that will periodically output statistics to fieldsFunc. It
// will also set the values of the exported fields to the described defaults. The values
// of the exported defaults can be changed at any point before Run is called.
func New(fieldsFunc FieldsFunc) *Collector {
	if fieldsFunc == nil {
		fieldsFunc = func(Fields) {}
	}

	return &Collector{
		PauseDur:   10 * time.Second,
		EnableCPU:  true,
		EnableMem:  true,
		EnableGC:   true,
		fieldsFunc: fieldsFunc,
	}
}

// Run gathers statistics then outputs them to the configured PointFunc every
// PauseDur. Unlike OneOff, this function will return until Done has been closed
// (or never if Done is nil), therefore it should be called in its own go routine.
func (c *Collector) Run() {
	c.outputStats()

	tick := time.NewTicker(c.PauseDur)
	defer tick.Stop()
	for {
		select {
		case <-c.Done:
			return
		case <-tick.C:
			c.outputStats()
		}
	}
}

// OneOff gathers returns a map containing all statistics. It is safe for use from
// multiple go routines
func (c *Collector) OneOff() Fields {
	c.outputStats()

	c.mu.Lock()
	defer func() {
		c.fields = Fields{}
		c.mu.Unlock()
	}()
	return c.fields
}

func (c *Collector) outputStats() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.EnableCPU {
		cStats := cpuStats{
			NumGoroutine: int64(runtime.NumGoroutine()),
			NumCgoCall:   int64(runtime.NumCgoCall()),
		}
		c.outputCPUStats(&cStats)
	}
	if c.EnableMem {
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		c.outputMemStats(m)
		if c.EnableGC {
			c.outputGCStats(m)
		}
	}

	c.fieldsFunc(c.fields)
}

func (c *Collector) outputCPUStats(s *cpuStats) {
	c.fields.NumGoroutine = int64(s.NumGoroutine)
	c.fields.NumCgoCall = int64(s.NumCgoCall)
}

func (c *Collector) outputMemStats(m *runtime.MemStats) {
	// General
	c.fields.Alloc = int64(m.Alloc)
	c.fields.TotalAlloc = int64(m.TotalAlloc)
	c.fields.Sys = int64(m.Sys)
	c.fields.Lookups = int64(m.Lookups)
	c.fields.Mallocs = int64(m.Mallocs)
	c.fields.Frees = int64(m.Frees)

	// Heap
	c.fields.HeapAlloc = int64(m.HeapAlloc)
	c.fields.HeapSys = int64(m.HeapSys)
	c.fields.HeapIdle = int64(m.HeapIdle)
	c.fields.HeapInuse = int64(m.HeapInuse)
	c.fields.HeapReleased = int64(m.HeapReleased)
	c.fields.HeapObjects = int64(m.HeapObjects)

	// Stack
	c.fields.StackInuse = int64(m.StackInuse)
	c.fields.StackSys = int64(m.StackSys)
	c.fields.MSpanInuse = int64(m.MSpanInuse)
	c.fields.MSpanSys = int64(m.MSpanSys)
	c.fields.MCacheInuse = int64(m.MCacheInuse)
	c.fields.MCacheSys = int64(m.MCacheSys)

	c.fields.OtherSys = int64(m.OtherSys)
}

func (c *Collector) outputGCStats(m *runtime.MemStats) {
	c.fields.GCSys = int64(m.GCSys)
	c.fields.NextGC = int64(m.NextGC)
	c.fields.LastGC = int64(m.LastGC)
	c.fields.PauseTotalNs = int64(m.PauseTotalNs)
	c.fields.PauseNs = int64(m.PauseNs[(m.NumGC+255)%256])
	c.fields.NumGC = int64(m.NumGC)
	c.fields.GCCPUFraction = float64(m.GCCPUFraction)
}

type cpuStats struct {
	NumGoroutine int64
	NumCgoCall   int64
}

// NOTE: uint64 is not supported by influxDB client due to potential overflows
type Fields struct {
	// CPU
	NumGoroutine int64 `json:"cpu.goroutines"`
	NumCgoCall   int64 `json:"cpu.cgo_calls"`

	// General
	Alloc      int64 `json:"mem.alloc"`
	TotalAlloc int64 `json:"mem.total"`
	Sys        int64 `json:"mem.sys"`
	Lookups    int64 `json:"mem.lookups"`
	Mallocs    int64 `json:"mem.malloc"`
	Frees      int64 `json:"mem.frees"`

	// Heap
	HeapAlloc    int64 `json:"mem.heap.alloc"`
	HeapSys      int64 `json:"mem.heap.sys"`
	HeapIdle     int64 `json:"mem.heap.idle"`
	HeapInuse    int64 `json:"mem.heap.inuse"`
	HeapReleased int64 `json:"mem.heap.released"`
	HeapObjects  int64 `json:"mem.heap.objects"`

	// Stack
	StackInuse  int64 `json:"mem.stack.inuse"`
	StackSys    int64 `json:"mem.stack.sys"`
	MSpanInuse  int64 `json:"mem.stack.mspan_inuse"`
	MSpanSys    int64 `json:"mem.stack.mspan_sys"`
	MCacheInuse int64 `json:"mem.stack.mcache_inuse"`
	MCacheSys   int64 `json:"mem.stack.mcache_sys"`

	OtherSys int64 `json:"mem.othersys"`

	// GC
	GCSys         int64   `json:"mem.gc.sys"`
	NextGC        int64   `json:"mem.gc.next"`
	LastGC        int64   `json:"mem.gc.last"`
	PauseTotalNs  int64   `json:"mem.gc.pause_total"`
	PauseNs       int64   `json:"mem.gc.pause"`
	NumGC         int64   `json:"mem.gc.count"`
	GCCPUFraction float64 `json:"mem.gc.cpu_fraction"`
}

func (f *Fields) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"cpu.goroutines": f.NumGoroutine,
		"cpu.cgo_calls":  f.NumCgoCall,

		"mem.alloc":   f.Alloc,
		"mem.total":   f.TotalAlloc,
		"mem.sys":     f.Sys,
		"mem.lookups": f.Lookups,
		"mem.malloc":  f.Mallocs,
		"mem.frees":   f.Frees,

		"mem.heap.alloc":    f.HeapAlloc,
		"mem.heap.sys":      f.HeapSys,
		"mem.heap.idle":     f.HeapIdle,
		"mem.heap.inuse":    f.HeapInuse,
		"mem.heap.released": f.HeapReleased,
		"mem.heap.objects":  f.HeapObjects,

		"mem.stack.inuse":        f.StackInuse,
		"mem.stack.sys":          f.StackSys,
		"mem.stack.mspan_inuse":  f.MSpanInuse,
		"mem.stack.mspan_sys":    f.MSpanSys,
		"mem.stack.mcache_inuse": f.MCacheInuse,
		"mem.stack.mcache_sys":   f.MCacheSys,
		"mem.othersys":           f.OtherSys,

		"mem.gc.sys":          f.GCSys,
		"mem.gc.next":         f.NextGC,
		"mem.gc.last":         f.LastGC,
		"mem.gc.pause_total":  f.PauseTotalNs,
		"mem.gc.pause":        f.PauseNs,
		"mem.gc.count":        f.NumGC,
		"mem.gc.cpu_fraction": float64(f.GCCPUFraction),
	}
}
