package unityprofiler

import (
	"encoding/binary"
	"math"
)

// 本文件是 Profiler 原始数据流的编码器，与 rawstream.go 的解码器一一对应。
// 除供假 Player 与单测使用外，它也是各记录布局的可执行说明。

// RecordWriter 按记录追加写出一条线程流的字节内容。
type RecordWriter struct {
	buf []byte
}

// Bytes 返回已写入的字节。
func (w *RecordWriter) Bytes() []byte { return w.buf }

// Len 返回已写入长度。
func (w *RecordWriter) Len() int { return len(w.buf) }

// Reset 清空缓冲以便复用。
func (w *RecordWriter) Reset() { w.buf = w.buf[:0] }

func (w *RecordWriter) u8(v byte) { w.buf = append(w.buf, v) }

func (w *RecordWriter) u16(v uint16) {
	w.buf = binary.LittleEndian.AppendUint16(w.buf, v)
}

func (w *RecordWriter) u32(v uint32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

func (w *RecordWriter) i64(v int64) {
	w.buf = binary.LittleEndian.AppendUint64(w.buf, uint64(v))
}

func (w *RecordWriter) str(s string) {
	w.u32(uint32(len(s)))
	w.buf = append(w.buf, s...)
}

// MarkerMeta 是 marker 声明的一项元数据。
type MarkerMeta struct {
	// DataType 为 ProfilerMarkerDataType，Unit 为 ProfilerMarkerDataUnit。
	DataType byte
	Unit     byte
	Name     string
}

// ProfilerMarkerDataType 常用取值。
const (
	DataTypeInstanceID byte = 0x01
	DataTypeInt32      byte = 0x02
	DataTypeUInt32     byte = 0x03
	DataTypeInt64      byte = 0x04
	DataTypeUInt64     byte = 0x05
	DataTypeFloat      byte = 0x06
	DataTypeDouble     byte = 0x07
	DataTypeString     byte = 0x08
	DataTypeString16   byte = 0x09
	DataTypeBlob       byte = 0x0B
)

// ProfilerMarkerDataUnit 常用取值。
const (
	UnitUndefined       byte = 0
	UnitTimeNanoseconds byte = 1
	UnitBytes           byte = 2
	UnitCount           byte = 3
)

// CategoryDef 写出一条 ProfilerCategory 定义（元数据流）。
func (w *RecordWriter) CategoryDef(id uint16, name string) {
	w.u8(recMetaCategory)
	w.u8(0)
	w.u16(id)
	w.u16(0) // 图表配色序号
	w.u16(1) // 分类标志位
	w.str(name)
}

// ThreadDef 写出一条线程定义（元数据流）。
func (w *RecordWriter) ThreadDef(threadID int64, startNS int64, group, name string) {
	w.u8(recMetaThread)
	w.u8(0)
	w.i64(threadID)
	w.i64(startNS)
	w.u32(0)
	w.str(group)
	w.str(name)
}

// MarkerDef 写出一条 marker 定义（元数据流）。
func (w *RecordWriter) MarkerDef(id uint32, flags, category uint16, name string, metas ...MarkerMeta) {
	w.u8(recMetaMarker)
	w.u8(0)
	w.u32(id)
	w.u16(flags)
	w.u16(category)
	w.str(name)
	w.u8(byte(len(metas)))
	for _, m := range metas {
		w.u8(m.DataType)
		w.u8(m.Unit)
		w.str(m.Name)
	}
}

// CounterDef 写出一条 ProfilerCounter 定义（元数据流）。
func (w *RecordWriter) CounterDef(id uint32, category uint16, name string, dataType, unit byte) {
	w.MarkerDef(id, markerFlagCounter, category, name, MarkerMeta{DataType: dataType, Unit: unit})
}

// MetaFlush 写出元数据流的分段结束记录。
func (w *RecordWriter) MetaFlush(ts int64) {
	w.u8(recMetaFlush)
	w.u8(0)
	w.u32(1)
	w.i64(ts)
	w.u32(0)
}

// FrameBoundary 写出帧边界记录（事件流）：该记录之后的采样属于新的一帧。
func (w *RecordWriter) FrameBoundary(index uint32, ts int64) {
	w.u8(recFrame)
	w.u8(0)
	w.u32(index)
	w.i64(ts)
}

// Begin 写出采样开始记录（事件流）。
func (w *RecordWriter) Begin(markerID uint32, ts int64) {
	w.u8(recBeginPlain)
	w.u16(0)
	w.u32(markerID)
	w.i64(ts)
}

// End 写出采样结束记录（事件流）。
func (w *RecordWriter) End(markerID uint32, ts int64) {
	w.u8(recEnd)
	w.u16(0)
	w.u32(markerID)
	w.i64(ts)
}

// CounterInt64 写出一条 Int64 计数器取值（事件流）。
func (w *RecordWriter) CounterInt64(markerID uint32, ts int64, v int64) {
	w.counterHead(markerID, ts)
	w.u8(DataTypeInt64)
	w.i64(v)
}

// CounterInt32 写出一条 Int32 计数器取值（事件流）。
func (w *RecordWriter) CounterInt32(markerID uint32, ts int64, v int32) {
	w.counterHead(markerID, ts)
	w.u8(DataTypeInt32)
	w.u32(uint32(v))
}

// CounterDouble 写出一条 Double 计数器取值（事件流）。
func (w *RecordWriter) CounterDouble(markerID uint32, ts int64, v float64) {
	w.counterHead(markerID, ts)
	w.u8(DataTypeDouble)
	w.buf = binary.LittleEndian.AppendUint64(w.buf, math.Float64bits(v))
}

func (w *RecordWriter) counterHead(markerID uint32, ts int64) {
	w.u8(recCounter)
	w.u16(0)
	w.u32(markerID)
	w.i64(ts)
	w.u8(1) // 取值个数
}

// EncodeStreamHeader 生成 56 字节流头。
func EncodeStreamHeader(h StreamHeader, major, minor, patch, releaseType, releaseNumber uint32) []byte {
	b := make([]byte, streamHeaderSize)
	le := binary.LittleEndian
	le.PutUint32(b[0:4], streamMagic)
	le.PutUint16(b[4:6], 1)
	le.PutUint16(b[6:8], 0x0B)
	le.PutUint32(b[8:12], h.FormatVersion)
	le.PutUint32(b[12:16], 1)
	le.PutUint32(b[20:24], 1)
	le.PutUint64(b[28:36], uint64(h.MainThreadID))
	le.PutUint32(b[36:40], major)
	le.PutUint32(b[40:44], minor)
	le.PutUint32(b[44:48], patch)
	le.PutUint32(b[48:52], releaseType)
	le.PutUint32(b[52:56], releaseNumber)
	return b
}
