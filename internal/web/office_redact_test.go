package web

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOfficeHostObservationSanitizesNestedOutputWithoutChangingShape(t *testing.T){
	raw:=json.RawMessage(`{"cursor":12,"lines":["hello","Authorization: Bearer secret-value"],"config":{"api_key":"private-value"}}`)
	safe:=officeSafeJSON(raw)
	encoded,err:=json.Marshal(safe);if err!=nil{t.Fatal(err)}
	if strings.Contains(string(encoded),"secret-value")||strings.Contains(string(encoded),"private-value"){t.Fatal("host secret crossed presentation boundary")}
	var result struct{Cursor int `json:"cursor"`;Lines []string `json:"lines"`}
	if json.Unmarshal(encoded,&result)!=nil||result.Cursor!=12||len(result.Lines)!=2||result.Lines[0]!="hello"{t.Fatal("sanitization changed observed protocol fields")}
}
