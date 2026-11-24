package butter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapprotocol/filter/internal/pkg/stream"
)

func sampleRequest() *stream.BridgeDataRequest {
	return &stream.BridgeDataRequest{
		Entrance:        "butter+ x~*!()",
		Affiliate:       []string{"9:30", "1201:0"},
		FromChainID:     "1360108768460801",
		ToChainID:       "56",
		Amount:          "1.234567",
		TokenInAddress:  "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		TokenOutAddress: "0x55d398326f99059fF775485246999027B3197955",
		MinAmountOut:    "1180591620717411303429",
		Receiver:        "0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B",
		Caller:          "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		EntranceId:      "9",
	}
}

// want is URLSearchParams.toString() from node for the same input
func TestBridgeDataQueryMatchesURLSearchParams(t *testing.T) {
	want := "fromChainId=1360108768460801&caller=9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM&toChainId=56&amount=1.234567" +
		"&tokenInAddress=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&tokenOutAddress=0x55d398326f99059fF775485246999027B3197955" +
		"&minAmountOut=1180591620717411303429&receiver=0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B&entranceId=9" +
		"&entrance=butter%2B+x%7E*%21%28%29&affiliate=9%3A30&affiliate=1201%3A0"
	if got := BridgeDataQuery(sampleRequest()); got != want {
		t.Fatalf("query\n got %s\nwant %s", got, want)
	}
	req := sampleRequest()
	req.Entrance, req.Affiliate = "", nil
	if got := BridgeDataQuery(req); strings.Contains(got, "entrance=") || strings.Contains(got, "affiliate") {
		t.Fatalf("query %s should omit empty entrance and affiliate", got)
	}
}

func TestRequestBridgeDataWithKey(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(`{"errno":0,"message":"ok","data":[{"data":"0xdead","relay":true,"receiver":"0x01"}]}`))
	}))
	defer srv.Close()

	got, err := RequestBridgeDataWithKey(srv.URL, "test-key", sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer test-key" || path != UrlOfMessageInBridgeData {
		t.Fatalf("auth %q path %q", auth, path)
	}
	if got["data"] != "0xdead" || got["relay"] != true || got["receiver"] != "0x01" {
		t.Fatalf("data = %v", got)
	}
}

func TestRequestBridgeDataWithKeyErrors(t *testing.T) {
	if _, err := RequestBridgeDataWithKey("http://butter.invalid", "", sampleRequest()); err == nil ||
		!strings.Contains(err.Error(), "MISSING_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	for body, code := range map[string]string{
		`{"message":"x","data":[{}]}`:              "INVALID_BUTTER_RESPONSE",
		`{"statusCode":2001,"message":"no route"}`: "2001",
		`{"errno":"0","data":[{}]}`:                `"0"`,
		`{"errno":0,"data":[]}`:                    "Code=0",
		`{"statusCode":0,"data":[]}`:               "EMPTY_DATA",
		`{"statusCode":0,"data":[null]}`:           "INVALID_BUTTER_RESPONSE",
	} {
		_, err := parseBridgeData("u", []byte(body))
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Fatalf("body %s err = %v, want %s", body, err, code)
		}
	}
}
