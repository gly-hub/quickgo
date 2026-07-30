package grpcep

import (
	"testing"

	gogoproto "github.com/gogo/protobuf/proto"
)

func TestCommonRespLegacySerializationCompatibility(t *testing.T) {
	original := &CommonResp{Code: 200, Msg: "ok"}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CommonResp
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Code != original.Code || decoded.Msg != original.Msg {
		t.Fatalf("decoded response = (code=%d, msg=%q), want (code=%d, msg=%q)", decoded.Code, decoded.Msg, original.Code, original.Msg)
	}

	legacyData, err := gogoproto.Marshal(original)
	if err != nil {
		t.Fatalf("gogo marshal: %v", err)
	}
	var legacyDecoded CommonResp
	if err := gogoproto.Unmarshal(legacyData, &legacyDecoded); err != nil {
		t.Fatalf("gogo unmarshal: %v", err)
	}
	if legacyDecoded.Code != original.Code || legacyDecoded.Msg != original.Msg {
		t.Fatalf("legacy decoded response = (code=%d, msg=%q), want (code=%d, msg=%q)", legacyDecoded.Code, legacyDecoded.Msg, original.Code, original.Msg)
	}
}
