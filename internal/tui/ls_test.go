package tui

import (
	"errors"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/ls"
)

func TestLSToMsg(t *testing.T) {
	cases := []struct {
		name string
		ev   ls.Event
		want any
	}{
		{"news", ls.News{Title: "제목"}, NewsMsg{Title: "제목"}},
		{"kospi", ls.Index{Code: "001", Value: 2712.4, ChangePct: 0.008}, IndexMsg{Market: "kospi", Value: 2712.4, ChangePct: 0.008}},
		{"kosdaq", ls.Index{Code: "301", Value: 782.15, ChangePct: -0.004}, IndexMsg{Market: "kosdaq", Value: 782.15, ChangePct: -0.004}},
		{"unknown-index", ls.Index{Code: "999"}, nil},
		{"connected", ls.Connected{}, ConnectedMsg{}},
		{"disconnected", ls.Disconnected{Err: errors.New("x")}, DisconnectedMsg{}},
		{"subscribe-error", ls.SubscribeError{TrCd: "NWS", Code: "E001"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lsToMsg(tc.ev)
			if got != tc.want {
				t.Errorf("lsToMsg(%+v) = %#v, want %#v", tc.ev, got, tc.want)
			}
		})
	}
}

func TestLSSubscriptions(t *testing.T) {
	want := []ls.Subscription{{TrCd: "NWS", TrKey: "NWS001"}, {TrCd: "IJ_", TrKey: "001"}, {TrCd: "IJ_", TrKey: "301"}}
	if len(lsSubscriptions) != len(want) {
		t.Fatalf("subs = %v", lsSubscriptions)
	}
	for i := range want {
		if lsSubscriptions[i] != want[i] {
			t.Errorf("sub %d = %v, want %v", i, lsSubscriptions[i], want[i])
		}
	}
}
