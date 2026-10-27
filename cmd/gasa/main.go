package main

import (
 "bufio"
 "context"
 "errors"
 "flag"
 "fmt"
 "net"
 "net/http"
 "os"
 "os/exec"
 "os/signal"
 "path/filepath"
 "runtime"
 "strings"
 "time"
)

func main() {
 if err := run(os.Args[1:]); err != nil { fmt.Fprintln(os.Stderr, "gasa:", err); os.Exit(1) }
}

func run(args []string) error {
 flags := flag.NewFlagSet("gasa", flag.ContinueOnError)
 noBrowser := flags.Bool("no-browser", false, "Do not open the dashboard in a browser")
 flags.Usage = func(){ help() }
 if err:=flags.Parse(args); err!=nil { return err }
 rest:=flags.Args()
 if len(rest)>0 {
  switch rest[0] {
  case "help": help(); return nil
  case "doctor": doctor(); return nil
  case "web":
   if len(rest)>1 && rest[1]=="open" { return serve(context.Background(),true,true) }
   return errors.New("usage: gasa web open")
  default: return fmt.Errorf("unknown command %q (try gasa --help)",rest[0])
  }
 }
 cwd,err:=os.Getwd(); if err!=nil{return err}
 fmt.Println("GasaCLI pre-alpha |",cwd)
 fmt.Println("This build is a local chat shell; /goal is not implemented.")
 fmt.Print("Trust this directory for future project operations? [y/N]: ")
 reader:=bufio.NewReader(os.Stdin)
 answer,_:=reader.ReadString('\n')
 if strings.ToLower(strings.TrimSpace(answer))!="y" {return errors.New("project not trusted")}
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt);defer stop()
 server,endpoint,err:=startServer();if err!=nil{return err}
 defer server.Shutdown(context.Background())
 fmt.Println("Dashboard:",endpoint)
 if !*noBrowser { _=openBrowser(endpoint) }
 fmt.Println("Type /help or /exit.")
 for {
  fmt.Print("gasa> ")
  line,err:=reader.ReadString('\n')
  if err!=nil {return nil}
  input:=strings.TrimSpace(line)
  switch input {
  case "/exit","/quit":return nil
  case "/help":help()
  case "/agents","/doctor":doctor()
  case "/status":fmt.Println("No active run. Orchestration is not implemented yet.")
  case "":continue
  default:
   if strings.HasPrefix(input,"/goal") {fmt.Println("/goal is planned but not implemented. No code was changed.")} else {fmt.Println("Interactive model chat is not connected yet. Use /help.")}
  }
  select {case <-ctx.Done():return nil;default:}
 }
}

func help(){
 fmt.Println("GasaCLI pre-alpha commands")
 fmt.Println("  gasa [--no-browser]   Open terminal shell and local dashboard")
 fmt.Println("  gasa doctor           Discover installed coding CLIs")
 fmt.Println("  gasa web open         Start standalone local dashboard")
 fmt.Println("  gasa --help           Show this help")
 fmt.Println("  /help /agents /status /exit")
 fmt.Println("  /goal /permissions /backup /restore (planned, not implemented)")
}

func doctor(){
 for _,name:=range []string{"claude","codex","gemini","cursor","opencode","orca","gh"} {
  path,err:=exec.LookPath(name)
  if err!=nil {fmt.Printf("%-10s not found\n",name);continue}
  fmt.Printf("%-10s installed at %s (authentication not checked)\n",name,path)
 }
}

func startServer()(*http.Server,string,error){
 mux:=http.NewServeMux()
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/" {http.NotFound(w,r);return}
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  w.Header().Set("Cache-Control","no-store")
  fmt.Fprint(w,`<!doctype html><html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GasaCLI</title><style>body{font:16px system-ui;background:#101722;color:#e9f1fa;max-width:760px;margin:12vh auto;padding:24px}section{border:1px solid #35465c;border-radius:16px;padding:24px}small{color:#aebed0}</style><section><h1>GasaCLI</h1><p>Dashboard local — pre-alpha</p><small>Monitoramento, agentes e analytics ainda não implementados. Nenhum dado de tokens ou custo foi coletado.</small></section></html>`)
 })
 listener,err:=net.Listen("tcp","127.0.0.1:3199")
 if err!=nil {listener,err=net.Listen("tcp","127.0.0.1:0")}
 if err!=nil{return nil,"",err}
 srv:=&http.Server{Handler:mux,ReadHeaderTimeout:5*time.Second}
 go func(){_ = srv.Serve(listener)}()
 return srv,"http://"+listener.Addr().String(),nil
}

func serve(ctx context.Context,launch,wait bool)error{
 srv,url,err:=startServer();if err!=nil{return err};defer srv.Shutdown(context.Background())
 fmt.Println("Dashboard:",url)
 if launch {_=openBrowser(url)}
 if wait {fmt.Println("Press Ctrl+C to stop");sig,stop:=signal.NotifyContext(ctx,os.Interrupt);defer stop();<-sig.Done()}
 return nil
}

func openBrowser(url string)error{
 var cmd *exec.Cmd
 switch runtime.GOOS {
 case "windows":cmd=exec.Command("rundll32","url.dll,FileProtocolHandler",url)
 case "darwin":cmd=exec.Command("open",url)
 default:cmd=exec.Command("xdg-open",url)
 }
 return cmd.Start()
}

var _ = filepath.Separator
