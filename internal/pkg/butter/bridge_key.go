package butter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mapprotocol/filter/internal/pkg/stream"
)

var keyedClient = &http.Client{Timeout: time.Minute}

// RequestBridgeDataWithKey ports compass-ts requestBridgeData: bearer api key, entranceId, and the
// first bridge item returned as decoded JSON so callers can copy its fields verbatim.
func RequestBridgeDataWithKey(domain, apiKey string, request *stream.BridgeDataRequest) (map[string]interface{}, error) {
	endpoint := domain + UrlOfMessageInBridgeData
	if apiKey == "" {
		return nil, &stream.ExternalRequestError{URL: endpoint, Msg: "Butter backend API key is required", Code: "MISSING_API_KEY"}
	}

	fullURL := endpoint + "?" + BridgeDataQuery(request)
	fmt.Printf("request butter bridge data url: %s\n", fullURL)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := keyedClient.Do(req)
	if err != nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: httpErrorMessage(resp.Status, body), Code: strconv.Itoa(resp.StatusCode)}
	}
	return parseBridgeData(fullURL, body)
}

func parseBridgeData(fullURL string, body []byte) (map[string]interface{}, error) {
	var ret struct {
		Errno      json.RawMessage `json:"errno"`
		StatusCode json.RawMessage `json:"statusCode"`
		Message    interface{}     `json:"message"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &ret); err != nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: err.Error()}
	}
	msg := func(def string) string {
		if s, ok := ret.Message.(string); ok && s != "" {
			return s
		}
		return def
	}
	if ret.StatusCode == nil && ret.Errno == nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: "Butter response missing statusCode/errno", Code: "INVALID_BUTTER_RESPONSE"}
	}
	if ret.StatusCode != nil && !isZero(ret.StatusCode) {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: msg("Butter request failed"), Code: string(ret.StatusCode)}
	}
	if ret.Errno != nil && !isZero(ret.Errno) {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: msg("Butter request failed"), Code: string(ret.Errno)}
	}

	var items []json.RawMessage
	_ = json.Unmarshal(ret.Data, &items)
	if len(items) == 0 {
		code := "EMPTY_DATA"
		if ret.Errno != nil {
			code = string(ret.Errno)
		}
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: msg("Transaction data not found"), Code: code,
			Detail: fmt.Errorf("transaction data not found")}
	}

	var first map[string]interface{}
	dec := json.NewDecoder(bytes.NewReader(items[0]))
	dec.UseNumber()
	if err := dec.Decode(&first); err != nil || first == nil {
		return nil, &stream.ExternalRequestError{URL: fullURL, Msg: "Butter bridge data is not an object", Code: "INVALID_BUTTER_RESPONSE"}
	}
	fmt.Printf("request butter bridge back data: %s\n", string(items[0]))
	return first, nil
}

// BridgeDataQuery keeps the parameter order and form encoding of URLSearchParams in compass-ts.
func BridgeDataQuery(request *stream.BridgeDataRequest) string {
	pairs := [][2]string{
		{"fromChainId", request.FromChainID},
		{"caller", request.Caller},
		{"toChainId", request.ToChainID},
		{"amount", request.Amount},
		{"tokenInAddress", request.TokenInAddress},
		{"tokenOutAddress", request.TokenOutAddress},
		{"minAmountOut", request.MinAmountOut},
		{"receiver", request.Receiver},
		{"entranceId", request.EntranceId},
	}
	if request.Entrance != "" {
		pairs = append(pairs, [2]string{"entrance", request.Entrance})
	}
	for _, a := range request.Affiliate {
		pairs = append(pairs, [2]string{"affiliate", a})
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, formEscape(p[0])+"="+formEscape(p[1]))
	}
	return strings.Join(parts, "&")
}

// formEscape is application/x-www-form-urlencoded as browsers and node implement it.
func formEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '*', c == '-', c == '.', c == '_':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isZero(raw json.RawMessage) bool {
	f, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	return err == nil && f == 0
}

func httpErrorMessage(status string, body []byte) string {
	if len(body) == 0 {
		return status
	}
	var m map[string]interface{}
	if json.Unmarshal(body, &m) == nil {
		for _, k := range []string{"message", "msg", "error"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return string(body)
}
