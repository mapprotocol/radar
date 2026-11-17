package config

import (
	"encoding/json"
	"testing"
)

func TestOptAcceptsStringMcsAndStartBlock(t *testing.T) {
	var o opt
	if err := json.Unmarshal([]byte(`{"mcs":"a,b","startBlock":"100","event":"x","butter":"u"}`), &o); err != nil {
		t.Fatal(err)
	}
	if o.Mcs != "a,b" || o.StartBlock != "100" || o.Event != "x" || o.Butter != "u" || o.StartBlocks != nil {
		t.Fatalf("opt = %+v", o)
	}
}

func TestOptAcceptsSolanaShape(t *testing.T) {
	var o opt
	data := `{"mcs":["p1","p2"],"startBlock":{"p1":"s1","p2":"s2"},"butterEntrance":"butter+",
		"supportedBridgeMints":["m1"],"nativeTokenOutAddress":"0x0000000000000000000000000000000000000000"}`
	if err := json.Unmarshal([]byte(data), &o); err != nil {
		t.Fatal(err)
	}
	if o.Mcs != "p1,p2" || o.StartBlock != "" || o.StartBlocks["p1"] != "s1" || o.StartBlocks["p2"] != "s2" {
		t.Fatalf("opt = %+v", o)
	}
	if o.ButterEntrance != "butter+" || len(o.SupportedBridgeMints) != 1 || o.NativeTokenOutAddress == "" {
		t.Fatalf("opt = %+v", o)
	}
}

func TestOptRejectsBadMcs(t *testing.T) {
	var o opt
	if err := json.Unmarshal([]byte(`{"mcs":1}`), &o); err == nil {
		t.Fatal("want error")
	}
}
