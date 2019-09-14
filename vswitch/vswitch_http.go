package vswitch

import (
    "os"
    "fmt"
    "html"
    "log"
    "time"
    "math/rand"
    "io/ioutil"
    "net/http"
    "encoding/json"
)

type VSwitchHttp struct {
    wroker *VSwitchWroker
    listen string
    adminToken string
    adminFile string
}

func getToken(n int) string {
    letters := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
    buf := make([]byte, n)

    rand.Seed(time.Now().UnixNano())

    for i := range buf {
        buf[i] = letters[rand.Int63() % int64(len(letters))]
    }

    return string(buf)
}

func NewVSwitchHttp(wroker *VSwitchWroker, c *Config)(this *VSwitchHttp) {
    token := c.Token
    if token == "" {
        token = getToken(16)
    }
    this = &VSwitchHttp {
        wroker: wroker,
        listen: c.HttpListen,
        adminToken: token,
        adminFile: c.TokenFile,
    }

    this.SaveToken()
    http.HandleFunc("/", this.Index)
    http.HandleFunc("/hi", this.Hi)
    http.HandleFunc("/user", this.User)
    http.HandleFunc("/neighbor", this.Neighbor)

    return 
}

func (this *VSwitchHttp) SaveToken() error {
    log.Printf("Info| VSwitchHttp.SaveToken: AdminToken: %s", this.adminToken)

    f, err := os.OpenFile(this.adminFile, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0600)
    defer f.Close()
    if err != nil {
        log.Printf("Error| VSwitchHttp.SaveToken: %s", err)
        return err
    }

    if _, err := f.Write([]byte(this.adminToken)); err != nil {
        log.Printf("Error| VSwitchHttp.SaveToken: %s", err)
        return err
    }

    return nil
}

func (this *VSwitchHttp) GoStart() error {
    log.Printf("Debug| NewHttp on %s", this.listen)
    if err := http.ListenAndServe(this.listen, nil); err != nil {
        log.Printf("Error| VSwitchHttp.GoStart on %s: %s", this.listen, err)
        return err
    }
    return nil
}

func (this *VSwitchHttp) IsAuth(w http.ResponseWriter, r *http.Request) bool {
    token, pass, ok := r.BasicAuth()
    if this.wroker.IsVerbose() {
        log.Printf("Debug| VSwitchHttp.IsAuth token: %s, pass: %s", token, pass)
    }
    if !ok  || token != this.adminToken {
        w.Header().Set("WWW-Authenticate", "Basic")
        http.Error(w, "Authorization Required.", 401)
        return false
    }

    return true
}

func (this *VSwitchHttp) Hi(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi %s %q", r.Method, html.EscapeString(r.URL.Path))

    for name, headers := range r.Header {
        for _, h := range headers {
            log.Printf("Info| VSwitchHttp.Hi %v: %v", name, h)
        }
    }
}

func (this *VSwitchHttp) Index(w http.ResponseWriter, r *http.Request) {
    if (!this.IsAuth(w, r)) {
        return
    }

    switch (r.Method) {
    case "GET":  
        body := fmt.Sprintf("uptime: %d\n", this.wroker.UpTime())
        body += "uptime, remoteaddr, device, receipt, transmission, error\n"
        for p := range this.wroker.ListPoint() {
            if p == nil {
                break
            }

            client, ifce := p.Client, p.Device
            body += fmt.Sprintf("%d, %s, %s, %d, %d, %d\n", 
                                client.UpTime(), client.GetAddr(), ifce.Name(),
                                client.RxOkay, client.TxOkay, client.TxError)
        }
        fmt.Fprintf(w, body)
    default:
        http.Error(w, fmt.Sprintf("Not support %s", r.Method), 400)
        return 
    }
}

func (this *VSwitchHttp) User(w http.ResponseWriter, r *http.Request) {
    if (!this.IsAuth(w, r)) {
        return
    }

    switch (r.Method) {
    case "GET":
        body := "username, password, token\n"
        for u := range this.wroker.ListUser() {
            if u == nil {
                break
            }
            body += fmt.Sprintf("%s, %s, %s\n", u.Name, u.Password, u.Token)
        }
        fmt.Fprintf(w, body)
    case "POST":
        defer r.Body.Close()
        body, err := ioutil.ReadAll(r.Body)
        if err != nil {
            http.Error(w, fmt.Sprintf("Error| VSwitchHttp.User: %s", err), 400)
            return
        }

        user := &User {}
        if err := json.Unmarshal([]byte(body), user); err != nil {
            http.Error(w, fmt.Sprintf("Error| VSwitchHttp.User: %s", err), 400)
            return
        }

        this.wroker.AddUser(user)

        fmt.Fprintf(w, "Saved it.")
    default:
        http.Error(w, fmt.Sprintf("Not support %s", r.Method), 400)
    }
}

func (this *VSwitchHttp) Neighbor(w http.ResponseWriter, r *http.Request) {
    if (!this.IsAuth(w, r)) {
        return
    }

    switch (r.Method) {
    case "GET":  
        body := "uptime, ethernet, address, remote\n"
        for n := range this.wroker.Neighbor.ListNeighbor() {
            if n == nil {
                break
            }
            
            body += fmt.Sprintf("%d, %s, %s, %s\n", 
                                n.UpTime(), n.HwAddr, n.IpAddr, n.Client)
        }
        fmt.Fprintf(w, body)
    default:
        http.Error(w, fmt.Sprintf("Not support %s", r.Method), 400)
        return 
    }
}

