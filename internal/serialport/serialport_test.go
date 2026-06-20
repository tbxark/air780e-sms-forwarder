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

func TestScorePortNameCountsMarkerOnce(t *testing.T) {
	air780 := ScorePortName("/dev/serial/by-id/usb-Air780_Compo")
	air780e := ScorePortName("/dev/serial/by-id/usb-Air780E_Compo")
	if air780 != air780e {
		t.Fatalf("air780 score = %d, air780e score = %d, want equal marker score", air780, air780e)
	}

	multiple := ScorePortName("/dev/serial/by-id/usb-EigenComm_Air780E_Luat-if03")
	single := ScorePortName("/dev/serial/by-id/usb-EigenComm-if03")
	if multiple != single {
		t.Fatalf("multiple marker score = %d, single marker score = %d, want equal", multiple, single)
	}
}

func TestScoreLinuxTTYInfoCountsRepeatedMarkerEvidenceOnce(t *testing.T) {
	score := scoreLinuxTTYInfo(linuxTTYUSBInfo{
		Manufacturer:    "EigenComm",
		Product:         "Air780E",
		Interface:       "Luat AT",
		InterfaceNumber: 3,
	})
	want := 100 + 30
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}
}
