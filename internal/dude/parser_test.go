package dude

import (
	"testing"
)

func TestParseDetailPrint_TwoRecords(t *testing.T) {
	in := `Flags: X - disabled
 0  name="AP1" address=10.0.0.1 mac-address=AA:BB:CC:11:22:33 type=mikrotik
    info-from-mac=true
 1  name="LINK-CORE" address=10.0.0.2 type=generic`
	got := ParseDetailPrint(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0]["name"] != "AP1" {
		t.Errorf("record0.name = %q, want AP1", got[0]["name"])
	}
	if got[0]["mac-address"] != "AA:BB:CC:11:22:33" {
		t.Errorf("record0.mac = %q", got[0]["mac-address"])
	}
	if got[0]["info-from-mac"] != "true" {
		t.Errorf("continuation line missing: %q", got[0]["info-from-mac"])
	}
	if got[1]["name"] != "LINK-CORE" {
		t.Errorf("record1.name = %q", got[1]["name"])
	}
}

func TestParseDetailPrint_QuotedSpaces(t *testing.T) {
	in := ` 0 name="My AP With Spaces" type=ap`
	got := ParseDetailPrint(in)
	if len(got) != 1 || got[0]["name"] != "My AP With Spaces" {
		t.Fatalf("quoted-value lost spaces: %+v", got)
	}
}

func TestParseDetailPrint_FlagPrefix(t *testing.T) {
	in := ` 0 X D name="DISABLED-AP" type=ap`
	got := ParseDetailPrint(in)
	if len(got) != 1 || got[0]["name"] != "DISABLED-AP" {
		t.Fatalf("flag prefix not stripped: %+v", got)
	}
}

func TestParseDetailPrint_EmptyAndComments(t *testing.T) {
	in := `# header
Flags: X - disabled

 0 name="A" type=router

 1 name="B" type=switch
`
	got := ParseDetailPrint(in)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d (%+v)", len(got), got)
	}
}

func TestParseSimplePrint(t *testing.T) {
	in := ` # NAME ADDRESS
 0 ap1 10.0.0.1
 1 cpe1 10.0.0.2`
	got := ParseSimplePrint(in)
	if len(got) != 2 || got[0][0] != "ap1" || got[1][1] != "10.0.0.2" {
		t.Fatalf("unexpected rows: %+v", got)
	}
}
