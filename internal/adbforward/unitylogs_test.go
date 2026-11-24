package adbforward

import "testing"

func TestParseUnityLog(t *testing.T) {
	if level, text, ok := parseUnityLog("W/Unity   ( 1234):  游戏日志"); !ok || level != "Warning" || text != " 游戏日志" {
		t.Fatalf("unexpected log entry: %q %q %t", level, text, ok)
	}
	if _, _, ok := parseUnityLog("E/OtherApp( 1234): noise"); ok {
		t.Fatal("unrelated logs must be ignored")
	}
}
