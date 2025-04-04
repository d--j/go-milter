package milter

import "testing"

func TestOptAction_String(t *testing.T) {
	tests := []struct {
		name string
		o    OptAction
		want string
	}{
		{"OptAddHeader", OptAddHeader, "OptAddHeader"},
		{"OptChangeBody", OptChangeBody, "OptChangeBody"},
		{"OptAddRcpt", OptAddRcpt, "OptAddRcpt"},
		{"OptRemoveRcpt", OptRemoveRcpt, "OptRemoveRcpt"},
		{"OptChangeHeader", OptChangeHeader, "OptChangeHeader"},
		{"OptQuarantine", OptQuarantine, "OptQuarantine"},
		{"OptChangeFrom", OptChangeFrom, "OptChangeFrom"},
		{"OptAddRcptWithArgs", OptAddRcptWithArgs, "OptAddRcptWithArgs"},
		{"OptSetMacros", OptSetMacros, "OptSetMacros"},
		{"multiple bits", OptQuarantine | OptChangeBody, "OptChangeBody|OptQuarantine"},
		{"unknown bits", OptQuarantine | OptAction(1<<21), "OptQuarantine|unknown bit 21"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.o.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOptProtocol_String(t *testing.T) {
	tests := []struct {
		name string
		o    OptProtocol
		want string
	}{
		{"OptNoConnect", OptNoConnect, "OptNoConnect"},
		{"OptNoHelo", OptNoHelo, "OptNoHelo"},
		{"OptNoMailFrom", OptNoMailFrom, "OptNoMailFrom"},
		{"OptNoRcptTo", OptNoRcptTo, "OptNoRcptTo"},
		{"OptNoBody", OptNoBody, "OptNoBody"},
		{"OptNoHeaders", OptNoHeaders, "OptNoHeaders"},
		{"OptNoEOH", OptNoEOH, "OptNoEOH"},
		{"OptNoHeaderReply", OptNoHeaderReply, "OptNoHeaderReply"},
		{"OptNoUnknown", OptNoUnknown, "OptNoUnknown"},
		{"OptNoData", OptNoData, "OptNoData"},
		{"OptSkip", OptSkip, "OptSkip"},
		{"OptRcptRej", OptRcptRej, "OptRcptRej"},
		{"OptNoConnReply", OptNoConnReply, "OptNoConnReply"},
		{"OptNoHeloReply", OptNoHeloReply, "OptNoHeloReply"},
		{"OptNoMailReply", OptNoMailReply, "OptNoMailReply"},
		{"OptNoRcptReply", OptNoRcptReply, "OptNoRcptReply"},
		{"OptNoDataReply", OptNoDataReply, "OptNoDataReply"},
		{"OptNoUnknownReply", OptNoUnknownReply, "OptNoUnknownReply"},
		{"OptNoEOHReply", OptNoEOHReply, "OptNoEOHReply"},
		{"OptNoBodyReply", OptNoBodyReply, "OptNoBodyReply"},
		{"OptHeaderLeadingSpace", OptHeaderLeadingSpace, "OptHeaderLeadingSpace"},
		{"optMds256K", OptProtocol(optMds256K), "optMds256K"},
		{"optMds1M", OptProtocol(optMds1M), "optMds1M"},
		{"unused-internal", OptProtocol(1 << 30), "internal bit 30"},
		{"multiple bits", OptNoMailReply | OptNoEOH, "OptNoEOH|OptNoMailReply"},
		{"unknown bits", OptNoMailReply | OptProtocol(1<<27), "OptNoMailReply|unknown bit 27"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.o.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
