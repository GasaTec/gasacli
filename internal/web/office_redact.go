package web

import (
	"encoding/json"
	"strings"

	"github.com/GasaTec/gasacli/internal/sessions"
)

// Host output is untrusted and may contain provider credentials or tool output.
// Sanitize string values without modifying JSON syntax or manufacturing receipt
// fields; the underlying durable receipt remains distinct from this UI view.
func officeSafeJSON(raw json.RawMessage)any{
	var value any
	if json.Unmarshal(raw,&value)!=nil{return map[string]string{"status":"unavailable","reason":"Resposta estruturada indisponível."}}
	return officeSafeValue(value)
}

func officeSafeValue(value any)any{
	switch v:=value.(type){
	case string:return sessions.Redact(v)
	case []any:
		for i:=range v{v[i]=officeSafeValue(v[i])};return v
	case map[string]any:
		for key,item:=range v{
			name:=strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key,"_",""),"-",""))
			switch name{
			case "token","accesstoken","refreshtoken","authorization","password","secret","apikey","privatekey","credentials":v[key]="[REDACTED]"
			default:v[key]=officeSafeValue(item)
			}
		};return v
	default:return value
	}
}
