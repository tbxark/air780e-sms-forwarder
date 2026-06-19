package serialport

import "testing"

func TestRankCandidates(t *testing.T) {
	candidates := RankCandidates([]Candidate{
		{Port: "/dev/ttyACM2", Source: "glob", Score: 10},
		{Port: "/dev/ttyACM0", Source: "by-id:usb-EigenComm_Compo-if03", Score: 190},
		{Port: "/dev/ttyACM1", Source: "by-id:usb-EigenComm_Compo-if05", Score: 170},
		{Port: "/dev/ttyACM0", Source: "glob", Score: 10},
	})
	if len(candidates) != 3 {
		t.Fatalf("len = %d", len(candidates))
	}
	if candidates[0].Port != "/dev/ttyACM0" {
		t.Fatalf("first port = %q", candidates[0].Port)
	}
	if candidates[0].Source != "by-id:usb-EigenComm_Compo-if03" {
		t.Fatalf("first source = %q", candidates[0].Source)
	}
}

func TestScorePortName(t *testing.T) {
	if ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= ScorePortName("/dev/ttyACM0") {
		t.Fatal("expected EigenComm by-id port to score higher")
	}
	if ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= ScorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if05") {
		t.Fatal("expected if03 to score higher than if05")
	}
}
