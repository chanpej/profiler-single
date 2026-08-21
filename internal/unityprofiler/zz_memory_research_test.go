package unityprofiler

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

type researchDataFrame struct {
	body  []byte
	stats int
}

type researchScore struct {
	offset                           int
	typ                              string
	corr, slope, intercept, maxResid float64
}

func TestZZMemoryResearch(t *testing.T) {
	b, err := os.ReadFile("../../test/same-session.uppc")
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "UPPC" {
		t.Fatal("not UPPC")
	}
	var bodies [][]byte
	for p := 8; p < len(b); {
		n := int(binary.LittleEndian.Uint32(b[p:]))
		p += 4
		bodies = append(bodies, b[p:p+n])
		p += n
	}
	var snaps []FrameSnapshot
	d := NewDecoder(func(s FrameSnapshot) { snaps = append(snaps, s) })
	for _, body := range bodies {
		if err := d.Feed(body); err != nil {
			t.Fatal(err)
		}
	}

	data, err := researchReadData("../../test/testdata/unity-profiler-proxy.data")
	if err != nil {
		t.Fatal(err)
	}
	const warmup = 21
	if len(snaps) < len(data)+warmup {
		t.Fatalf("snaps %d data %d", len(snaps), len(data))
	}
	t.Logf("snapshots=%d data=%d first stats body offset=%d, AllProfilerStats-relative offset=0", len(snaps), len(data), data[0].stats)

	keys := map[string]bool{}
	for _, s := range snaps[warmup : warmup+len(data)] {
		for k := range s.Counters {
			if strings.HasPrefix(k, "Memory/") {
				keys[k] = true
			}
		}
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		x := make([]float64, len(data))
		for i := range data {
			x[i] = snaps[i+warmup].Counters[name]
		}
		best := make([]researchScore, 0)
		exact := make([]string, 0)
		for _, typ := range []string{"u32", "i32", "u64", "i64", "f32", "f64"} {
			sz := 4
			if typ == "u64" || typ == "i64" || typ == "f64" {
				sz = 8
			}
			for off := 0; off+sz <= data[0].stats+1060; off += 4 {
				y := make([]float64, len(data))
				ok := true
				for i, f := range data {
					y[i], ok = researchValue(f.body[off:], typ)
					if !ok {
						break
					}
				}
				if !ok {
					continue
				}
				corr, slope, intercept, maxr := researchFit(x, y)
				if math.Abs(corr) > .99999 && maxr < 4 {
					best = append(best, researchScore{off, typ, corr, slope, intercept, maxr})
				}
				for _, div := range []float64{1, 1024, 1024 * 1024} {
					var max float64
					for i := range x {
						if v := math.Abs(y[i] - x[i]/div); v > max {
							max = v
						}
					}
					if max < .5 {
						exact = append(exact, fmt.Sprintf("body+%d/All%+d %s=x/%.0f max=%.3g", off, off-data[0].stats, typ, div, max))
					}
				}
			}
		}
		sort.Slice(best, func(i, j int) bool { return math.Abs(best[i].corr) > math.Abs(best[j].corr) })
		if len(best) > 4 {
			best = best[:4]
		}
		t.Logf("%s raw first=%.0f last=%.0f exact=%v correlated=%+v", name, x[0], x[len(x)-1], exact, best)
	}
}

func researchReadData(path string) ([]researchDataFrame, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []researchDataFrame
	for p := 0; p < len(b); {
		if p+4 > len(b) {
			return nil, fmt.Errorf("truncated")
		}
		if binary.LittleEndian.Uint32(b[p:]) == 0xDEADFEED {
			break
		}
		if p+28 > len(b) {
			return nil, fmt.Errorf("header")
		}
		n := int(binary.LittleEndian.Uint32(b[p+4:]))
		p += 28
		if p+n > len(b) {
			return nil, fmt.Errorf("body")
		}
		body := b[p : p+n]
		p += n
		if binary.LittleEndian.Uint32(body[16:]) == 0 {
			continue
		}
		q := 28 + 33*4 + 4
		for binary.LittleEndian.Uint32(body[q:]) != 0xFFFFFFFF {
			q += 8
		}
		q += 4 + 16*4
		if q+1060 > len(body) {
			return nil, fmt.Errorf("stats")
		}
		out = append(out, researchDataFrame{body, q})
	}
	return out, nil
}
func researchValue(b []byte, typ string) (float64, bool) {
	switch typ {
	case "u32":
		return float64(binary.LittleEndian.Uint32(b)), true
	case "i32":
		return float64(int32(binary.LittleEndian.Uint32(b))), true
	case "u64":
		return float64(binary.LittleEndian.Uint64(b)), true
	case "i64":
		return float64(int64(binary.LittleEndian.Uint64(b))), true
	case "f32":
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case "f64":
		v := math.Float64frombits(binary.LittleEndian.Uint64(b))
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	return 0, false
}
func researchFit(x, y []float64) (corr, slope, intercept, maxr float64) {
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(len(x))
	my /= float64(len(x))
	var xx, yy, xy float64
	for i := range x {
		a := x[i] - mx
		b := y[i] - my
		xx += a * a
		yy += b * b
		xy += a * b
	}
	if xx == 0 || yy == 0 {
		return 0, 0, 0, 0
	}
	slope = xy / xx
	intercept = my - slope*mx
	corr = xy / math.Sqrt(xx*yy)
	for i := range x {
		r := math.Abs(y[i] - (slope*x[i] + intercept))
		if r > maxr {
			maxr = r
		}
	}
	return
}
