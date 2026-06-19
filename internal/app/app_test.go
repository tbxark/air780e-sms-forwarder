package app

import "testing"

func TestDecodeSMSPDU_GSM7(t *testing.T) {
	sms, err := decodeSMSPDU("07915892200417F5240BA14180889969F100006260918022320004F4F29C0E", 23)
	if err != nil {
		t.Fatal(err)
	}
	if sms.From != "14088899961" {
		t.Fatalf("from = %q", sms.From)
	}
	if sms.Text != "test" {
		t.Fatalf("text = %q", sms.Text)
	}
}

func TestDecodeSMSPDURejectsLengthMismatch(t *testing.T) {
	_, err := decodeSMSPDU("07915892200417F5240BA14180889969F100006260918022320004F4F29C0E", 24)
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func TestCMTPDUHeader(t *testing.T) {
	cases := map[string]string{
		`+CMT:,23`:       "23",
		`+CMT: , 23`:     "23",
		`+CMT:"",23`:     "23",
		`+CMT:"alpha",5`: "5",
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			got := cmtPDUHeaderRE.FindStringSubmatch(line)
			if len(got) != 2 {
				t.Fatalf("expected match")
			}
			if got[1] != want {
				t.Fatalf("length = %q", got[1])
			}
		})
	}

	if cmtPDUHeaderRE.MatchString(`+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"`) {
		t.Fatal("text-mode CMT header should not match PDU header")
	}
}

func TestParseCMTIndicationText(t *testing.T) {
	sms, err := parseCMTIndication([]string{
		`+CMT: "+86138xxxx0000","","26/06/19,16:30:00+32"`,
		"Your code is 123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sms.From != "+86138xxxx0000" {
		t.Fatalf("from = %q", sms.From)
	}
	if sms.Text != "Your code is 123456" {
		t.Fatalf("text = %q", sms.Text)
	}
}

func TestParseCMTIndicationPDU(t *testing.T) {
	sms, err := parseCMTIndication([]string{
		`+CMT:,23`,
		"07915892200417F5240BA14180889969F100006260918022320004F4F29C0E",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sms.From != "14088899961" {
		t.Fatalf("from = %q", sms.From)
	}
	if sms.Text != "test" {
		t.Fatalf("text = %q", sms.Text)
	}
}

func TestRankSerialCandidates(t *testing.T) {
	candidates := rankSerialCandidates([]serialCandidate{
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
	if scorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= scorePortName("/dev/ttyACM0") {
		t.Fatal("expected EigenComm by-id port to score higher")
	}
	if scorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if03") <= scorePortName("/dev/serial/by-id/usb-EigenComm_Compo-if05") {
		t.Fatal("expected if03 to score higher than if05")
	}
}

func TestBuildNotifiers(t *testing.T) {
	if got := buildNotifiers(Config{}); len(got) != 0 {
		t.Fatalf("len = %d", len(got))
	}

	got := buildNotifiers(Config{TelegramToken: "token", TelegramChat: "chat"})
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name() != "Telegram" {
		t.Fatalf("name = %q", got[0].Name())
	}
}
