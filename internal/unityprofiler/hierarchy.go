package unityprofiler

import (
	"sort"
	"strings"
	"unicode/utf16"

	"unityperfagent/internal/protocol"
)

const (
	maxHierarchyNodes = 1200
	nsPerMSHierarchy  = 1e6
)

func cloneValues(in []value) []value {
	if len(in) == 0 {
		return nil
	}
	out := make([]value, len(in))
	for i, v := range in {
		raw := make([]byte, len(v.Raw))
		copy(raw, v.Raw)
		out[i] = value{Type: v.Type, Raw: raw}
	}
	return out
}

func buildHierarchy(d *Decoder, roots []*treeNode) []protocol.SampleNode {
	nodes := treeToSamples(d, roots)
	nodes = mergeSampleNodes(nodes)
	budget := maxHierarchyNodes
	return capSampleNodes(nodes, &budget)
}

func treeToSamples(d *Decoder, nodes []*treeNode) []protocol.SampleNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]protocol.SampleNode, 0, len(nodes))
	for _, n := range nodes {
		name, cat := "Sample", ""
		if info, ok := d.meta.markers[n.markerID]; ok {
			name = info.Name
			cat = d.meta.categoryName(info.Category)
		}
		s := protocol.SampleNode{
			Name:     name,
			Category: cat,
			TotalMs:  float32(float64(n.totalNS) / nsPerMSHierarchy),
			SelfMs:   float32(float64(n.selfNS) / nsPerMSHierarchy),
			Calls:    1,
			Children: treeToSamples(d, n.kids),
		}
		for _, v := range n.meta {
			switch v.Type {
			case DataTypeString:
				s.Callstack = mergeCallstack(s.Callstack, splitCallstack(string(v.Raw)))
			case DataTypeString16:
				s.Callstack = mergeCallstack(s.Callstack, splitCallstack(utf16BytesToString(v.Raw)))
			default:
				if num, ok := v.Number(); ok && strings.EqualFold(name, "GC.Alloc") {
					s.GCAllocKB = float32(num / bytesPerKB)
				}
			}
		}
		out = append(out, s)
	}
	return out
}

func splitCallstack(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mergeCallstack(dst, src []string) []string {
	if len(src) == 0 {
		return dst
	}
	if len(dst) == 0 {
		return src
	}
	return append(dst, src...)
}

func utf16BytesToString(b []byte) string {
	if len(b) < 2 {
		return string(b)
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u))
}

func mergeSampleNodes(nodes []protocol.SampleNode) []protocol.SampleNode {
	if len(nodes) == 0 {
		return nil
	}
	type acc struct {
		node protocol.SampleNode
		kids []protocol.SampleNode
	}
	order := make([]string, 0, len(nodes))
	byKey := make(map[string]*acc, len(nodes))
	for _, n := range nodes {
		key := n.Name + "\x00" + n.Category
		a, ok := byKey[key]
		if !ok {
			cp := n
			cp.Children = nil
			a = &acc{node: cp}
			byKey[key] = a
			order = append(order, key)
		} else {
			a.node.TotalMs += n.TotalMs
			a.node.SelfMs += n.SelfMs
			a.node.Calls += n.Calls
			a.node.GCAllocKB += n.GCAllocKB
			if len(a.node.Callstack) == 0 && len(n.Callstack) > 0 {
				a.node.Callstack = n.Callstack
			}
		}
		if len(n.Children) > 0 {
			a.kids = append(a.kids, n.Children...)
		}
	}
	out := make([]protocol.SampleNode, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		a.node.Children = mergeSampleNodes(a.kids)
		out = append(out, a.node)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TotalMs > out[j].TotalMs })
	return out
}

func capSampleNodes(nodes []protocol.SampleNode, budget *int) []protocol.SampleNode {
	if *budget <= 0 || len(nodes) == 0 {
		return nil
	}
	out := make([]protocol.SampleNode, 0, len(nodes))
	for _, n := range nodes {
		if *budget <= 0 {
			break
		}
		*budget--
		n.Children = capSampleNodes(n.Children, budget)
		out = append(out, n)
	}
	return out
}
